---
name: datamask
description: 对 .docx 文档中的敏感信息进行脱敏与可逆恢复。当用户要求脱敏 Word 文档、隐藏/打码身份证号、手机号、银行卡号等敏感信息，或按恢复日志还原脱敏文档时，使用本技能。
agent_created: true
---

# DataMask 脱敏工具调用指南

DataMask 是一个 .docx 敏感信息脱敏 CLI（身份证、银行卡、护照、手机号、邮箱、固话、IP、统一社会信用代码等 16 类规则，可配置）。脱敏会生成 JSON 恢复日志，凭日志可完整还原原文。

## 第一步：定位可执行文件

按以下顺序查找 datamask 二进制，找到即用：

1. 用户消息中给出的路径（优先）
2. Windows：`datamask.exe` / `datamask-win-x64.exe`；macOS：`datamask` / `datamask-macos-x86_64` / `datamask-macos-arm64`
3. 常见位置：当前项目 `dist/` 目录、用户 Downloads 目录、系统 PATH
4. 都找不到时，询问用户二进制位置，并提示可从 https://github.com/ju16/datamask/releases 下载

在 Git Bash / WSL 中调用 Windows 二进制用 `./datamask-windows.exe` 形式；路径含空格时加引号。

## 第二步：执行对应操作

### 脱敏单个文件

```bash
<bin> mask <input.docx> -o <output.docx> -l <log.json>
```

- `-o` 省略时默认输出 `原名_masked.docx`；`-l` 省略时默认 `同名.masklog.json`
- `-r <规则文件>` 可指定自定义规则文件（省略用内置规则）

### 脱敏批量文件

遍历目标目录下全部 `.docx`，逐个执行 mask，日志与脱敏文件放同一输出目录：

```bash
<bin> mask "in_dir/文件1.docx" -o "out_dir/文件1_masked.docx" -l "out_dir/文件1.masklog.json"
<bin> mask "in_dir/文件2.docx" -o "out_dir/文件2_masked.docx" -l "out_dir/文件2.masklog.json"
```

用脚本循环实现（如 `for f in in_dir/*.docx; do ...`），不要逐个手敲。完成后汇报：处理文件数、各文件替换处数。

### 恢复单个文件

```bash
<bin> restore <log.json> -o <restored.docx>
```

- 日志内记录了源文件与脱敏文件路径；若原位置文件已变动，先确认再执行
- 省略 `-o` 时默认输出 `原名_restored.docx`

### 恢复批量文件

遍历目录下全部 `.masklog.json`，逐个执行 restore，输出到指定目录。同样用循环脚本实现。

### 查看日志摘要

```bash
<bin> log-show <log.json>
```

展示每条替换的规则、段落、原文、掩码结果。用户问"脱敏了哪些内容"时使用。

### 查看当前规则

```bash
<bin> rules              # 内置规则
<bin> rules -r <规则文件>  # 检查自定义规则文件是否合法
```

列出全部规则（名称、正则、替换式、启用状态）。

### 自定义规则文件

用户想增删改脱敏规则时：

1. `datamask rules-init [文件]` 导出内置规则为可编辑的纯文本规则文件（默认 `datamask-rules.txt`）
2. 提醒用户用编辑器修改（或按用户口述代为修改）。文件格式：每条规则一行顶格写名称，下面缩进写 `查找:` / `替换:` / `说明:` / `开关:`，文件头有完整格式说明
3. 修改后先 `<bin> rules -r <文件>` 验证合法性，再 `<bin> mask <input.docx> -r <文件>` 使用

注意：规则文件由用户所有，不要覆盖已存在的规则文件。

## 关键约束

- 隐私红线：只接收用户的**文件路径**，调用 CLI 在本地处理；严禁把 .docx 内容读取进对话上下文（包括用 Read 工具打开二进制文档、或要求用户把文件拖入对话框附件）。用户主动粘贴文档内容时，提醒其改用路径
- 恢复日志是还原的唯一依据，脱敏后必须提醒用户妥善保管 `<name>.masklog.json`，丢失则无法还原
- 脱敏不可原地进行：始终输出到新文件，不覆盖用户原始文档
- 日志与脱敏文档分离存放时（如日志在本地、文档发给他人），恢复时需要原文档+日志双方都在
- 批量处理前若输出目录不存在，先创建
- 执行完毕后，从日志统计或 log-show 结果中汇总汇报替换总数
