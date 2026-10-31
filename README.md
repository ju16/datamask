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
| Windows | 64 位 | [datamask-win-x64.exe](https://github.com/ju16/datamask/releases/download/v1.1.0/datamask-win-x64.exe) | ~3.0 MB |
| macOS | Intel | [datamask-macos-x86_64](https://github.com/ju16/datamask/releases/download/v1.1.0/datamask-macos-x86_64) | ~3.0 MB |
| macOS | Apple Silicon | [datamask-macos-arm64](https://github.com/ju16/datamask/releases/download/v1.1.0/datamask-macos-arm64) | ~2.8 MB |

不确定自己的 Mac 是哪种架构？点击屏幕左上角苹果菜单 → 关于本机：显示"Intel 芯片"选 Intel，显示"Apple M1/M2/M3/M4"选 Apple Silicon。

下载后按平台处理一下即可在命令行使用（下文命令中的 `datamask` 均指处理后的可执行文件）：

**Windows**：将下载的 `datamask-win-x64.exe` 重命名为 `datamask.exe`，把所在目录加入 PATH（设置 → 搜索"编辑系统环境变量" → 环境变量 → Path → 新建），之后新开终端即可直接使用 `datamask` 命令。嫌麻烦也可以不配置，调用时写全路径，如 `D:\Downloads\datamask-win-x64.exe mask input.docx`。

**macOS**：先执行 `chmod +x datamask-macos-*` 赋予执行权限，可重命名为 `datamask` 后用全路径调用，如 `~/Downloads/datamask-macos-arm64 mask input.docx`；想直接使用 `datamask` 命令就 `sudo mv ~/Downloads/datamask-macos-arm64 /usr/local/bin/datamask`。

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

# 导出内置规则为可编辑文件，改完再用
datamask rules-init
datamask mask input.docx -r datamask-rules.txt

# 版本
datamask version
```

不带 `-o` 时默认输出 `原名_masked.docx` / `原名_restored.docx`，日志默认 `同名.masklog.json`。

## 在 WorkBuddy 中使用

命令行之外，也可以直接在 WorkBuddy 里用自然语言驱动 DataMask。

**首次使用**：新开一个 WorkBuddy 对话，把下面这条消息整个发出去——下载二进制和安装技能一次完成：

```text
装一下 datamask：从 https://github.com/ju16/datamask/releases 下载适合我这台电脑的版本放好，并把 https://github.com/ju16/datamask/tree/main/skills/datamask 安装为 skill。
```

之后在任何对话里直接说"脱敏"即可，无需再提工具名。以下提示词可直接复制使用：

> **隐私提醒**：始终把**文件路径**发给 WorkBuddy（如 "把 `D:\Docs\合同.docx` 脱敏"），由它调用命令行在本地完成处理。不要把文件本身拖进对话框——附件内容会进入对话上下文，失去脱敏的意义。对隐私安全要求极高的场景，建议直接按上方命令行用法手工执行。

### 脱敏

单个文件：

```text
帮我把 D:\Docs\合同.docx 脱敏，输出到 D:\Docs\合同_masked.docx，并生成恢复日志。
```

批量文件：

```text
把 D:\Contracts 文件夹里所有的 .docx 全部脱敏，脱敏结果和恢复日志统一放到 D:\Contracts\masked\ 目录。
```

### 恢复

单个文件：

```text
按 D:\Docs\合同.masklog.json 恢复原文，输出到 D:\Docs\合同_restored.docx。
```

批量文件：

```text
把 D:\Contracts\masked\ 目录下所有 .masklog.json 批量恢复成原文 docx。
```

提示词不指定输出位置时，按命令行默认规则处理（`原名_masked.docx` / `同名.masklog.json`）。恢复日志务必妥善保管——没有它，脱敏后的文档无法还原。

## 跨平台编译

```bash
# Windows 64 位
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/datamask-win-x64.exe .

# macOS（Intel + Apple Silicon）
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o dist/datamask-macos-x86_64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/datamask-macos-arm64 .
```

`CGO_ENABLED=0` 产出纯静态二进制，丢到任何同架构机器上直接跑。一键构建见 `build.sh`。

## 自定义规则

规则文件为纯文本格式，一行一条规则、字段一目了然，用任意编辑器就能改。`rules-init` 一键导出内置规则作为起点：

```bash
datamask rules-init                    # 导出内置规则到 datamask-rules.txt
datamask rules -r datamask-rules.txt   # 检查修改后的规则是否合法
datamask mask input.docx -r datamask-rules.txt
```

规则写法（每条规则：一行顶格写名称，下面缩进的行写字段）：

```text
手机号
  说明: 中国手机号，保留前3后4
  查找: \b(1[3-9]\d)\d{4}(\d{4})\b
  替换: \1****\2

银行卡号
  查找: \b(\d{4})\d{8,11}(\d{4})\b
  替换: \1********\2

内网IP
  查找: \b(\d{1,3})\.\d{1,3}\.\d{1,3}\.\d{1,3}\b
  开关: 关
```

- `查找`：正则表达式，与常见写法一致（`\d` 数字、`a|b` 二选一、`\u4e00` 中文也支持），必填
- `替换`：`\1`、`\2` 表示保留对应括号捕获的内容，`*` 为掩码字符；省略时整段匹配替换为 `***`
- `说明`：备注，给人看的，不影响执行
- `开关`：`开` / `关`，省略默认开；临时停用某条规则只改这一个词
- 以 `#` 开头的行为注释；规则按文件顺序生效，越具体的规则放越前面（如身份证在银行卡之前）

也兼容 YAML 配置格式，`-r` 与 `-c` 等价，加载时按内容自动识别：

```yaml
version: "2.0"
rules:
  - name: id_card
    description: 中国大陆18位身份证号（保留前6后4）
    pattern: \b(\d{6})\d{8}(\d{3}[\dXx])\b
    replacement: \1********\2
    enabled: true
```

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
├── main.go        # CLI 入口（mask/restore/log-show/rules/rules-init）
├── config.go      # 配置加载 + 规则引擎
├── rulestxt.go    # 纯文本规则文件解析/导出
├── docx.go        # docx 读写（zip + OOXML 扫描）
├── recovery.go    # JSON 恢复日志
├── config.yaml    # 内置脱敏规则（go:embed 嵌入二进制）
└── main_test.go   # 端到端测试
```
