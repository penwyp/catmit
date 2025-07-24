package git

import (
	"context"
	"strings"
)

// GetDefaultBranch detects the default branch name of a remote repository
// It follows these steps:
// 1. Try to get the default branch from the remote HEAD reference
// 2. If that fails, check common branch names in order: main, master, develop, trunk
// 3. If all checks fail, return "main" as fallback
func (r *realRunner) GetDefaultBranch(ctx context.Context, remote string) (string, error) {
	// Try to get default branch from remote HEAD
	output, err := r.Run(ctx, "git", "ls-remote", "--symref", remote, "HEAD")
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "ref: refs/heads/") {
				branch := strings.TrimPrefix(line, "ref: refs/heads/")
				parts := strings.Fields(branch)
				if len(parts) > 0 {
					return parts[0], nil
				}
			}
		}
	}

	// Fallback to common defaults
	commonDefaults := []string{"main", "master", "develop", "trunk"}
	for _, branch := range commonDefaults {
		_, err = r.Run(ctx, "git", "ls-remote", "--heads", remote, branch)
		if err == nil {
			return branch, nil
		}
	}

	// Final fallback
	return "main", nil
}

// GetDefaultBranchWithRunner detects the default branch using a provided runner
// This allows using the function with different runner implementations
func GetDefaultBranchWithRunner(ctx context.Context, runner Runner, remote string) (string, error) {
	// Check if runner has the GetDefaultBranch method
	if realRunner, ok := runner.(*realRunner); ok {
		return realRunner.GetDefaultBranch(ctx, remote)
	}

	// Fallback implementation for other runner types
	output, err := runner.Run(ctx, "git", "ls-remote", "--symref", remote, "HEAD")
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "ref: refs/heads/") {
				branch := strings.TrimPrefix(line, "ref: refs/heads/")
				parts := strings.Fields(branch)
				if len(parts) > 0 {
					return parts[0], nil
				}
			}
		}
	}

	// Fallback to common defaults
	commonDefaults := []string{"main", "master", "develop", "trunk"}
	for _, branch := range commonDefaults {
		_, err = runner.Run(ctx, "git", "ls-remote", "--heads", remote, branch)
		if err == nil {
			return branch, nil
		}
	}

	// Final fallback
	return "main", nil
}