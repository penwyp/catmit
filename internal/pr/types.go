package pr

import "github.com/penwyp/catmit/internal/template"

// PROptions defines options for creating a Pull Request
type PROptions struct {
	// Common fields
	Title      string // PR title
	Body       string // PR description/body
	BaseBranch string // Target branch (required)
	HeadBranch string // Source branch (required for Gitea)
	Draft      bool   // Whether the PR is a draft

	// Repository information (for cross-fork PRs)
	TargetOwner string // Target repository owner
	TargetRepo  string // Target repository name
	SourceOwner string // Source repository owner (for fork workflow)

	// Metadata
	Labels    []string // Labels
	Assignees []string // Assignees
	Reviewers []string // Reviewers
	Milestone string   // Milestone

	// Special options
	Fill bool // GitHub --fill option, auto-fill title and description
}

// CreateOptions defines advanced options for PR creation
type CreateOptions struct {
	Remote     string   // Git remote name, defaults to "origin"
	Title      string   // PR title
	Body       string   // PR description
	BaseBranch string   // Target branch
	HeadBranch string   // Source branch (optional, auto-detect)
	Draft      bool     // Whether the PR is a draft
	Labels     []string // Labels
	Assignees  []string // Assignees
	Reviewers  []string // Reviewers
	Fill       bool     // Use --fill option

	// Template related options
	UseTemplate  bool                   // Whether to use a template
	TemplateData *template.TemplateData // Template data (if provided)
}
