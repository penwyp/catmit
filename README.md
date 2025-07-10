<div align="center">
  <img src="catmit.png" alt="catmit logo" width="200" height="200">
  
  # 🐱 catmit
  
  **AI-Powered Git Commit Message Generator**
  
  [![Go Report Card](https://goreportcard.com/badge/github.com/penwyp/catmit)](https://goreportcard.com/report/github.com/penwyp/catmit)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
  [![Release](https://img.shields.io/github/release/penwyp/catmit.svg)](https://github.com/penwyp/catmit/releases)
  [![Go Version](https://img.shields.io/github/go-mod/go-version/penwyp/catmit)](https://golang.org/doc/devel/release.html)

  *Never struggle with commit messages again! Let AI craft perfect conventional commits for you.*
</div>

## ✨ Features

- 🤖 **AI-Powered**: Uses DeepSeek LLM to analyze your changes and generate meaningful commit messages
- 📝 **Conventional Commits**: Follows conventional commit format with proper type, scope, and description
- 🎨 **Beautiful TUI**: Interactive terminal interface with real-time progress indicators
- 🌍 **Multi-Language**: Supports both English and Chinese output
- ⚡ **Fast & Reliable**: Built in Go with robust error handling and timeout support
- 🔧 **Flexible Usage**: Works in both interactive and automated (CI/CD) modes
- 📊 **Smart Analysis**: Analyzes git history, file changes, and repository context
- 🎯 **High Accuracy**: Generates contextually relevant commit messages with >95% quality

## 🚀 Quick Start

### Installation

#### Using Homebrew (macOS/Linux)
```bash
brew tap penwyp/catmit
brew install catmit
```

#### Using Go
```bash
go install github.com/penwyp/catmit@latest
```

#### Download Binary
Download the latest release from [GitHub Releases](https://github.com/penwyp/catmit/releases) for your platform.

### Setup

1. **Get your DeepSeek API key** from [DeepSeek Console](https://platform.deepseek.com/api_keys)

2. **Set environment variable:**
   ```bash
   export CATMIT_LLM_API_KEY="sk-your-api-key-here"
   ```

3. **Make some changes and stage them:**
   ```bash
   git add .
   ```

4. **Generate and commit:**
   ```bash
   catmit
   ```

## 📖 Usage

### Basic Usage
```bash
# Interactive mode with TUI
catmit

# Auto-commit without confirmation
catmit -y

# Preview message only (dry run)
catmit --dry-run

# Generate in Chinese
catmit -l zh

# Set custom timeout (default: 30s)
catmit -t 60

# Provide seed text for better context
catmit "fix user authentication"
```

### Advanced Usage
```bash
# Custom API endpoint
export CATMIT_LLM_API_URL="https://your-api-endpoint.com"

# Silent mode (no TUI, direct output)
catmit --dry-run -y

# Get help
catmit --help

# Check version
catmit --version
```

## 🏗️ How It Works

1. **Analyze Repository**: Scans recent commits, branch info, and current changes
2. **Context Building**: Creates rich prompts with file changes, commit history, and patterns
3. **AI Generation**: Sends context to DeepSeek LLM for intelligent message generation
4. **Quality Assurance**: Validates conventional commit format and provides review interface
5. **Smart Commit**: Executes git commit with the generated message

## 🎯 Example Output

### Before (manual)
```bash
git commit -m "fix bug"
git commit -m "update stuff"
git commit -m "changes"
```

### After (catmit)
```bash
fix(auth): resolve token validation race condition

- Add mutex to prevent concurrent token refresh
- Update error handling for expired tokens  
- Improve test coverage for edge cases

Closes #123
```

## 🛠️ Development

### Prerequisites
- Go 1.22+
- Git
- DeepSeek API key

### Building from Source
```bash
git clone https://github.com/penwyp/catmit.git
cd catmit
make build
```

### Running Tests
```bash
# Run all tests
make test

# Run with coverage
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# Run E2E tests
make e2e

# Lint code
make lint
```

### Project Structure
```
catmit/
├── client/         # DeepSeek API client
├── collector/      # Git operations and data collection
├── cmd/           # Cobra CLI commands with dependency injection
├── prompt/        # Prompt template builder
├── ui/           # Bubble Tea TUI components
├── test/e2e/     # End-to-end tests
└── docs/         # Documentation
```

## 🔧 Configuration

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `CATMIT_LLM_API_KEY` | DeepSeek API key (required) | - |
| `CATMIT_LLM_API_URL` | OpenAI-compatible API endpoint | `https://api.deepseek.com/v1/chat/completions` |
| `CATMIT_LLM_MODEL`   | Model name used for completion | `deepseek-chat` |

### Exit Codes
| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `124` | Timeout exceeded |

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and add tests
4. Ensure tests pass (`make test`)
5. Commit using catmit (`catmit`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## 🐛 Troubleshooting

### Common Issues

**API Key Issues:**
```bash
# Verify your API key is set
echo $CATMIT_LLM_API_KEY

# Test API connectivity
catmit --dry-run
```

**No Staged Changes:**
```bash
# Make sure you have staged changes
git status
git add .
```

**Timeout Issues:**
```bash
# Increase timeout
catmit -t 60
```

For more help, check our [Issues](https://github.com/penwyp/catmit/issues) or create a new one.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [DeepSeek](https://www.deepseek.com/) for providing the AI capabilities
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the amazing TUI framework
- [Cobra](https://github.com/spf13/cobra) for the CLI framework
- [Conventional Commits](https://www.conventionalcommits.org/) for the commit standard

## ⭐ Star History

[![Star History Chart](https://api.star-history.com/svg?repos=penwyp/catmit&type=Date)](https://star-history.com/#penwyp/catmit&Date)

---

<div align="center">
  Made with ❤️ by <a href="https://github.com/penwyp">penwyp</a>
  
  If catmit helped you, please consider giving it a ⭐!
</div>