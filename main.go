package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed config.yaml
var embeddedConfig []byte

var version = "1.2.0"

const defaultConfigName = "config.yaml"
const defaultRulesTxtName = "datamask-rules.txt"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "mask":
		os.Exit(cmdMask(os.Args[2:]))
	case "restore":
		os.Exit(cmdRestore(os.Args[2:]))
	case "log-show":
		os.Exit(cmdLogShow(os.Args[2:]))
	case "rules":
		os.Exit(cmdRules(os.Args[2:]))
	case "rules-init":
		os.Exit(cmdRulesInit(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("DataMask v%s\n", version)
		os.Exit(0)
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "[错误] 未知命令: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`DataMask - 数据脱敏工具 v` + version + `

用法:
  datamask mask <input.docx> [-o output] [-l log] [-r rules]   对文档执行脱敏
  datamask restore <log.json> [-o output]                      根据日志恢复文档
  datamask log-show <log.json>                                 查看脱敏日志摘要
  datamask rules [-r rules]                                    查看当前脱敏规则
  datamask rules-init [输出文件]                                导出内置规则为可编辑的规则文件
  datamask version                                             查看版本

选项:
  -o, --output   输出文件路径
  -l, --log      恢复日志文件路径
  -r, --rules    规则文件路径，一行一条规则、纯文本好编辑（也兼容 YAML 配置）
  -c, --config   同 -r（兼容旧写法）

示例:
  datamask mask contract.docx
  datamask mask contract.docx -o out.docx -l out.masklog.json
  datamask rules-init                          导出内置规则到 datamask-rules.txt
  datamask mask contract.docx -r my-rules.txt  用自定义规则脱敏
  datamask restore out.masklog.json
  datamask rules -r my-rules.txt               检查自定义规则
`)
}

// resolveConfig 解析配置文件：优先外部文件，否则用内置配置
func resolveConfig(configPath string) (string, error) {
	if configPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return "", fmt.Errorf("配置文件不存在: %s", configPath)
		}
		return configPath, nil
	}
	// 当前目录有 config.yaml 则优先使用，否则落地内置配置到临时文件
	local := defaultConfigName
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	tmp := filepath.Join(os.TempDir(), "datamask-embedded-config.yaml")
	if err := os.WriteFile(tmp, embeddedConfig, 0644); err != nil {
		return "", fmt.Errorf("写入内置配置失败: %v", err)
	}
	return tmp, nil
}

// parseFlags 简易参数解析：支持 -o value / --output value / -o=value
func parseFlags(args []string) (positional []string, opts map[string]string, err error) {
	opts = make(map[string]string)
	aliases := map[string]string{
		"-o": "output", "--output": "output",
		"-l": "log", "--log": "log",
		"-c": "config", "--config": "config",
		"-r": "config", "--rules": "config",
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			name := a
			value := ""
			hasValue := false
			if eq := strings.IndexByte(a, '='); eq >= 0 {
				name = a[:eq]
				value = a[eq+1:]
				hasValue = true
			}
			key, ok := aliases[name]
			if !ok {
				return nil, nil, fmt.Errorf("未知选项: %s", name)
			}
			if !hasValue {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("选项 %s 缺少参数", name)
				}
				i++
				value = args[i]
			}
			opts[key] = value
		} else {
			positional = append(positional, a)
		}
	}
	return positional, opts, nil
}

func cmdMask(args []string) int {
	positional, opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "[错误] mask 需要输入文件参数")
		return 1
	}
	input := positional[0]
	if _, err := os.Stat(input); err != nil {
		fmt.Fprintf(os.Stderr, "[错误] 输入文件不存在: %s\n", input)
		return 1
	}

	configPath, err := resolveConfig(opts["config"])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}

	fmt.Printf("[信息] 正在脱敏: %s\n", input)
	result, err := MaskFile(input, opts["output"], configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}

	logPath := opts["log"]
	if logPath == "" {
		logPath = defaultLogPath(result.MaskedFile)
	}
	log := NewRecoveryLog(result.SourceFile, result.MaskedFile, result.RulesApplied, result.Replacements)
	if _, err := log.Save(logPath); err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}

	fmt.Printf("[完成] 脱敏文件: %s\n", result.MaskedFile)
	fmt.Printf("[完成] 恢复日志: %s\n", logPath)
	fmt.Printf("[统计] 共替换 %d 处\n", result.TotalReplacements)
	return 0
}

func cmdRestore(args []string) int {
	positional, opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "[错误] restore 需要日志文件参数")
		return 1
	}
	logPath := positional[0]

	log, err := LoadRecoveryLog(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	fmt.Printf("[信息] 加载日志: %s\n", logPath)
	fmt.Printf("[信息] 源文件: %s\n", log.SourceFile)
	fmt.Printf("[信息] 脱敏文件: %s\n", log.MaskedFile)

	if err := RestoreFile(logPath, opts["output"]); err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	output := opts["output"]
	if output == "" {
		ext := filepath.Ext(log.MaskedFile)
		output = strings.TrimSuffix(log.MaskedFile, ext) + "_restored" + ext
	}
	fmt.Printf("[完成] 恢复文件: %s\n", output)
	fmt.Printf("[统计] 恢复 %d 处替换\n", log.TotalReplacements)
	return 0
}

func cmdLogShow(args []string) int {
	positional, _, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "[错误] log-show 需要日志文件参数")
		return 1
	}
	log, err := LoadRecoveryLog(positional[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	fmt.Println(log.Summary())
	return 0
}

func cmdRules(args []string) int {
	positional, opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	_ = positional
	configPath, err := resolveConfig(opts["config"])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	line := strings.Repeat("=", 50)
	verLabel := cfg.Version()
	if verLabel == "text" {
		verLabel = "自定义规则文件"
	} else {
		verLabel = "版本 " + verLabel
	}
	fmt.Println(line)
	fmt.Printf("DataMask 脱敏规则 (%s)\n", verLabel)
	fmt.Println(line)
	for _, rule := range cfg.AllRules() {
		status := "启用"
		if !rule.Enabled {
			status = "禁用"
		}
		fmt.Printf("\n[%s] %s\n", status, rule.Name)
		fmt.Printf("  描述: %s\n", rule.Description)
		fmt.Printf("  正则: %s\n", rule.RawPattern)
		fmt.Printf("  替换: %s\n", rule.RawReplacement)
	}
	fmt.Println(line)
	return 0
}

// cmdRulesInit 导出内置规则为可编辑的纯文本规则文件
func cmdRulesInit(args []string) int {
	positional, opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		return 1
	}
	out := defaultRulesTxtName
	if len(positional) >= 1 {
		out = positional[0]
	}
	if opts["output"] != "" {
		out = opts["output"]
	}
	if _, err := os.Stat(out); err == nil {
		fmt.Fprintf(os.Stderr, "[错误] 文件已存在: %s（如需覆盖请先删除或指定其他路径）\n", out)
		return 1
	}
	_, raws, err := parseYAMLRules(embeddedConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[错误] 解析内置规则失败: %v\n", err)
		return 1
	}
	if err := os.WriteFile(out, renderRulesTxt(raws), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[错误] 写入规则文件失败: %v\n", err)
		return 1
	}
	fmt.Printf("[完成] 已导出 %d 条内置规则到: %s\n", len(raws), out)
	fmt.Println("[提示] 用任意编辑器打开修改（一行一条规则，文件头有格式说明），然后执行：")
	fmt.Printf("       datamask rules -r %s       ← 先检查规则是否合法\n", out)
	fmt.Printf("       datamask mask 文档.docx -r %s\n", out)
	return 0
}
