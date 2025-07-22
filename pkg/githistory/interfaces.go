package githistory

import (
	"context"
	"time"
)

// Commit represents a Git commit with all relevant metadata
type Commit struct {
	SHA         string    // Full SHA hash
	ShortSHA    string    // Short SHA (first 7 characters)
	Subject     string    // First line of commit message
	Body        string    // Rest of commit message
	Author      string    // Author name and email
	AuthorDate  time.Time // When the commit was authored
	Committer   string    // Committer name and email
	CommitDate  time.Time // When the commit was committed
	ParentSHAs  []string  // Parent commit SHAs (can be multiple for merge commits)
}

// HistoryReader provides read-only operations for Git history analysis.
// This interface focuses on gathering information about commits and branches
// without modifying the repository state.
type HistoryReader interface {
	// FindMergeBase finds the common ancestor between two refs (branches/commits)
	// Equivalent to: git merge-base ref1 ref2
	// Returns the SHA of the merge base commit
	FindMergeBase(ctx context.Context, ref1, ref2 string) (string, error)

	// GetUnpushedCommits returns commits that exist locally but not on remote
	// Uses git cherry or git rev-list to find commits unique to local branch
	// Returns commits in chronological order (oldest first)
	GetUnpushedCommits(ctx context.Context, base, head string) ([]Commit, error)

	// GetCommit retrieves detailed information about a specific commit
	// Accepts SHA (full or short) or ref name
	GetCommit(ctx context.Context, ref string) (*Commit, error)

	// GetCommitsBetween returns all commits between two refs (exclusive of base, inclusive of head)
	// Useful for understanding the full history that will be squashed
	GetCommitsBetween(ctx context.Context, base, head string) ([]Commit, error)

	// HasUncommittedChanges checks if there are any uncommitted changes
	// Returns true if working directory is dirty
	HasUncommittedChanges(ctx context.Context) (bool, error)

	// GetCurrentBranch returns the name of the current branch
	// Returns error if in detached HEAD state
	GetCurrentBranch(ctx context.Context) (string, error)
}

// HistoryModifier provides operations that modify Git history.
// These operations should be used with caution as they can rewrite history.
// Always create backups before performing destructive operations.
type HistoryModifier interface {
	// BackupBranch creates a backup of the current branch
	// Creates a new branch with name "{original}_bak" pointing to current HEAD
	// Returns the name of the backup branch
	BackupBranch(ctx context.Context, branch string) (string, error)

	// RebaseInteractive performs an interactive rebase to squash commits
	// base: The commit to rebase onto (exclusive)
	// commits: The commits to be squashed (in chronological order)
	// newMessage: The new commit message for the squashed commit
	// Returns error if rebase fails or conflicts occur
	RebaseInteractive(ctx context.Context, base string, commits []Commit, newMessage string) error

	// ResetHard performs a hard reset to a specific commit
	// Useful for recovery if rebase fails
	// WARNING: This will lose uncommitted changes
	ResetHard(ctx context.Context, ref string) error

	// AbortRebase aborts an in-progress rebase operation
	// Safe to call even if no rebase is in progress
	AbortRebase(ctx context.Context) error

	// CherryPick applies specific commits on top of current HEAD
	// Useful for selective history modification
	CherryPick(ctx context.Context, commits []string) error
}

// HistoryManager combines both reader and modifier interfaces
// This is the main interface that implementations should provide
type HistoryManager interface {
	HistoryReader
	HistoryModifier
}