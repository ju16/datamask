package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- 测试辅助：构造 docx ----------

func buildDocx(t *testing.T, paragraphs []string) []byte {
	t.Helper()
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString("<w:p><w:r><w:t>")
		body.WriteString(escapeXML(p))
		body.WriteString("</w:t></w:r></w:p>")
	}
	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>%s</w:body></w:document>`, body.String())

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("[Content_Types].xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`))
	w, _ = zw.Create("word/document.xml")
	w.Write([]byte(docXML))
	zw.Close()
	return buf.Bytes()
}

// buildDocxMultiRun 构造文本被拆分到多个 run 的 docx（模拟真实 Word 文档）
func buildDocxMultiRun(t *testing.T, paragraphs [][]string) []byte {
	t.Helper()
	var body strings.Builder
	for _, runs := range paragraphs {
		body.WriteString("<w:p>")
		for _, r := range runs {
			body.WriteString(fmt.Sprintf(`<w:r><w:t xml:space="preserve">%s</w:t></w:r>`, escapeXML(r)))
		}
		body.WriteString("</w:p>")
	}
	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>%s</w:body></w:document>`, body.String())

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("[Content_Types].xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`))
	w, _ = zw.Create("word/document.xml")
	w.Write([]byte(docXML))
	zw.Close()
	return buf.Bytes()
}

func writeTempDocx(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "contract.docx")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 内置配置的路径（go:embed 的 config.yaml 在测试中同样可用）
func testConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, embeddedConfig, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------- 端到端：脱敏 + 恢复闭环 ----------

func TestMaskRestoreRoundtrip(t *testing.T) {
	paragraphs := []string{
		"甲方信息：统一社会信用代码 91310000313644633B，联系电话 13812345678。",
		"法定代表人：张三，身份证号 110101199001011234。",
		"开户银行：中国某银行，账号 6222021234567890123。",
		"联系人邮箱 zhangsan@example.com，传真 010-88888888。",
		"护照号 E12345678，香港身份证 A123456(7)。",
		"合同金额：¥1,234,567.89，大写：壹佰贰拾叁万元整。",
		"乙方地址：广东省深圳市南山区科技南路18号。",
		"IP 地址 192.168.1.1（默认规则禁用，不应被掩码）。",
		"这是一段没有任何敏感信息的普通文本。",
	}
	path := writeTempDocx(t, buildDocx(t, paragraphs))
	cfg := testConfigPath(t)

	// ---- 脱敏 ----
	result, err := MaskFile(path, "", cfg)
	if err != nil {
		t.Fatalf("mask failed: %v", err)
	}
	if result.TotalReplacements == 0 {
		t.Fatal("expected replacements, got 0")
	}

	maskedDoc, err := OpenDocx(result.MaskedFile)
	if err != nil {
		t.Fatalf("reopen masked doc failed: %v", err)
	}
	maskedTexts := make([]string, len(paragraphs))
	for i := range paragraphs {
		txt, err := maskedDoc.ParagraphText("word/document.xml", i)
		if err != nil {
			t.Fatalf("read masked para %d: %v", i, err)
		}
		maskedTexts[i] = txt
	}

	// 验证关键掩码（期望值与 config.yaml 规则定义一致：credit_code 保留前2后2）
	checks := []struct {
		para   int
		has    string // 应出现
		hasNot string // 不应出现
	}{
		{0, "91**************3B", "91310000313644633B"},
		{0, "138****5678", "13812345678"},
		{1, "法定代表人：**", "110101199001011234"},
		{1, "110101********1234", "张三"},
		{2, "6222********0123", "6222021234567890123"},
		{3, "z***@example.com", "zhangsan@example.com"},
		{3, "010-8888****", "010-88888888"},
		{4, "E********", "E12345678"},
		{4, "A******(7)", "A123456(7)"},
		{5, "¥******.**元", "1,234,567.89"},
		{6, "广东省深圳市南山区****", "科技南路18号"},
		{7, "192.168.1.1", ""}, // ip_address 默认禁用
		{8, "没有任何敏感信息", ""},
	}
	for _, c := range checks {
		got := maskedTexts[c.para]
		if c.has != "" && !strings.Contains(got, c.has) {
			t.Errorf("para %d: expected to contain %q, got %q", c.para, c.has, got)
		}
		if c.hasNot != "" && strings.Contains(got, c.hasNot) {
			t.Errorf("para %d: expected NOT to contain %q, got %q", c.para, c.hasNot, got)
		}
	}

	// ---- 恢复 ----
	log := NewRecoveryLog(result.SourceFile, result.MaskedFile, result.RulesApplied, result.Replacements)
	logPath := filepath.Join(filepath.Dir(result.MaskedFile), "test.masklog.json")
	if _, err := log.Save(logPath); err != nil {
		t.Fatal(err)
	}
	if err := RestoreFile(logPath, ""); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	restoredPath := strings.TrimSuffix(result.MaskedFile, ".docx") + "_restored.docx"

	restoredDoc, err := OpenDocx(restoredPath)
	if err != nil {
		t.Fatalf("reopen restored doc failed: %v", err)
	}
	for i, want := range paragraphs {
		got, err := restoredDoc.ParagraphText("word/document.xml", i)
		if err != nil {
			t.Fatalf("read restored para %d: %v", i, err)
		}
		if got != want {
			t.Errorf("para %d restore mismatch:\n want %q\n  got %q", i, want, got)
		}
	}
}

// ---------- 跨 run 段落的脱敏与恢复 ----------

func TestMultiRunParagraph(t *testing.T) {
	runs := [][]string{
		{"身份证号：", "110101", "19900101", "1234", "，请核对。"},
		{"电话 ", "138", "12345678", " 结束"},
	}
	path := writeTempDocx(t, buildDocxMultiRun(t, runs))
	cfg := testConfigPath(t)

	result, err := MaskFile(path, "", cfg)
	if err != nil {
		t.Fatalf("mask failed: %v", err)
	}
	if result.TotalReplacements < 2 {
		t.Fatalf("expected >=2 replacements, got %d", result.TotalReplacements)
	}

	maskedDoc, _ := OpenDocx(result.MaskedFile)
	t0, _ := maskedDoc.ParagraphText("word/document.xml", 0)
	if strings.Contains(t0, "110101199001011234") {
		t.Errorf("cross-run id_card not masked: %q", t0)
	}
	if !strings.Contains(t0, "110101********1234") {
		t.Errorf("expected masked id_card, got %q", t0)
	}
	t1, _ := maskedDoc.ParagraphText("word/document.xml", 1)
	if !strings.Contains(t1, "138****5678") {
		t.Errorf("expected masked phone, got %q", t1)
	}

	// 恢复
	log := NewRecoveryLog(result.SourceFile, result.MaskedFile, result.RulesApplied, result.Replacements)
	logPath := filepath.Join(filepath.Dir(result.MaskedFile), "multi.masklog.json")
	log.Save(logPath)
	if err := RestoreFile(logPath, ""); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	restoredPath := strings.TrimSuffix(result.MaskedFile, ".docx") + "_restored.docx"
	restoredDoc, _ := OpenDocx(restoredPath)
	got, _ := restoredDoc.ParagraphText("word/document.xml", 0)
	want := "身份证号：110101199001011234，请核对。"
	if got != want {
		t.Errorf("multi-run restore mismatch:\n want %q\n  got %q", want, got)
	}
}

// ---------- XML 实体与 preserve ----------

func TestXMLEntitiesAndPreserve(t *testing.T) {
	paragraphs := []string{
		"甲方<张三> & 乙方：电话 13812345678（含XML特殊字符）",
	}
	path := writeTempDocx(t, buildDocx(t, paragraphs))
	cfg := testConfigPath(t)

	result, err := MaskFile(path, "", cfg)
	if err != nil {
		t.Fatalf("mask failed: %v", err)
	}
	maskedDoc, _ := OpenDocx(result.MaskedFile)
	got, _ := maskedDoc.ParagraphText("word/document.xml", 0)
	if strings.Contains(got, "13812345678") {
		t.Errorf("phone not masked: %q", got)
	}
	if !strings.Contains(got, "甲方<张三> & 乙方") {
		t.Errorf("XML entities corrupted: %q", got)
	}

	// 首尾空格 preserve
	spacePath := writeTempDocx(t, buildDocxMultiRun(t, [][]string{{"  前导空格 13812345678  "}}))
	result2, err := MaskFile(spacePath, "", cfg)
	if err != nil {
		t.Fatalf("mask 2 failed: %v", err)
	}
	maskedDoc2, err := OpenDocx(result2.MaskedFile)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	got2, _ := maskedDoc2.ParagraphText("word/document.xml", 0)
	if !strings.Contains(got2, "138****5678") {
		t.Errorf("phone not masked in spaced para: %q", got2)
	}
	if !strings.HasPrefix(got2, "  前导空格") || !strings.HasSuffix(got2, "  ") {
		t.Errorf("leading/trailing spaces lost: %q", got2)
	}
	// xml:space="preserve" 应已写入首个子节点的开标签
	partXML := string(maskedDoc2.parts["word/document.xml"].xml)
	if !strings.Contains(partXML, `xml:space="preserve"`) {
		t.Errorf("xml:space=preserve missing in: %s", partXML)
	}
}

// ---------- 规则引擎单元测试 ----------

func TestPatternConversion(t *testing.T) {
	cases := []struct{ in, want string }{
		{`[\u4e00-\u9fff]{2,4}`, `[\x{4e00}-\x{9fff}]{2,4}`},
		{`\b(\d{6})\d{8}`, `\b(\d{6})\d{8}`},
		{`\u0041`, `\x{0041}`},
		{`\\u123`, `\\u123`}, // \\u 不转换
		{`\uzzzz`, `\uzzzz`}, // 非十六进制不转换
	}
	for _, c := range cases {
		if got := convertPattern(c.in); got != c.want {
			t.Errorf("convertPattern(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReplacementConversion(t *testing.T) {
	cases := []struct{ in, want string }{
		{`\1********\2`, `${1}********${2}`},
		{`\1：**`, `${1}：**`},
		{`¥******.**元`, `¥******.**元`},
	}
	for _, c := range cases {
		if got := convertReplacement(c.in); got != c.want {
			t.Errorf("convertReplacement(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRuleApplyRecords(t *testing.T) {
	cfg, err := LoadConfig(testConfigPath(t))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0)
	for _, r := range cfg.Rules() {
		names = append(names, r.Name)
	}
	// ip_address / mac_address 默认禁用
	for _, n := range names {
		if n == "ip_address" || n == "mac_address" {
			t.Errorf("disabled rule %s should not be active", n)
		}
	}
	// 规则顺序与配置一致
	if len(cfg.Rules()) != len(cfg.AllRules())-2 {
		t.Errorf("expected %d active rules, got %d", len(cfg.AllRules())-2, len(cfg.Rules()))
	}
}

// ---------- 恢复策略：位置偏移场景 ----------

func TestRestorePositionShift(t *testing.T) {
	// 构造：掩码后长度变化 + 同段多条替换，验证反序恢复
	text := "电话 13812345678 和 15998765432"
	cfg, _ := LoadConfig(testConfigPath(t))
	doc := &Document{}
	_ = doc

	// 直接用规则引擎模拟
	var records []ReplacementRecord
	current := text
	for _, rule := range cfg.Rules() {
		masked, reps := rule.Apply(current, "word/document.xml", 0)
		if len(reps) > 0 {
			records = append(records, reps...)
			current = masked
		}
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 phone records, got %d: %+v", len(records), records)
	}
	if !strings.Contains(current, "138****5678") || !strings.Contains(current, "159****5432") {
		t.Fatalf("masking failed: %q", current)
	}
	// 反向恢复
	restored := current
	for i := len(records) - 1; i >= 0; i-- {
		rep := records[i]
		s, e := rep.Start, rep.End
		if e <= s {
			e = s + len(rep.Masked)
		}
		if s < len(restored) && e <= len(restored) && restored[s:e] == rep.Masked {
			restored = restored[:s] + rep.Original + restored[e:]
			continue
		}
		if pos := strings.LastIndex(restored, rep.Masked); pos >= 0 {
			restored = restored[:pos] + rep.Original + restored[pos+len(rep.Masked):]
			continue
		}
		for off := 0; off+len(rep.Masked) <= len(restored); off++ {
			if restored[off:off+len(rep.Masked)] == rep.Masked {
				restored = restored[:off] + rep.Original + restored[off+len(rep.Masked):]
				break
			}
		}
	}
	if restored != text {
		t.Errorf("restore mismatch:\n want %q\n  got %q", text, restored)
	}
}

// ---------- 日志序列化兼容 ----------

func TestLogBackwardCompat(t *testing.T) {
	// 旧版日志无 part 字段
	oldJSON := `{"version":"1.0","created_at":"2026-01-01T00:00:00","source_file":"a.docx","masked_file":"b.docx","rules_applied":["phone"],"total_replacements":1,"replacements":[{"rule":"phone","paragraph_index":0,"start":3,"end":14,"original":"13812345678","masked":"138****5678"}]}`
	dir := t.TempDir()
	p := filepath.Join(dir, "old.masklog.json")
	os.WriteFile(p, []byte(oldJSON), 0644)
	log, err := LoadRecoveryLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if log.Replacements[0].Part != "word/document.xml" {
		t.Errorf("expected default part, got %q", log.Replacements[0].Part)
	}
	if log.TotalReplacements != 1 || log.MaskedFile != "b.docx" {
		t.Errorf("log fields wrong: %+v", log)
	}
}
