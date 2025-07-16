# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`catmit` is a Go CLI/TUI tool that auto-generates high-quality Git commit messages using DeepSeek LLM. It analyzes repository history and staged changes to create conventional commit messages with optional Chinese/English output and interactive confirmation.

## Development Commands

### Build and Test
```bash
make build      # Build binary to bin/catmit
make test       # Run all unit/integration/E2E tests
make lint       # Run golangci-lint
make e2e        # Run E2E tests only
make clean      # Remove bin directory
```

### Direct Go commands
```bash
go test ./...                    # Run all tests
go test ./test/e2e              # Run E2E tests only
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out  # Test with coverage
golangci-lint run               # Lint check
```

### Environment Setup
Required environment variable:
- `CATMIT_LLM_API_KEY=sk-xxxx` - LLM API key for calls

Optional:
- `CATMIT_LLM_API_URL` - Complete API endpoint URL (defaults to https://api.deepseek.com/v1/chat/completions)
- `CATMIT_LLM_MODEL` - Model name (defaults to deepseek-chat)

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

The codebase follows a modular design with clear separation of concerns:

### Core Modules
- **`collector/`** - Git operations (log, diff, branch info) with Runner interface abstraction and comprehensive change analysis
- **`client/`** - LLM API client with Provider abstraction supporting multiple OpenAI-compatible APIs
- **`prompt/`** - Prompt template builder with language support, diff truncation, and token budgeting
- **`ui/`** - Bubble Tea TUI models for loading, progress, commit workflow, and review screens
- **`cmd/`** - Cobra CLI with dependency injection interfaces for testability and GitHub integration

### Dependency Injection Pattern
The `cmd/root.go` uses interface-based dependency injection to enable testing:
- `collectorInterface` - Git data collection with comprehensive diff analysis
- `promptInterface` - Prompt building with token budgeting
- `clientInterface` - LLM API calls 
- `commitInterface` - Git commit execution and GitHub PR creation

Mock implementations can be injected by setting the provider functions (`collectorProvider`, `promptProvider`, etc.).

### UI Architecture
The TUI uses a unified `MainModel` that manages the entire lifecycle:
- **Phase Management**: Loading → Review → Commit → Done
- **State Transitions**: Handles user input, API calls, and commit operations
- **Error Handling**: Graceful error display and recovery
- **Real-time Updates**: Spinner animations and progress indicators

### LLM Provider Architecture
The client now uses a Provider pattern for maximum flexibility:
- `LLMProvider` interface abstracts different LLM APIs
- `OpenAICompatibleProvider` implements OpenAI-style APIs (DeepSeek, Volcengine, etc.)
- Future non-OpenAI providers can implement the same interface

### GitHub Integration
The tool includes GitHub CLI integration for PR creation:
- **PR Creation**: Uses `gh pr create --fill --base main --draft=false`
- **URL Extraction**: Parses command output to extract PR URLs
- **Error Handling**: Gracefully handles existing PRs by showing URL instead of error
- **No-Changes PR**: Supports creating PRs even when no changes are present

### Key Interfaces
```go
type collectorInterface interface {
    RecentCommits(ctx context.Context, n int) ([]string, error)
    Diff(ctx context.Context) (string, error)
    BranchName(ctx context.Context) (string, error)
    ChangedFiles(ctx context.Context) ([]string, error)
    ComprehensiveDiff(ctx context.Context) (string, error)
    AnalyzeChanges(ctx context.Context) (*ChangesSummary, error)
}

type commitInterface interface {
    Commit(ctx context.Context, message string) error
    Push(ctx context.Context) error
    StageAll(ctx context.Context) error
    HasStagedChanges(ctx context.Context) bool
    CreatePullRequest(ctx context.Context) (string, error)
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
catmit "feat: seed"  # Seed text for generation
```

### Pull Request workflow
```bash
catmit --create-pr   # Commit, push, and create GitHub PR
catmit -y --create-pr  # Auto-commit and create PR
catmit -p=false --create-pr  # Create PR without pushing (for existing branches)
catmit auth status   # Check authentication status for all remotes
```

### PR Feature Details
- **GitHub Support**: Full support via `gh` CLI integration
- **Auto Push**: Automatically pushes if needed before PR creation
- **Error Handling**: Shows existing PR URL if PR already exists
- **Auth Check**: Validates CLI installation and authentication
- **Future Support**: GitLab and Gitea support planned

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
- Core logic (`prompt/`, `client/`): >90%
- Integration logic (`collector/`, `ui/`): >85%
- Overall project: >80%

## Code Conventions

### Language and Comments
- Mix of Chinese and English comments (following existing pattern)
- Interface documentation in English
- Implementation comments often in Chinese

### Error Handling
- Context-aware operations with timeout support
- Specific error types (e.g., `collector.ErrNoDiff`, `ErrPRAlreadyExists`)
- Graceful degradation on API failures
- GitHub integration errors handled with user-friendly messages

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
The collector now provides enhanced change analysis:
- **File Prioritization**: Sorts files by change importance and type
- **Untracked File Support**: Includes untracked files in diff analysis
- **Token Budgeting**: Intelligently truncates large diffs to fit LLM token limits
- **Batch Operations**: Concurrent git operations for better performance

### UI/UX Improvements
- **Unified Model**: Single `MainModel` handles entire workflow
- **Real-time Progress**: Visual feedback for all operations
- **Error Recovery**: Graceful handling of various failure scenarios
- **Responsive Design**: Adapts to different terminal sizes

### GitHub Workflow Integration
- **PR Creation**: Seamless GitHub CLI integration
- **Existing PR Detection**: Intelligent handling of duplicate PRs
- **URL Display**: Shows PR URLs for easy access
- **No-Changes Support**: Creates PRs even without local changes

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