# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`catmit` is a Go CLI/TUI tool that auto-generates high-quality Git commit messages using AI (LLM). It analyzes repository history and staged changes to create conventional commit messages with optional Chinese/English output and interactive confirmation. The tool supports multiple Git providers (GitHub, GitLab) with PR creation capabilities.

## Development Commands

### Build and Test
```bash
make build      # Build binary to bin/catmit
make test       # Run all unit/integration/E2E tests
make lint       # Run golangci-lint
make e2e        # Run E2E tests only
make clean      # Remove bin directory
make install    # Install binary to GOBIN
make release    # Tag and push release (triggers GitHub Actions)
```

### Direct Go commands
```bash
go test ./...                    # Run all tests
go test ./test/e2e              # Run E2E tests only
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out  # Test with coverage
golangci-lint run               # Lint check
```

### Testing Commands
```bash
# Run single test or test file
go test -run TestGitInfo_RecentCommits ./pkg/gitinfo  # Run specific test
go test -run TestGitInfo ./...                         # Run all tests matching pattern

# Testing options
go test -v ./...                # Verbose output
go test -race ./...             # With race detection
go test -count=1 ./...          # Disable test caching
go test -timeout 30s ./...      # With custom timeout

# Coverage analysis
go test -cover ./pkg/gitinfo                   # Basic coverage for package
go test -coverprofile=coverage.out ./...       # Generate coverage report
go tool cover -html=coverage.out               # View coverage in browser
```

### Environment Setup
Required environment variable:
- `CATMIT_LLM_API_KEY=sk-xxxx` - LLM API key for calls

Optional environment variables:
- `CATMIT_LLM_API_URL` - Complete API endpoint URL (defaults to https://api.deepseek.com/v1/chat/completions)
- `CATMIT_LLM_MODEL` - Model name (defaults to deepseek-chat)
- `CATMIT_CONFIG_PATH` - Path to configuration file (defaults to ~/.config/catmit/config.yaml)

#### Supported LLM Providers
The client supports any OpenAI-compatible API through environment variables:

**DeepSeek (Default):**
```bash
export CATMIT_LLM_API_KEY="sk-xxxx"
# URL and model use defaults
```

**Volcengine Ark:**
```bash
export CATMIT_LLM_API_KEY="********"
export CATMIT_LLM_API_URL="https://ark.cn-beijing.volces.com/api/v3/chat/completions"
export CATMIT_LLM_MODEL="deepseek-v3-250324"
```

**Other OpenAI-compatible providers:**
Set the three environment variables accordingly.

## Architecture

The codebase follows a modular design with clear separation between public APIs (`pkg/`) and internal implementation (`internal/`):

### Public Package Modules (`pkg/`)
- **`pkg/gitinfo/`** - Git operations collector with comprehensive change analysis, file prioritization, and token budgeting
- **`pkg/llm/`** - LLM API client with Provider abstraction supporting multiple OpenAI-compatible APIs
- **`pkg/prompt/`** - Prompt template builder with language support, diff truncation, and token budgeting

### Internal Modules (`internal/`)
- **`internal/app/`** - Application dependencies and providers management
- **`internal/cli/`** - CLI detection and version management for git, gh, glab tools
- **`internal/config/`** - YAML-based configuration management with hot-reload support
- **`internal/errors/`** - Custom error types and handlers (e.g., ErrPRAlreadyExists)
- **`internal/git/`** - Git operations (commit, push, stage, remote management)
- **`internal/pr/`** - Pull request creation logic with cross-platform support
- **`internal/provider/`** - Git provider auto-detection (GitHub, GitLab, Gitea)
- **`internal/squash/`** - Commit squashing functionality with editor integration
- **`internal/template/`** - Customizable PR template management
- **`internal/ui/`** - Bubble Tea TUI components for interactive workflow
- **`internal/workflow/`** - Workflow orchestration and state management

### Commands (`cmd/`)
- **`cmd/root.go`** - Main CLI entry with dependency injection
- **`cmd/squash.go`** - Squash command implementation
- **`cmd/auth_*.go`** - Authentication status checking for all remotes
- **`cmd/demo_ui.go`** - UI demonstration for testing

### Configuration Management
The tool supports YAML-based configuration with hot-reload:
```yaml
# ~/.config/catmit/config.yaml
llm:
  api_key: "sk-xxxx"
  api_url: "https://api.deepseek.com/v1/chat/completions"
  model: "deepseek-chat"
  
pr:
  template: |
    ## Summary
    {{ .Summary }}
    
    ## Changes
    {{ .Changes }}
```

### Dependency Injection Pattern
The application uses interface-based dependency injection for testability:
- `GitInfoInterface` - Git data collection with comprehensive diff analysis
- `PromptInterface` - Prompt building with token budgeting
- `LLMInterface` - LLM API calls 
- `GitInterface` - Git operations (commit, push, PR creation)

Mock implementations can be injected for testing.

### UI Architecture
The TUI uses a unified `MainModel` that manages the entire lifecycle:
- **Phase Management**: Loading → Review → Commit → Done
- **State Transitions**: Handles user input, API calls, and commit operations
- **Error Handling**: Graceful error display and recovery
- **Real-time Updates**: Spinner animations and progress indicators

### Provider Detection
The tool automatically detects Git providers:
- **GitHub**: Full support via `gh` CLI
- **GitLab**: Support via `glab` CLI
- **Gitea**: Planned support
- **Fallback**: Generic Git operations

### Key Interfaces
```go
// Git information collection
type GitInfoInterface interface {
    RecentCommits(ctx context.Context, n int) ([]string, error)
    Diff(ctx context.Context) (string, error)
    BranchName(ctx context.Context) (string, error)
    ChangedFiles(ctx context.Context) ([]string, error)
    ComprehensiveDiff(ctx context.Context) (string, error)
    AnalyzeChanges(ctx context.Context) (*ChangesSummary, error)
}

// Git operations
type GitInterface interface {
    Commit(ctx context.Context, message string) error
    Push(ctx context.Context) error
    StageAll(ctx context.Context) error
    HasStagedChanges(ctx context.Context) (bool, error)
    CreatePullRequest(ctx context.Context, title, body string) (string, error)
}

// LLM operations
type LLMInterface interface {
    GenerateCommitMessage(ctx context.Context, prompt string) (string, error)
}
```

## CLI Usage Patterns

### Standard workflow
```bash
catmit                # Interactive mode with TUI
catmit -y            # Auto-commit without confirmation
catmit --dry-run     # Preview message only
catmit -l zh         # Chinese output
catmit -t 30         # 30 second timeout
catmit "feat: auth"  # Seed text via positional argument
catmit --seed "feat: auth"  # Seed text via flag (same effect)
```

### Squash workflow
```bash
catmit squash-draft        # Default mode (opens editor) with TUI
catmit squash-draft -n     # No confirmation, output directly
catmit squash-draft -l zh  # Generate Chinese commit message
catmit squash-draft -t 60  # 60 second timeout
```

The squash command helps consolidate multiple commit messages into a single, comprehensive commit message. It:
- Opens your default editor ($EDITOR) to input commit messages
- Uses LLM to analyze and merge them intelligently
- Maintains conventional commit format
- Automatically copies result to clipboard with confirmation
- Cleans output to ensure no AI prefixes are included
- Supports all language options from main command

### Pull Request workflow
```bash
catmit --pr   # Commit, push, and create PR
catmit -y --pr  # Auto-commit and create PR
catmit -p=false --pr  # Create PR without pushing (for existing branches)
catmit auth status   # Check authentication status for all remotes
```

### PR Feature Details
- **Multi-Platform Support**: GitHub (`gh`), GitLab (`glab`), with auto-detection
- **Template Support**: Customizable PR templates via config file
- **Auto Push**: Automatically pushes if needed before PR creation
- **Error Handling**: Shows existing PR URL if PR already exists
- **Auth Check**: Validates CLI installation and authentication

### Exit codes
- `0` - Success
- `124` - Timeout exceeded (follows CLI convention)
- `1` - General error

## Testing Strategy

### Test Structure
- **Unit tests (~70%)** - Each module with mocked dependencies
- **Integration tests (~20%)** - Module interactions with httptest for API
- **E2E tests (~10%)** - Full binary testing in temporary git repos

### Testing Requirements
- Use `stretchr/testify` for assertions and mocks
- Mock external dependencies (git commands, HTTP calls)
- Test both success and error paths
- Verify conventional commit format compliance

### Coverage Targets
- Core logic (`pkg/`): >90%
- Integration logic (`internal/`): >85%
- Overall project: >80%

## Code Conventions

### Language and Comments
- Mix of Chinese and English comments (following existing pattern)
- Interface documentation in English
- Implementation comments often in Chinese

### Error Handling
- Context-aware operations with timeout support
- Custom error types in `internal/errors` package
- Graceful degradation on API failures
- Provider-specific error handling with user-friendly messages

### Testing Conventions
- Interface mocking for external dependencies
- `httptest.NewServer` for API testing
- Temporary git repositories for E2E tests

## Development Workflow

1. **TDD Approach** - Write tests first, then implementation
2. **Interface-First** - Define interfaces before concrete implementations
3. **Context Propagation** - All operations accept `context.Context`
4. **Dependency Injection** - Use provider functions for testability

## Enhanced Features

### Comprehensive Change Analysis
The gitinfo package provides enhanced change analysis:
- **File Prioritization**: Sorts files by change importance and type
- **Untracked File Support**: Includes untracked files in diff analysis
- **Token Budgeting**: Intelligently truncates large diffs to fit LLM token limits
- **Batch Operations**: Concurrent git operations for better performance

### Configuration System
- **YAML Configuration**: User preferences and API settings
- **Hot Reload**: Changes take effect without restart
- **Template Support**: Customizable PR templates
- **Environment Override**: Env vars take precedence over config file

### Multi-Provider Support
- **Auto-Detection**: Automatically detects GitHub/GitLab/Gitea
- **Provider-Specific Features**: Leverages platform-specific CLI tools
- **Fallback Support**: Generic Git operations when CLI tools unavailable
- **Extensible Design**: Easy to add new providers

### UI/UX Improvements
- **Unified Model**: Single `MainModel` handles entire workflow
- **Real-time Progress**: Visual feedback for all operations
- **Error Recovery**: Graceful handling of various failure scenarios
- **Responsive Design**: Adapts to different terminal sizes

## Documentation

### README Files
The project maintains two README files:
- **`README.md`** - English version with comprehensive project documentation, installation, usage, and contributing guidelines
- **`README_zh.md`** - Chinese version with the same content translated for Chinese-speaking users

Both README files include:
- Project overview with logo and badges
- Feature highlights with emojis for better visual appeal
- Installation instructions (Homebrew, Go install, binary download)
- Usage examples (basic and advanced)
- Development setup and testing instructions
- Troubleshooting section
- Contributing guidelines

The README follows modern GitHub project standards with:
- Professional styling and layout
- Clear sectioning with emoji headers
- Code examples with syntax highlighting
- Badges for build status, license, and version
- Star history chart
- Acknowledgments section

## Release Process

- Uses `goreleaser` with GitHub Actions
- Supports macOS/Linux (amd64/arm64)
- Static compilation with `CGO_ENABLED=0`
- Automated releases on git tag creation
- Homebrew tap support for easy installation

### Release Commands
```bash
make release    # Tag and push release (triggers GitHub Actions)
# Manual release process:
# 1. Create and push tag: git tag vX.Y.Z && git push origin vX.Y.Z
# 2. GitHub Actions will automatically build and create release
```