package git

import "context"

// Remote represents information about a Git remote repository
type Remote struct {
	Name     string // Remote repository name, e.g., origin
	FetchURL string // Fetch URL
	PushURL  string // Push URL
}

// Runner is the interface for executing Git commands
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (string, error)
}

// RemoteManager is the interface for managing Git remote repositories
type RemoteManager interface {
	// GetRemotes retrieves all remote repositories
	GetRemotes(ctx context.Context) ([]Remote, error)

	// SelectRemote selects a remote repository by priority
	SelectRemote(remotes []Remote, preferredName string) (*Remote, error)

	// GetCurrentBranch retrieves the current branch name
	GetCurrentBranch(ctx context.Context) (string, error)

	// HasUpstreamBranch checks if the branch has an upstream branch
	HasUpstreamBranch(ctx context.Context, branch string) bool
}
