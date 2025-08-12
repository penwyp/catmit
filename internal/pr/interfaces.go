package pr

import "context"

// ExtendedGitRunner extends GitRunner with additional methods for PR analysis
type ExtendedGitRunner interface {
	GitRunner
	// Run executes a git command with args
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// CreatorInterface defines the interface for PR creators
type CreatorInterface interface {
	Create(ctx context.Context, options CreateOptions) (string, error)
	// CheckExists checks if a PR already exists for the current branch
	// Returns: exists (true if PR exists), prURL (URL of existing PR), error
	CheckExists(ctx context.Context, options CreateOptions) (bool, string, error)
}

// LLMInterface defines the interface for LLM clients
type LLMInterface interface {
	GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
