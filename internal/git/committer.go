package git

import (
	"context"
)

// Committer handles git commit and push operations
type Committer interface {
	// Commit creates a git commit with the given message
	Commit(ctx context.Context, message string) error

	// Push pushes the current branch to remote
	Push(ctx context.Context) error

	// StageAll stages all changes (tracked and untracked)
	StageAll(ctx context.Context) error

	// HasStagedChanges checks if there are staged changes
	HasStagedChanges(ctx context.Context) bool

	// CreatePullRequest creates a pull request
	CreatePullRequest(ctx context.Context) (string, error)

	// NeedsPush checks if the current branch needs to be pushed
	NeedsPush(ctx context.Context) (bool, error)
}
