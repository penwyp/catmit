# catmit

`catmit` 是一款跨平台 Golang CLI/TUI 工具，自动生成符合 Conventional Commits 规范的高质量 Git 提交信息，支持多语言输出（默认英语，可 `--lang zh`）。

## 快速开始

```bash
# 安装（待发布 Homebrew Tap）
brew install penwyp/catmit/catmit

# 环境变量
export DEEPSEEK_API_KEY=sk-xxxx   # 必填，用户提供 DeepSeek API Key
# 可选自定义 API 基础地址
# export DEEPSEEK_API_BASE_URL=https://internal-proxy.example.com

# 在有修改的仓库目录下运行
catmit                # 交互式确认
catmit -y             # 无确认直接提交
catmit --dry-run      # 仅打印消息，不提交
```

## 核心特性

* 收集最近 10 条提交记录与当前 diff
* DeepSeek LLM 生成符合 Conventional Commits 的消息
* `Bubble Tea` 进度 / 预览 TUI
* `--yes`、`--dry-run`、`--timeout` 等常用 CLI 选项
* 静态编译，macOS/Linux (amd64/arm64) 一键运行

## 本地开发

```bash
# 安装依赖
make build      # 构建可执行文件
make test       # 运行全部单元/集成/E2E 测试
make lint       # golangci-lint
make e2e        # 仅运行 E2E
```

## 发布

项目使用 [goreleaser](https://goreleaser.com) 自动发布，打 tag 即可触发 GitHub Actions。

```bash
# 首次本地验证
make release    # --snapshot 本地包
```

## 贡献

欢迎提 PR！请遵循 `golangci-lint` 及 Conventional Commits 规范提交信息。 