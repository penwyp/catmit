<div align="center">
  <img src="catmit.png" alt="catmit logo" width="200" height="200">
  
  # 🐱 catmit
  
  **AI 驱动的 Git 提交信息生成器**
  
  [![Go Report Card](https://goreportcard.com/badge/github.com/penwyp/catmit)](https://goreportcard.com/report/github.com/penwyp/catmit)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
  [![Release](https://img.shields.io/github/release/penwyp/catmit.svg)](https://github.com/penwyp/catmit/releases)
  [![Go Version](https://img.shields.io/github/go-mod/go-version/penwyp/catmit)](https://golang.org/doc/devel/release.html)

  *再也不用为提交信息发愁！让 AI 为你生成完美的规范化提交信息。*
  
  [English](README.md) | 中文
</div>

## ✨ 特性

- 🤖 **AI 驱动**: 使用 DeepSeek LLM 分析你的代码变更并生成有意义的提交信息
- 📝 **规范化提交**: 遵循 Conventional Commits 格式，包含合适的类型、范围和描述
- 🎨 **精美 TUI**: 交互式终端界面，实时进度指示器
- 🌍 **多语言支持**: 支持中文和英文输出
- ⚡ **快速可靠**: 使用 Go 构建，具有强大的错误处理和超时支持
- 🔧 **灵活使用**: 支持交互式和自动化（CI/CD）模式
- 📊 **智能分析**: 分析 git 历史、文件变更和仓库上下文
- 🎯 **高准确率**: 生成上下文相关的提交信息，质量达 95% 以上

## 🚀 快速开始

### 安装

#### 使用 Homebrew (macOS/Linux)
```bash
brew tap penwyp/catmit
brew install catmit
```

#### 使用 Go
```bash
go install github.com/penwyp/catmit@latest
```

#### 下载二进制文件
从 [GitHub Releases](https://github.com/penwyp/catmit/releases) 下载适合你平台的最新版本。

### 配置

1. **获取 DeepSeek API 密钥** 从 [DeepSeek 控制台](https://platform.deepseek.com/api_keys)

2. **设置环境变量：**
   ```bash
   export CATMIT_LLM_API_KEY="sk-your-api-key-here"
   ```

3. **做一些修改并暂存：**
   ```bash
   git add .
   ```

4. **生成并提交：**
   ```bash
   catmit
   ```

## 📖 使用方法

### 基本用法
```bash
# 交互式模式，带 TUI
catmit

# 无需确认直接提交
catmit -y

# 仅预览信息（试运行）
catmit --dry-run

# 生成中文提交信息
catmit -l zh

# 设置自定义超时时间（默认：30秒）
catmit -t 60

# 提供种子文本以获得更好的上下文
catmit "修复用户认证"
```

### 高级用法
```bash
# 自定义 API 端点
export CATMIT_LLM_API_URL="https://your-api-endpoint.com"

# 静默模式（无 TUI，直接输出）
catmit --dry-run -y

# 获取帮助
catmit --help

# 查看版本
catmit --version
```

## 🏗️ 工作原理

1. **分析仓库**: 扫描最近的提交、分支信息和当前变更
2. **构建上下文**: 使用文件变更、提交历史和模式创建丰富的提示
3. **AI 生成**: 将上下文发送给 DeepSeek LLM 进行智能信息生成
4. **质量保证**: 验证规范化提交格式并提供审查界面
5. **智能提交**: 使用生成的信息执行 git commit

## 🎯 输出示例

### 之前（手动）
```bash
git commit -m "修复bug"
git commit -m "更新东西"
git commit -m "变更"
```

### 之后（catmit）
```bash
fix(auth): 解决令牌验证竞态条件

- 添加互斥锁防止并发令牌刷新
- 更新过期令牌的错误处理
- 改进边缘情况的测试覆盖率

Closes #123
```

## 🛠️ 开发

### 前置要求
- Go 1.22+
- Git
- DeepSeek API 密钥

### 从源码构建
```bash
git clone https://github.com/penwyp/catmit.git
cd catmit
make build
```

### 运行测试
```bash
# 运行所有测试
make test

# 运行覆盖率测试
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# 运行端到端测试
make e2e

# 代码检查
make lint
```

### 项目结构
```
catmit/
├── client/         # DeepSeek API 客户端
├── collector/      # Git 操作和数据收集
├── cmd/           # Cobra CLI 命令和依赖注入
├── prompt/        # 提示模板构建器
├── ui/           # Bubble Tea TUI 组件
├── test/e2e/     # 端到端测试
└── docs/         # 文档
```

## 🔧 配置

### 环境变量
| 变量 | 描述 | 默认值 |
|------|------|--------|
| `CATMIT_LLM_API_KEY` | DeepSeek API 密钥（必需） | - |
| `CATMIT_LLM_API_URL` | 自定义 API 端点 | `https://api.deepseek.com` |

### 退出代码
| 代码 | 含义 |
|------|------|
| `0` | 成功 |
| `1` | 一般错误 |
| `124` | 超时 |

## 🤝 贡献

我们欢迎贡献！详情请查看我们的 [贡献指南](CONTRIBUTING.md)。

1. Fork 仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 进行更改并添加测试
4. 确保测试通过 (`make test`)
5. 使用 catmit 提交 (`catmit`)
6. 推送到分支 (`git push origin feature/amazing-feature`)
7. 开启 Pull Request

## 🐛 故障排除

### 常见问题

**API 密钥问题：**
```bash
# 验证 API 密钥是否设置
echo $CATMIT_LLM_API_KEY

# 测试 API 连接
catmit --dry-run
```

**没有暂存变更：**
```bash
# 确保有暂存的变更
git status
git add .
```

**超时问题：**
```bash
# 增加超时时间
catmit -t 60
```

如需更多帮助，请查看我们的 [Issues](https://github.com/penwyp/catmit/issues) 或创建新的问题。

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

- [DeepSeek](https://www.deepseek.com/) 提供 AI 能力
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) 提供出色的 TUI 框架
- [Cobra](https://github.com/spf13/cobra) 提供 CLI 框架
- [Conventional Commits](https://www.conventionalcommits.org/) 提供提交标准

## ⭐ Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=penwyp/catmit&type=Date)](https://star-history.com/#penwyp/catmit&Date)

---

<div align="center">
  用 ❤️ 制作，作者 <a href="https://github.com/penwyp">penwyp</a>
  
  如果 catmit 对你有帮助，请考虑给它一个 ⭐！
</div>