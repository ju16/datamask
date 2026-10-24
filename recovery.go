package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RecoveryLog 脱敏恢复日志
type RecoveryLog struct {
	Version           string              `json:"version"`
	CreatedAt         string              `json:"created_at"`
	SourceFile        string              `json:"source_file"`
	MaskedFile        string              `json:"masked_file"`
	RulesApplied      []string            `json:"rules_applied"`
	TotalReplacements int                 `json:"total_replacements"`
	Replacements      []ReplacementRecord `json:"replacements"`
}

// NewRecoveryLog 创建恢复日志
func NewRecoveryLog(sourceFile, maskedFile string, rulesApplied []string, replacements []ReplacementRecord) *RecoveryLog {
	return &RecoveryLog{
		Version:           "1.0",
		CreatedAt:         time.Now().Format("2006-01-02T15:04:05"),
		SourceFile:        sourceFile,
		MaskedFile:        maskedFile,
		RulesApplied:      rulesApplied,
		TotalReplacements: len(replacements),
		Replacements:      replacements,
	}
}

// Save 保存日志到文件
func (l *RecoveryLog) Save(path string) (string, error) {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化日志失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入日志失败 %s: %v", path, err)
	}
	return path, nil
}

// LoadRecoveryLog 从文件加载恢复日志
func LoadRecoveryLog(path string) (*RecoveryLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("日志文件不存在: %s", path)
	}
	var l RecoveryLog
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("日志文件解析失败: %v", err)
	}
	// 兼容无 part 字段的旧日志
	for i := range l.Replacements {
		if l.Replacements[i].Part == "" {
			l.Replacements[i].Part = "word/document.xml"
		}
	}
	return &l, nil
}

// Summary 返回日志摘要
func (l *RecoveryLog) Summary() string {
	var b strings.Builder
	line := strings.Repeat("=", 50)
	sub := strings.Repeat("-", 50)
	b.WriteString(line + "\n")
	b.WriteString("脱敏日志摘要\n")
	b.WriteString("  源文件: " + l.SourceFile + "\n")
	b.WriteString("  脱敏文件: " + l.MaskedFile + "\n")
	b.WriteString("  创建时间: " + l.CreatedAt + "\n")
	b.WriteString("  应用规则: " + strings.Join(l.RulesApplied, ", ") + "\n")
	b.WriteString("  替换总数: " + fmt.Sprint(l.TotalReplacements) + "\n")
	b.WriteString(sub + "\n")
	for i, r := range l.Replacements {
		b.WriteString(fmt.Sprintf("  [%d] 规则=%s, 段落=%d, '%s' -> '%s'\n",
			i+1, r.Rule, r.ParagraphIndex, r.Original, r.Masked))
	}
	b.WriteString(line)
	return b.String()
}

// defaultLogPath 默认日志路径：脱敏文件同目录 .masklog.json
func defaultLogPath(maskedPath string) string {
	ext := filepath.Ext(maskedPath)
	return strings.TrimSuffix(maskedPath, ext) + ".masklog.json"
}
