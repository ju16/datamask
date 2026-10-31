package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReplacementRecord 记录一次脱敏替换，用于恢复原文
type ReplacementRecord struct {
	Rule           string `json:"rule"`
	Part           string `json:"part"`
	ParagraphIndex int    `json:"paragraph_index"`
	Start          int    `json:"start"`
	End            int    `json:"end"`
	Original       string `json:"original"`
	Masked         string `json:"masked"`
}

// MaskRule 单条脱敏规则
type MaskRule struct {
	Name        string
	Description string
	Pattern     *regexp.Regexp
	Replacement string // Go Expand 格式（${1} 风格）
	Enabled     bool

	RawPattern     string // 配置中的原始正则
	RawReplacement string // 配置中的原始替换模板
}

// Apply 对文本应用脱敏规则，返回脱敏后文本和替换记录
func (r *MaskRule) Apply(text string, part string, paraIdx int) (string, []ReplacementRecord) {
	if !r.Enabled || text == "" {
		return text, nil
	}

	var records []ReplacementRecord
	// FindAllStringSubmatchIndex 返回完整匹配及捕获组位置，供 ExpandString 展开 \1 \2
	matches := r.Pattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		original := text[start:end]
		expanded := r.Pattern.ExpandString(nil, r.Replacement, text, m)
		records = append(records, ReplacementRecord{
			Rule:           r.Name,
			Part:           part,
			ParagraphIndex: paraIdx,
			Start:          start,
			End:            end,
			Original:       original,
			Masked:         string(expanded),
		})
		b.WriteString(text[last:start])
		b.WriteString(string(expanded))
		last = end
	}
	b.WriteString(text[last:])
	return b.String(), records
}

// convertPattern 将 Python re 的 \uXXXX 转义转换为 Go RE2 的 \x{XXXX} 写法
func convertPattern(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == 'u' && i+6 <= len(s) {
			hex := s[i+2 : i+6]
			ok := true
			for _, c := range hex {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
					ok = false
					break
				}
			}
			if ok {
				b.WriteString("\\x{")
				b.WriteString(hex)
				b.WriteString("}")
				i += 5
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// convertReplacement 将 Python re 风格的 \1 转换为 Go Expand 风格的 ${1}
func convertReplacement(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next >= '0' && next <= '9' {
				b.WriteString("${")
				b.WriteByte(next)
				b.WriteString("}")
				i++
				continue
			}
			b.WriteByte(next)
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// rawRule 配置解析阶段的原始规则（Pattern/Replacement 均为用户书写形式，未转换）
type rawRule struct {
	Name        string
	Description string
	Pattern     string
	Replacement string
	Enabled     *bool
}

// Config 配置管理器
type Config struct {
	path  string
	ver   string
	rules []*MaskRule
}

// LoadConfig 加载配置文件，自动识别格式：YAML（version:/rules: 开头）或纯文本规则文件（一行一条，| 分隔）
func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("配置文件不存在: %s", configPath)
	}

	var ver string
	var raws []rawRule
	if isYAMLConfig(data) {
		ver, raws, err = parseYAMLRules(data)
		if err != nil {
			return nil, err
		}
	} else {
		raws, err = parseRulesTxt(data)
		if err != nil {
			return nil, err
		}
		ver = "text"
	}
	return buildConfig(ver, raws, configPath)
}

// buildConfig 校验并编译规则，组装 Config
func buildConfig(ver string, raws []rawRule, configPath string) (*Config, error) {
	c := &Config{path: configPath, ver: ver}
	if c.ver == "" {
		c.ver = "1.0"
	}
	for _, rd := range raws {
		if rd.Name == "" || rd.Pattern == "" {
			return nil, fmt.Errorf("规则缺少 name 或 pattern 字段")
		}
		re, err := regexp.Compile(convertPattern(rd.Pattern))
		if err != nil {
			return nil, fmt.Errorf("规则 %s 正则编译失败: %v", rd.Name, err)
		}
		enabled := true
		if rd.Enabled != nil {
			enabled = *rd.Enabled
		}
		c.rules = append(c.rules, &MaskRule{
			Name:           rd.Name,
			Description:    rd.Description,
			Pattern:        re,
			Replacement:    convertReplacement(rd.Replacement),
			Enabled:        enabled,
			RawPattern:     rd.Pattern,
			RawReplacement: rd.Replacement,
		})
	}
	if len(c.rules) == 0 {
		return nil, fmt.Errorf("配置文件中没有规则")
	}
	return c, nil
}

// parseYAMLRules 解析 YAML 配置为原始规则
func parseYAMLRules(data []byte) (string, []rawRule, error) {
	var raw struct {
		Version string `yaml:"version"`
		Rules   []rawRuleYAML
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return "", nil, fmt.Errorf("配置文件解析失败: %v", err)
	}
	raws := make([]rawRule, 0, len(raw.Rules))
	for _, rd := range raw.Rules {
		raws = append(raws, rawRule{
			Name:        rd.Name,
			Description: rd.Description,
			Pattern:     rd.Pattern,
			Replacement: rd.Replacement,
			Enabled:     rd.Enabled,
		})
	}
	return raw.Version, raws, nil
}

// rawRuleYAML YAML 规则条目
type rawRuleYAML struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
	Enabled     *bool  `yaml:"enabled"`
}

// Rules 返回启用的规则（按配置顺序）
func (c *Config) Rules() []*MaskRule {
	var out []*MaskRule
	for _, r := range c.rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out
}

// AllRules 返回全部规则（含禁用）
func (c *Config) AllRules() []*MaskRule {
	return c.rules
}

// Version 返回配置版本
func (c *Config) Version() string { return c.ver }
