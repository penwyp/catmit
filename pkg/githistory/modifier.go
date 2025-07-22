package githistory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// modifier implements HistoryModifier interface
type modifier struct {
	runner Runner
}

// NewModifier creates a new HistoryModifier instance
func NewModifier(runner Runner) HistoryModifier {
	return &modifier{runner: runner}
}

// BackupBranch creates a backup of the current branch
func (m *modifier) BackupBranch(ctx context.Context, branch string) (string, error) {
	backupName := fmt.Sprintf("%s_bak", branch)
	
	// Check if backup branch already exists
	_, err := m.runner.Run(ctx, "git", "rev-parse", "--verify", backupName)
	if err == nil {
		// Backup exists, add timestamp
		backupName = fmt.Sprintf("%s_bak_%d", branch, time.Now().Unix())
	}
	
	// Create the backup branch
	_, err = m.runner.Run(ctx, "git", "branch", backupName)
	if err != nil {
		return "", fmt.Errorf("failed to create backup branch %s: %w", backupName, err)
	}
	
	return backupName, nil
}

// RebaseInteractive performs an interactive rebase to squash commits
func (m *modifier) RebaseInteractive(ctx context.Context, base string, commits []Commit, newMessage string) error {
	if len(commits) == 0 {
		return fmt.Errorf("no commits to rebase")
	}
	
	// For now, we'll use a simpler approach: reset soft and recommit
	// This is safer and more predictable than trying to automate git rebase -i
	
	// First, get the current HEAD reference for potential recovery
	currentHead, err := m.runner.Run(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current HEAD: %w", err)
	}
	currentHead = strings.TrimSpace(currentHead)
	
	// Perform a soft reset to the base commit
	// This keeps all changes staged
	_, err = m.runner.Run(ctx, "git", "reset", "--soft", base)
	if err != nil {
		return fmt.Errorf("failed to reset to base %s: %w", base, err)
	}
	
	// Now all the changes from the squashed commits are staged
	// Create a new commit with the consolidated message
	_, err = m.runner.Run(ctx, "git", "commit", "-m", newMessage)
	if err != nil {
		// Try to recover by resetting back to original HEAD
		_, _ = m.runner.Run(ctx, "git", "reset", "--hard", currentHead)
		return fmt.Errorf("failed to create new commit: %w", err)
	}
	
	return nil
}

// ResetHard performs a hard reset to a specific commit
func (m *modifier) ResetHard(ctx context.Context, ref string) error {
	_, err := m.runner.Run(ctx, "git", "reset", "--hard", ref)
	if err != nil {
		return fmt.Errorf("failed to reset to %s: %w", ref, err)
	}
	return nil
}

// AbortRebase aborts an in-progress rebase operation
func (m *modifier) AbortRebase(ctx context.Context) error {
	_, err := m.runner.Run(ctx, "git", "rebase", "--abort")
	// Ignore error if no rebase is in progress
	if err != nil && !strings.Contains(err.Error(), "No rebase in progress") {
		return fmt.Errorf("failed to abort rebase: %w", err)
	}
	return nil
}

// CherryPick applies specific commits on top of current HEAD
func (m *modifier) CherryPick(ctx context.Context, commits []string) error {
	if len(commits) == 0 {
		return fmt.Errorf("no commits to cherry-pick")
	}
	
	args := append([]string{"cherry-pick"}, commits...)
	_, err := m.runner.Run(ctx, "git", args...)
	if err != nil {
		return fmt.Errorf("failed to cherry-pick commits: %w", err)
	}
	
	return nil
}

