package main

import (
	"fmt"
	"strings"
)

// rulesTxtHeader 规则文件导出时的说明头
const rulesTxtHeader = `# DataMask 脱敏规则文件
# ─────────────────────────────────────────────
# 每条规则两段式：一行顶格写规则名称，下面缩进的行写内容：
#
#   规则名称
#     查找: 正则表达式（必填）
#     替换: 替换模板（可省略，省略时整段匹配内容替换为 ***）
#     说明: 这条规则做什么（可省略）
#     开关: 开 或 关（可省略，默认开）
#
# 说明：
#   - 查找正则与常见写法一致：\d 数字、\b 词边界、a|b 二选一、\u4e00 也支持
#   - 替换模板中 \1、\2 表示保留第 1、2 个括号里捕获的内容，* 为掩码字符
#   - 以 # 开头的行是注释，空行忽略；正则必须写在同一行内
#
# 编辑保存后，用下面的命令使用本文件：
#   datamask mask 文档.docx -r 本文件路径
# 查看规则是否生效：
#   datamask rules -r 本文件路径
`

// isYAMLConfig 判断配置内容是 YAML 还是纯文本规则文件。
// 取第一个非空非注释行：YAML 以 version:/rules: 开头；文本规则以规则名（顶格非注释行）开头。
func isYAMLConfig(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return strings.HasPrefix(t, "version:") || strings.HasPrefix(t, "rules:")
	}
	return true // 空文件交给 YAML 解析去报错
}

// txtRuleField 一条文本规则的字段
type txtRuleField struct {
	name        string
	description string
	pattern     string
	replacement string
	enabled     bool
	hasEnabled  bool
}

// parseRulesTxt 解析纯文本规则文件（两段式块格式：规则名顶格，内容缩进行 key: value）
func parseRulesTxt(data []byte) ([]rawRule, error) {
	var txts []txtRuleField
	var cur *txtRuleField
	badLine := func(lineNo int, msg string) error {
		return fmt.Errorf("规则文件第 %d 行有误: %s", lineNo, msg)
	}

	for i, rawLine := range strings.Split(string(data), "\n") {
		lineNo := i + 1
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// 缩进行 → 当前规则的内容字段
		if line != trimmed { // 有前导空白
			if cur == nil {
				return nil, badLine(lineNo, "缩进的内容行之前缺少顶格的规则名称")
			}
			// 取最先出现的冒号（全角/半角均可作为字段分隔符）
			fi := strings.IndexByte(trimmed, ':')
			fj := strings.Index(trimmed, "：")
			var key, value string
			switch {
			case fi >= 0 && (fj < 0 || fi < fj):
				key, value = trimmed[:fi], trimmed[fi+1:]
			case fj >= 0:
				key, value = trimmed[:fj], trimmed[fj+len("："):]
			default:
				return nil, badLine(lineNo, fmt.Sprintf("缺少「:」，应为「字段: 内容」格式（%q）", trimmed))
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "查找", "找", "正则", "匹配":
				if cur.pattern != "" {
					return nil, badLine(lineNo, "重复的「查找」字段（正则必须写在同一行内）")
				}
				cur.pattern = value
			case "替换", "换":
				cur.replacement = value
			case "说明", "描述":
				cur.description = value
			case "开关", "启用":
				v, ok := parseToggle(value)
				if !ok {
					return nil, badLine(lineNo, fmt.Sprintf("无法识别的开关状态 %q（应为 开/关/启用/禁用 等）", value))
				}
				cur.enabled = v
				cur.hasEnabled = true
			default:
				return nil, badLine(lineNo, fmt.Sprintf("未知字段 %q（支持：查找/替换/说明/开关）", key))
			}
			continue
		}

		// 顶格行 → 新规则名
		txts = append(txts, txtRuleField{name: trimmed, enabled: true})
		cur = &txts[len(txts)-1]
	}

	if len(txts) == 0 {
		return nil, fmt.Errorf("规则文件中没有规则（每条规则：一行顶格写名称，下面缩进写「查找: 正则」）")
	}

	var rules []rawRule
	for _, tr := range txts {
		if tr.pattern == "" {
			return nil, fmt.Errorf("规则 %q 缺少「查找: 正则」字段", tr.name)
		}
		enabled := true
		if tr.hasEnabled {
			enabled = tr.enabled
		}
		repl := tr.replacement
		if repl == "" {
			repl = "***"
		}
		rules = append(rules, rawRule{
			Name:        tr.name,
			Description: tr.description,
			Pattern:     tr.pattern,
			Replacement: repl,
			Enabled:     &enabled,
		})
	}
	return rules, nil
}

// parseToggle 解析启用/禁用关键字
func parseToggle(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "开", "启用", "true", "yes", "on", "y", "enabled":
		return true, true
	case "关", "禁用", "false", "no", "off", "n", "disabled":
		return false, true
	}
	return false, false
}

// renderRulesTxt 把规则渲染为可编辑的纯文本规则文件（两段式块格式）
func renderRulesTxt(rules []rawRule) []byte {
	var b strings.Builder
	b.WriteString(rulesTxtHeader)
	b.WriteString("\n")
	for _, r := range rules {
		b.WriteString(r.Name + "\n")
		if r.Description != "" {
			b.WriteString("  说明: " + r.Description + "\n")
		}
		b.WriteString("  查找: " + r.Pattern + "\n")
		if r.Replacement != "" && r.Replacement != "***" {
			b.WriteString("  替换: " + r.Replacement + "\n")
		} else if r.Replacement == "" {
			b.WriteString("  替换: ***\n")
		}
		if r.Enabled != nil && !*r.Enabled {
			b.WriteString("  开关: 关\n")
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}
