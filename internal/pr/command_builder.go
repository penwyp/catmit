package pr

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
)

// CommandBuilder is a builder for PR commands
type CommandBuilder struct{}

// NewCommandBuilder creates a new CommandBuilder instance
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

// BuildCommand builds the corresponding PR command based on the provider
func (b *CommandBuilder) BuildCommand(provider string, options PROptions) (string, []string, error) {
	switch provider {
	case "github":
		return b.BuildGitHubPRCommand(options)
	case "gitea":
		return b.BuildGiteaPRCommand(options)
	case "gitlab":
		return b.BuildGitLabMRCommand(options)
	default:
		return "", nil, errors.New(errors.ErrTypeProvider, fmt.Sprintf("unsupported provider: %s", provider)).WithSuggestion("Supported providers: GitHub, GitLab, Gitea, and Bitbucket")
	}
}

// BuildGitHubPRCommand builds the PR creation command for GitHub CLI
func (b *CommandBuilder) BuildGitHubPRCommand(options PROptions) (string, []string, error) {
	// Validate required fields
	if options.BaseBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "base branch is required").WithSuggestion("Use --pr-base parameter to specify the base branch")
	}

	args := []string{"pr", "create"}

	// If using fill option, other options may not be needed
	if options.Fill {
		args = append(args, "--fill")
		args = append(args, "--base", options.BaseBranch)
		if options.Draft {
			args = append(args, "--draft=true")
		} else {
			args = append(args, "--draft=false")
		}
		return "gh", args, nil
	}

	// Title and body
	if options.Title != "" {
		args = append(args, "--title", options.Title)
	}
	if options.Body != "" {
		args = append(args, "--body", options.Body)
	}

	// Base branch
	args = append(args, "--base", options.BaseBranch)

	// Draft status
	if options.Draft {
		args = append(args, "--draft")
	}

	// Assignees
	if len(options.Assignees) > 0 {
		args = append(args, "--assignee", strings.Join(options.Assignees, ","))
	}

	// Labels
	if len(options.Labels) > 0 {
		args = append(args, "--label", strings.Join(options.Labels, ","))
	}

	// Reviewers
	if len(options.Reviewers) > 0 {
		args = append(args, "--reviewer", strings.Join(options.Reviewers, ","))
	}

	return "gh", args, nil
}

// BuildGiteaPRCommand builds the PR creation command for tea CLI (Gitea)
func (b *CommandBuilder) BuildGiteaPRCommand(options PROptions) (string, []string, error) {
	// Validate required fields
	if options.BaseBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "base branch is required").WithSuggestion("Use --pr-base parameter to specify the base branch")
	}
	if options.HeadBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "head branch is required for Gitea")
	}

	args := []string{"pr", "create"}

	// If target repository info is provided, add --repo parameter
	if options.TargetOwner != "" && options.TargetRepo != "" {
		args = append(args, "--repo", options.TargetOwner+"/"+options.TargetRepo)
	}

	// Title and description (Gitea uses description instead of body)
	if options.Title != "" {
		args = append(args, "--title", options.Title)
	}
	if options.Body != "" {
		args = append(args, "--description", options.Body)
	}

	// Branches
	args = append(args, "--base", options.BaseBranch)
	args = append(args, "--head", options.HeadBranch)

	// Assignees (Gitea uses assignees)
	if len(options.Assignees) > 0 {
		args = append(args, "--assignees", strings.Join(options.Assignees, ","))
	}

	// Labels
	if len(options.Labels) > 0 {
		args = append(args, "--labels", strings.Join(options.Labels, ","))
	}

	// Milestone
	if options.Milestone != "" {
		args = append(args, "--milestone", options.Milestone)
	}

	return "tea", args, nil
}

// ParseGitHubPROutput parses the output of GitHub CLI to get the PR URL
func (b *CommandBuilder) ParseGitHubPROutput(output string) (string, error) {
	// Regular expression for GitHub PR URL
	urlRegex := regexp.MustCompile(`https://github\.com/[\w-]+/[\w-]+/pull/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", errors.New(errors.ErrTypePR, "no PR URL found in output")
}

// ParseGiteaPROutput parses the output of tea CLI to get the PR URL
func (b *CommandBuilder) ParseGiteaPROutput(output string) (string, error) {
	// Regular expression for Gitea PR URL (more general, supports various domains)
	urlRegex := regexp.MustCompile(`https?://[^\s]+/pulls?/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", errors.New(errors.ErrTypePR, "no PR URL found in output")
}

// ParseGiteaErrorForPRInfo extracts PR-related information from tea error output
// Error format: "pull request already exists for these targets [id: 840, issue_id: 80, ...]"
func (b *CommandBuilder) ParseGiteaErrorForPRInfo(errorOutput string, remoteHost string, owner string, repo string) (string, error) {
	// Regular expression to extract issue_id
	issueIDRegex := regexp.MustCompile(`issue_id:\s*(\d+)`)
	matches := issueIDRegex.FindStringSubmatch(errorOutput)
	if len(matches) > 1 {
		issueID := matches[1]
		// Build PR URL
		// Gitea PR URL format: https://host/owner/repo/pulls/issue_id
		protocol := "https"
		if strings.Contains(errorOutput, "http://") {
			protocol = "http"
		}
		prURL := fmt.Sprintf("%s://%s/%s/%s/pulls/%s", protocol, remoteHost, owner, repo, issueID)
		return prURL, nil
	}

	return "", errors.New(errors.ErrTypePR, "could not extract issue ID from error message")
}

// BuildGitLabMRCommand builds the MR creation command for GitLab CLI
func (b *CommandBuilder) BuildGitLabMRCommand(options PROptions) (string, []string, error) {
	// Validate required fields
	if options.BaseBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "base branch is required").WithSuggestion("Use --pr-base parameter to specify the base branch")
	}

	args := []string{"mr", "create"}

	// If using fill option, only provide target branch
	if options.Fill {
		args = append(args, "--fill")
		args = append(args, "--target-branch", options.BaseBranch)
		if options.Draft {
			args = append(args, "--draft")
		}
		// Remove WIP prefix by default
		args = append(args, "--remove-source-branch=false")
		return "glab", args, nil
	}

	// Title and description
	if options.Title != "" {
		args = append(args, "--title", options.Title)
	}
	if options.Body != "" {
		args = append(args, "--description", options.Body)
	}

	// Target branch
	args = append(args, "--target-branch", options.BaseBranch)

	// Draft status
	if options.Draft {
		args = append(args, "--draft")
	}

	// Assignees
	if len(options.Assignees) > 0 {
		args = append(args, "--assignee", strings.Join(options.Assignees, ","))
	}

	// Labels
	if len(options.Labels) > 0 {
		args = append(args, "--label", strings.Join(options.Labels, ","))
	}

	// Reviewers
	if len(options.Reviewers) > 0 {
		// GitLab uses --reviewer for reviewers
		args = append(args, "--reviewer", strings.Join(options.Reviewers, ","))
	}

	// Milestone
	if options.Milestone != "" {
		args = append(args, "--milestone", options.Milestone)
	}

	// Don't remove source branch by default
	args = append(args, "--remove-source-branch=false")

	return "glab", args, nil
}

// ParseGitLabMROutput parses the output of glab CLI to get the MR URL
func (b *CommandBuilder) ParseGitLabMROutput(output string) (string, error) {
	// Regular expression for GitLab MR URL
	urlRegex := regexp.MustCompile(`https?://[^\s]+/-/merge_requests/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", errors.New(errors.ErrTypePR, "no MR URL found in output")
}
