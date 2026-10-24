package main

import (
	"path/filepath"
	"strings"
)

// MaskFile 对文档执行脱敏，返回脱敏结果
func MaskFile(inputPath, outputPath, configPath string) (*MaskResult, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		outputPath = strings.TrimSuffix(inputPath, ext) + "_masked" + ext
	}

	doc, err := OpenDocx(inputPath)
	if err != nil {
		return nil, err
	}

	rules := cfg.Rules()
	ruleNames := make([]string, len(rules))
	for i, r := range rules {
		ruleNames[i] = r.Name
	}

	var all []ReplacementRecord
	// Python 语义：逐段落，段内按规则顺序依次应用。
	// 这里按部件/段落循环，段内按规则序应用，保证替换记录顺序与 Python 一致。
	all, err = doc.Mask(func(part string, paraIdx int, text string) (string, []ReplacementRecord) {
		var records []ReplacementRecord
		current := text
		for _, rule := range rules {
			masked, reps := rule.Apply(current, part, paraIdx)
			if len(reps) > 0 {
				records = append(records, reps...)
				current = masked
			}
		}
		return current, records
	})
	if err != nil {
		return nil, err
	}

	if err := doc.Save(outputPath); err != nil {
		return nil, err
	}

	return &MaskResult{
		SourceFile:        inputPath,
		MaskedFile:        outputPath,
		RulesApplied:      ruleNames,
		TotalReplacements: len(all),
		Replacements:      all,
	}, nil
}

// MaskResult 脱敏结果
type MaskResult struct {
	SourceFile        string
	MaskedFile        string
	RulesApplied      []string
	TotalReplacements int
	Replacements      []ReplacementRecord
}

// RestoreFile 根据日志恢复文档
func RestoreFile(logPath, outputPath string) error {
	log, err := LoadRecoveryLog(logPath)
	if err != nil {
		return err
	}
	doc, err := OpenDocx(log.MaskedFile)
	if err != nil {
		return err
	}
	if err := doc.Restore(log.Replacements); err != nil {
		return err
	}
	if outputPath == "" {
		ext := filepath.Ext(log.MaskedFile)
		outputPath = strings.TrimSuffix(log.MaskedFile, ext) + "_restored" + ext
	}
	return doc.Save(outputPath)
}
