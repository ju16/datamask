package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------- XML 文本节点扫描 ----------

var (
	// 段落开始：<w:p> 或 <w:p 属性...>（排除 <w:pPr>）
	reParaStart = regexp.MustCompile(`<w:p(?:\s[^>]*)?>`)
	// 段落结束：</w:p>（排除 </w:pPr>）
	reParaEnd = regexp.MustCompile(`</w:p>`)
	// 文本节点：<w:t> 或 <w:t 属性...>内容</w:t>（排除 <w:tbl> / <w:tc> 等）
	reTextNode = regexp.MustCompile(`(<w:t(?:\s[^>]*)?>)([^<]*)</w:t>`)
)

// textSpan 一个 w:t 文本节点在原始 XML 中的位置
type textSpan struct {
	openStart, openEnd       int // <w t ...> 开标签的字节范围
	contentStart, contentEnd int
	openTag                  string // 开标签原文（用于检查 xml:space）
}

// paragraph 一个段落的文本节点集合
type paragraph struct {
	text  string
	spans []textSpan
}

// docPart 一个 OOXML 部件（document/header/footer 等）
type docPart struct {
	name       string
	xml        []byte
	paragraphs []*paragraph
}

// Document 打开的 .docx 文档
type Document struct {
	path       string
	partOrder  []string            // 原始 zip 条目顺序
	rawEntries map[string][]byte   // 全部条目原始内容
	parts      map[string]*docPart // 需要处理的部件
}

// targetParts 需要扫描脱敏的部件
var targetPartRe = regexp.MustCompile(`^word/(document\.xml|header\d*\.xml|footer\d*\.xml|footnotes\.xml|endnotes\.xml)$`)

// OpenDocx 打开 .docx 文件并解析其中的段落
func OpenDocx(path string) (*Document, error) {
	if filepath.Ext(strings.ToLower(path)) != ".docx" {
		return nil, fmt.Errorf("不支持的文件格式: %s，仅支持 .docx 文件", filepath.Ext(path))
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("不是有效的 .docx 文件（无法解析 zip）: %v", err)
	}
	defer zr.Close()

	doc := &Document{
		path:       path,
		rawEntries: make(map[string][]byte),
		parts:      make(map[string]*docPart),
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("读取 zip 条目失败 %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("读取 zip 条目失败 %s: %v", f.Name, err)
		}
		doc.rawEntries[f.Name] = data
		doc.partOrder = append(doc.partOrder, f.Name)
		if targetPartRe.MatchString(f.Name) {
			doc.parts[f.Name] = parsePart(f.Name, data)
		}
	}
	if len(doc.parts) == 0 {
		return nil, fmt.Errorf("未找到 word/document.xml，不是有效的 Word 文档")
	}
	return doc, nil
}

// parsePart 解析单个 XML 部件，按文档顺序提取段落及其文本节点
func parsePart(name string, data []byte) *docPart {
	xml := string(data)
	p := &docPart{name: name, xml: data}

	// 三类标签分别扫描，按出现顺序归并处理
	type ev struct {
		kind byte // 's'=段落开始 'e'=段落结束 't'=文本节点
		span textSpan
		text string
		pos  int
	}
	var events []ev
	for _, m := range reParaStart.FindAllStringIndex(xml, -1) {
		events = append(events, ev{kind: 's', pos: m[0], span: textSpan{openStart: m[0], openEnd: m[1]}})
	}
	for _, m := range reParaEnd.FindAllStringIndex(xml, -1) {
		events = append(events, ev{kind: 'e', pos: m[0]})
	}
	for _, m := range reTextNode.FindAllStringSubmatchIndex(xml, -1) {
		events = append(events, ev{kind: 't', pos: m[0],
			span: textSpan{openStart: m[2], openEnd: m[3], contentStart: m[4], contentEnd: m[5], openTag: string(xml[m[2]:m[3]])},
			text: string(xml[m[4]:m[5]])})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].pos < events[j].pos })

	var cur *paragraph
	for _, e := range events {
		switch e.kind {
		case 's':
			cur = &paragraph{}
			p.paragraphs = append(p.paragraphs, cur)
		case 't':
			if cur != nil {
				cur.spans = append(cur.spans, e.span)
				cur.text += unescapeXML(e.text)
			}
		case 'e':
			cur = nil
		}
	}
	return p
}

// ---------- 脱敏与恢复 ----------

// MaskFunc 对单段文本执行脱敏，返回新文本和替换记录
type MaskFunc func(part string, paraIdx int, text string) (string, []ReplacementRecord)

// Mask 对文档所有部件执行脱敏
func (d *Document) Mask(fn MaskFunc) ([]ReplacementRecord, error) {
	var all []ReplacementRecord
	names := make([]string, 0, len(d.parts))
	for n := range d.parts {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		part := d.parts[name]
		edits := []spanEdit{}
		for idx, para := range part.paragraphs {
			if strings.TrimSpace(para.text) == "" {
				continue
			}
			newText, records := fn(name, idx, para.text)
			if len(records) == 0 {
				continue
			}
			all = append(all, records...)
			edits = append(edits, rewriteParagraph(para, newText)...)
		}
		if len(edits) > 0 {
			part.xml = applyEdits(part.xml, edits)
		}
	}
	return all, nil
}

// Restore 根据替换记录恢复文档
func (d *Document) Restore(records []ReplacementRecord) error {
	// 按部件分组
	byPart := map[string][]ReplacementRecord{}
	for _, r := range records {
		byPart[r.Part] = append(byPart[r.Part], r)
	}

	for name, recs := range byPart {
		part, ok := d.parts[name]
		if !ok {
			return fmt.Errorf("日志中的部件 %s 在当前文档中不存在", name)
		}
		// 按段落索引分组
		byPara := map[int][]ReplacementRecord{}
		maxIdx := -1
		for _, r := range recs {
			byPara[r.ParagraphIndex] = append(byPara[r.ParagraphIndex], r)
			if r.ParagraphIndex > maxIdx {
				maxIdx = r.ParagraphIndex
			}
		}
		if maxIdx >= len(part.paragraphs) {
			return fmt.Errorf("日志中的段落索引 %d 超出范围（共 %d 段）", maxIdx, len(part.paragraphs))
		}

		// 段落从后往前处理（避免索引偏移）
		paraIdxs := make([]int, 0, len(byPara))
		for i := range byPara {
			paraIdxs = append(paraIdxs, i)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(paraIdxs)))

		var edits []spanEdit
		for _, pi := range paraIdxs {
			para := part.paragraphs[pi]
			text := para.text
			// 反序遍历替换记录 = 最后应用的规则先恢复
			reps := byPara[pi]
			for i := len(reps) - 1; i >= 0; i-- {
				rep := reps[i]
				original, masked := rep.Original, rep.Masked
				s, e := rep.Start, rep.End
				if e <= s {
					e = s + len(masked)
				}
				// 策略 A：按记录的位置直接替换
				if s < len(text) && e <= len(text) && text[s:e] == masked {
					text = text[:s] + original + text[e:]
					continue
				}
				// 策略 B：位置偏移了，用 rfind 自右向左查找
				if pos := strings.LastIndex(text, masked); pos >= 0 {
					text = text[:pos] + original + text[pos+len(masked):]
					continue
				}
				// 策略 C：从左向右逐个查找
				done := false
				for off := 0; off+len(masked) <= len(text); off++ {
					if text[off:off+len(masked)] == masked {
						text = text[:off] + original + text[off+len(masked):]
						done = true
						break
					}
				}
				if !done {
					return fmt.Errorf("恢复失败：在段落 %d 中找不到掩码文本 %q", pi, masked)
				}
			}
			if text != para.text {
				edits = append(edits, rewriteParagraph(para, text)...)
			}
		}
		if len(edits) > 0 {
			part.xml = applyEdits(part.xml, edits)
		}
	}
	return nil
}

// spanEdit 一处字节替换
type spanEdit struct {
	start, end int
	repl       []byte
}

// rewriteParagraph 把段落整体文本写入第一个 w:t，其余清空
func rewriteParagraph(para *paragraph, newText string) []spanEdit {
	if len(para.spans) == 0 {
		return nil
	}
	var edits []spanEdit
	esc := []byte(escapeXML(newText))
	first := para.spans[0]
	edits = append(edits, spanEdit{first.contentStart, first.contentEnd, esc})
	// 首尾有空白时确保 xml:space="preserve"
	if newText != strings.TrimSpace(newText) && !strings.Contains(first.openTag, "xml:space") {
		edits = append(edits, spanEdit{first.openEnd - 1, first.openEnd - 1,
			[]byte(` xml:space="preserve"`)})
	}
	for _, sp := range para.spans[1:] {
		edits = append(edits, spanEdit{sp.contentStart, sp.contentEnd, nil})
	}
	return edits
}

// applyEdits 按位置倒序应用字节替换
func applyEdits(xml []byte, edits []spanEdit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := xml
	for _, e := range edits {
		if e.start < 0 || e.end > len(out) || e.start > e.end {
			continue
		}
		buf := make([]byte, 0, len(out)-(e.end-e.start)+len(e.repl))
		buf = append(buf, out[:e.start]...)
		buf = append(buf, e.repl...)
		buf = append(buf, out[e.end:]...)
		out = buf
	}
	return out
}

// ---------- 保存 ----------

// Save 保存文档到 .docx（保留其余条目不变）
func (d *Document) Save(path string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range d.partOrder {
		data := d.rawEntries[name]
		if part, ok := d.parts[name]; ok {
			data = part.xml
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return fmt.Errorf("创建 zip 条目失败 %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("写入 zip 条目失败 %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("关闭 zip 失败: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %v", path, err)
	}
	return nil
}

// ParagraphCount 返回各部件的段落数（测试用）
func (d *Document) ParagraphCount(part string) int {
	if p, ok := d.parts[part]; ok {
		return len(p.paragraphs)
	}
	return 0
}

// ParagraphText 返回指定部件段落的文本（测试用）
func (d *Document) ParagraphText(part string, idx int) (string, error) {
	p, ok := d.parts[part]
	if !ok {
		return "", fmt.Errorf("部件不存在: %s", part)
	}
	if idx < 0 || idx >= len(p.paragraphs) {
		return "", fmt.Errorf("段落索引越界: %d", idx)
	}
	return p.paragraphs[idx].text, nil
}

// ---------- XML 转义 ----------

func unescapeXML(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	repl := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&apos;", "'",
		"&#39;", "'",
		"&amp;", "&",
	)
	// 处理数字实体 &#nn; / &#xhh;
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '&' && i+1 < len(s) && s[i+1] == '#' {
			end := strings.IndexByte(s[i:], ';')
			if end > 0 {
				code := s[i+2 : i+end]
				var n int64
				var err error
				if len(code) > 1 && (code[0] == 'x' || code[0] == 'X') {
					n, err = strconv.ParseInt(code[1:], 16, 32)
				} else {
					n, err = strconv.ParseInt(code, 10, 32)
				}
				if err == nil {
					b.WriteRune(rune(n))
					i += end
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return repl.Replace(b.String())
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}
