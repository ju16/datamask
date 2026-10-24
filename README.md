# DataMask

对 `.docx` 文档中的敏感信息进行脱敏处理的命令行工具。**单文件二进制，零运行时依赖，跨平台**。

## 特性

- **纯标准库实现 docx 处理**：直接解析 zip + OOXML，不依赖任何第三方库
- **正则规则引擎**：内置身份证、银行卡、护照、手机号、邮箱、固话、IP、统一社会信用代码等 16 类规则，`config.yaml` 可自定义增删
- **全文覆盖**：正文、页眉、页脚、脚注、尾注中的段落全部处理（含表格内文字），敏感信息跨多个文本 run 也能正确掩码
- **可逆恢复**：脱敏生成 JSON 恢复日志，`restore` 一键还原（按应用顺序反序恢复，支持位置偏移自愈）
- **XML 安全**：完整的实体转义/还原，保留 `xml:space="preserve"`，不破坏文档结构

## 下载

预编译二进制（纯静态，无运行时依赖，下载即用）：

| 平台 | 架构 | 文件 | 大小 |
|---|---|---|---|
| Windows | 64 位 | [datamask-windows.exe](https://github.com/ju16/datamask/releases/download/v1.1.0/datamask-windows.exe) | ~3.0 MB |
| macOS | Intel | [datamask-macos-intel](https://github.com/ju16/datamask/releases/download/v1.1.0/datamask-macos-intel) | ~3.0 MB |
| macOS | Apple Silicon | [datamask-macos-apple-silicon](https://github.com/ju16/datamask/releases/download/v1.1.0/datamask-macos-apple-silicon) | ~2.8 MB |

不确定自己的 Mac 是哪种架构？点击屏幕左上角苹果菜单 → 关于本机：显示"Intel 芯片"选 Intel，显示"Apple M1/M2/M3/M4"选 Apple Silicon。

Windows 用户下载 `.exe` 直接双击或在命令行调用；macOS 用户下载后先执行 `chmod +x datamask-macos-*` 再加权限运行。

全部版本见 [Releases 页面](https://github.com/ju16/datamask/releases)。

## 从源码构建

```bash
go build -o datamask .
```

## 使用

```bash
# 脱敏文档
datamask mask input.docx -o output.docx -l recovery.masklog.json

# 根据日志恢复原文
datamask restore recovery.masklog.json -o restored.docx

# 查看日志摘要
datamask log-show recovery.masklog.json

# 查看当前生效的脱敏规则
datamask rules

# 使用自定义配置（默认用内置规则）
datamask mask input.docx -c my-config.yaml

# 版本
datamask version
```

不带 `-o` 时默认输出 `原名_masked.docx` / `原名_restored.docx`，日志默认 `同名.masklog.json`。

## 跨平台编译

```bash
# Windows 64 位
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/datamask-windows.exe .

# macOS（Intel + Apple Silicon）
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o dist/datamask-macos-intel .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/datamask-macos-apple-silicon .
```

`CGO_ENABLED=0` 产出纯静态二进制，丢到任何同架构机器上直接跑。一键构建见 `build.sh`。

## 规则配置

`config.yaml` 定义全部规则，加载时自动完成转义换算（`\uXXXX` → RE2 写法、`\1` → `${1}`），书写时按常规正则习惯写即可：

```yaml
version: "2.0"
rules:
  - name: id_card
    description: 中国大陆18位身份证号（保留前6后4）
    pattern: \b(\d{6})\d{8}(\d{3}[\dXx])\b
    replacement: \1********\2
    enabled: true
```

匹配优先级从上到下：先具体规则（身份证），后宽泛规则（银行卡）。`enabled: false` 可单独禁用某条规则。

## 恢复日志格式

```json
{
  "version": "1.0",
  "created_at": "2026-10-24T00:31:13",
  "source_file": "contract.docx",
  "masked_file": "masked.docx",
  "rules_applied": ["id_card", "phone", "..."],
  "total_replacements": 11,
  "replacements": [
    {
      "rule": "id_card",
      "part": "word/document.xml",
      "paragraph_index": 2,
      "start": 6,
      "end": 24,
      "original": "110101199001011234",
      "masked": "110101********1234"
    }
  ]
}
```

## 测试

```bash
go test ./...
```

覆盖：全类别数据脱敏/恢复闭环、跨 run 段落、XML 实体与 preserve、规则转换、恢复位置偏移策略、日志向后兼容。

## 项目结构

```
datamask/
├── main.go        # CLI 入口（mask/restore/log-show/rules）
├── config.go      # 配置加载 + 规则引擎
├── docx.go        # docx 读写（zip + OOXML 扫描）
├── recovery.go    # JSON 恢复日志
├── config.yaml    # 内置脱敏规则（go:embed 嵌入二进制）
└── main_test.go   # 端到端测试
```
