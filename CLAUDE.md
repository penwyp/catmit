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
- `DEEPSEEK_API_KEY=sk-xxxx` - DeepSeek API key for LLM calls

Optional:
- `DEEPSEEK_API_BASE_URL` - Custom API endpoint (defaults to https://api.deepseek.com)

## Architecture

The codebase follows a modular design with clear separation of concerns:

### Core Modules
- **`collector/`** - Git operations (log, diff, branch info) with Runner interface abstraction
- **`client/`** - DeepSeek API client with timeout and error handling  
- **`prompt/`** - Prompt template builder with language support and diff truncation
- **`ui/`** - Bubble Tea TUI models for loading, progress, and review screens
- **`cmd/`** - Cobra CLI with dependency injection interfaces for testability

### Dependency Injection Pattern
The `cmd/root.go` uses interface-based dependency injection to enable testing:
- `collectorInterface` - Git data collection
- `promptInterface` - Prompt building
- `clientInterface` - LLM API calls
- `commitInterface` - Git commit execution

Mock implementations can be injected by setting the provider functions (`collectorProvider`, `promptProvider`, etc.).

### Key Interfaces
```go
type collectorInterface interface {
    RecentCommits(ctx context.Context, n int) ([]string, error)
    Diff(ctx context.Context) (string, error)
    BranchName(ctx context.Context) (string, error)
    ChangedFiles(ctx context.Context) ([]string, error)
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
- Specific error types (e.g., `collector.ErrNoDiff`)
- Graceful degradation on API failures

### Testing Conventions
- Interface mocking for external dependencies
- `httptest.NewServer` for API testing
- Temporary git repositories for E2E tests

## Development Workflow

1. **TDD Approach** - Write tests first, then implementation
2. **Interface-First** - Define interfaces before concrete implementations
3. **Context Propagation** - All operations accept `context.Context`
4. **Dependency Injection** - Use provider functions for testability

## Release Process

- Uses `goreleaser` with GitHub Actions
- Supports macOS/Linux (amd64/arm64)
- Static compilation with `CGO_ENABLED=0`
- Automated releases on git tag creation