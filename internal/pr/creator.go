package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/penwyp/catmit/internal/template"
	"go.uber.org/zap"
)

// ErrPRAlreadyExists is returned when a PR already exists for the branch
// This wraps the framework error with additional URL information
type ErrPRAlreadyExists struct {
	URL string
	err error
}

func (e *ErrPRAlreadyExists) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return "pull request already exists: " + e.URL
}

func (e *ErrPRAlreadyExists) Unwrap() error {
	return e.err
}

// Minimum version requirements for CLI tools
var minVersionRequirements = map[string]string{
	"github": "2.0.0",
	"gitea":  "0.8.0",
	"gitlab": "1.0.0",
}

// GitRunner interface for running git commands
type GitRunner interface {
	GetRemoteURL(ctx context.Context, remote string) (string, error)
	GetCurrentBranch(ctx context.Context) (string, error)
	GetCommitMessage(ctx context.Context, ref string) (string, error)
	GetDefaultBranch(ctx context.Context, remote string) (string, error)
	GetParentBranch(ctx context.Context, remote string) (string, error)
}

// ProviderDetector interface for detecting provider
type ProviderDetector interface {
	DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error)
}

// CLIDetector interface for detecting CLI
type CLIDetector interface {
	DetectCLI(ctx context.Context, provider string) (cli.CLIStatus, error)
	CheckMinVersion(current, minimum string) (bool, error)
}

// CommandBuilderInterface interface for building commands
type CommandBuilderInterface interface {
	BuildCommand(provider string, options PROptions) (string, []string, error)
	ParseGitHubPROutput(output string) (string, error)
	ParseGiteaPROutput(output string) (string, error)
	ParseGitLabMROutput(output string) (string, error)
	ParseGiteaErrorForPRInfo(errorOutput string, remoteHost string, owner string, repo string) (string, error)
}

// CommandRunner interface for running commands
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Creator is the pull request creator
type Creator struct {
	git              GitRunner
	providerDetector ProviderDetector
	cliDetector      CLIDetector
	commandBuilder   CommandBuilderInterface
	commandRunner    CommandRunner
	templateManager  template.Manager // Optional template manager
	logger           *zap.Logger      // Optional logger
}

// NewCreator creates a new PR creator
func NewCreator(
	git GitRunner,
	providerDetector ProviderDetector,
	cliDetector CLIDetector,
	commandBuilder CommandBuilderInterface,
	commandRunner CommandRunner,
) *Creator {
	return &Creator{
		git:              git,
		providerDetector: providerDetector,
		cliDetector:      cliDetector,
		commandBuilder:   commandBuilder,
		commandRunner:    commandRunner,
	}
}

// WithTemplateManager sets the template manager
func (c *Creator) WithTemplateManager(tm template.Manager) *Creator {
	c.templateManager = tm
	return c
}

// WithLogger sets the logger
func (c *Creator) WithLogger(logger *zap.Logger) *Creator {
	c.logger = logger
	return c
}

// Create creates a pull request
func (c *Creator) Create(ctx context.Context, options CreateOptions) (string, error) {
	// Prepare all context needed for PR creation
	prContext, err := c.PrepareContext(ctx, options)
	if err != nil {
		return "", err
	}

	// Use the resolved options from context
	options = prContext.Options
	remoteInfo := prContext.RemoteInfo
	headBranch := prContext.CurrentBranch

	// Set head branch if needed for provider-specific requirements
	if options.HeadBranch == "" {
		if remoteInfo.Provider == "gitea" {
			options.HeadBranch = headBranch
		}
	} else {
		headBranch = options.HeadBranch
	}

	// Handle template (if enabled)
	if options.UseTemplate && c.templateManager != nil {
		// Try to load template
		tmpl, err := c.templateManager.LoadTemplate(ctx, &remoteInfo)
		if err == nil {
			// If template loaded successfully, prepare template data
			templateData := options.TemplateData
			if templateData == nil {
				// If no template data provided, create basic data
				templateData = &template.TemplateData{
					Branch:     headBranch,
					BaseBranch: options.BaseBranch,
					Remote:     options.Remote,
					RepoOwner:  remoteInfo.Owner,
					RepoName:   remoteInfo.Repo,
				}

				// Use title and body if provided
				if options.Title != "" {
					templateData.CommitTitle = options.Title
				}
				if options.Body != "" {
					templateData.CommitMessage = options.Body
					templateData.CommitBody = options.Body
				}
			}

			// Process template
			processedBody, err := c.templateManager.ProcessTemplate(ctx, tmpl, templateData)
			if err == nil {
				// If processed successfully, use template-generated content
				options.Body = processedBody
				// If template contains title, it may need to be extracted
				// But usually title is a separate field
			}
			// If template processing fails, continue using original content
		}
		// If template loading fails, continue using original content
	}

	// Build PR options
	prOptions := PROptions{
		Title:       options.Title,
		Body:        options.Body,
		BaseBranch:  options.BaseBranch,
		HeadBranch:  options.HeadBranch,
		Draft:       options.Draft,
		Labels:      options.Labels,
		Assignees:   options.Assignees,
		Reviewers:   options.Reviewers,
		Fill:        options.Fill,
		TargetOwner: remoteInfo.Owner,
		TargetRepo:  remoteInfo.Repo,
	}

	// If not using origin, it may be a cross-fork PR
	// Get origin info as the source repository
	if options.Remote != "origin" {
		if c.logger != nil {
			c.logger.Debug("Cross-fork PR detected, fetching origin info")
		}
		originURL, err := c.git.GetRemoteURL(ctx, "origin")
		if err == nil {
			// Parse origin URL to get source repo info
			originInfo, err := c.providerDetector.DetectFromRemote(ctx, originURL)
			if err == nil {
				prOptions.SourceOwner = originInfo.Owner
				if c.logger != nil {
					c.logger.Debug("Origin info retrieved",
						zap.String("origin_owner", originInfo.Owner),
						zap.String("origin_repo", originInfo.Repo))
				}
				// For Gitea, set correct head format
				if remoteInfo.Provider == "gitea" && prOptions.SourceOwner != "" {
					prOptions.HeadBranch = prOptions.SourceOwner + ":" + headBranch
					if c.logger != nil {
						c.logger.Debug("Gitea cross-fork head branch formatted",
							zap.String("head_branch", prOptions.HeadBranch))
					}
				}
			}
		}
	}

	// Build command
	if c.logger != nil {
		c.logger.Debug("Building PR command",
			zap.String("provider", remoteInfo.Provider),
			zap.String("target_owner", prOptions.TargetOwner),
			zap.String("target_repo", prOptions.TargetRepo),
			zap.String("base_branch", prOptions.BaseBranch),
			zap.String("head_branch", prOptions.HeadBranch),
			zap.String("source_owner", prOptions.SourceOwner))
	}

	cmd, args, err := c.commandBuilder.BuildCommand(remoteInfo.Provider, prOptions)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypePR, "failed to build command", err)
	}

	if c.logger != nil {
		c.logger.Debug("PR command built",
			zap.String("command", cmd),
			zap.Strings("args", args))
	}

	// Execute command
	if c.logger != nil {
		c.logger.Debug("Executing PR command")
	}

	output, err := c.commandRunner.Run(ctx, cmd, args...)
	outputStr := string(output)

	if c.logger != nil {
		c.logger.Debug("Command execution completed",
			zap.Error(err),
			zap.String("output", outputStr))
	}

	// Parse output to get PR URL
	var prURL string
	var parseErr error

	switch remoteInfo.Provider {
	case "github":
		prURL, parseErr = c.commandBuilder.ParseGitHubPROutput(outputStr)
	case "gitea":
		prURL, parseErr = c.commandBuilder.ParseGiteaPROutput(outputStr)
	case "gitlab":
		prURL, parseErr = c.commandBuilder.ParseGitLabMROutput(outputStr)
	}

	// If command execution failed
	if err != nil {
		// Check if PR already exists
		if strings.Contains(outputStr, "already exists") {
			// If URL is parsed, return specific error
			if parseErr == nil && prURL != "" {
				return "", &ErrPRAlreadyExists{URL: prURL, err: errors.ErrPRAlreadyExists}
			}

			// For Gitea, try to extract PR info from error message
			if remoteInfo.Provider == "gitea" {
				prURL, parseErr := c.commandBuilder.ParseGiteaErrorForPRInfo(outputStr, remoteInfo.Host, remoteInfo.Owner, remoteInfo.Repo)
				if parseErr == nil && prURL != "" {
					return "", &ErrPRAlreadyExists{URL: prURL, err: errors.ErrPRAlreadyExists}
				}
			}

			// Even if URL cannot be parsed, return ErrPRAlreadyExists for user-friendly message
			return "", &ErrPRAlreadyExists{
				URL: "",
				err: errors.Wrapf(errors.ErrTypePR, "PR already exists", errors.New(errors.ErrTypePR, outputStr)),
			}
		}
		return "", errors.Wrapf(errors.ErrTypePR, "failed to create PR\nOutput: %s", err, outputStr)
	}

	// If command succeeded and URL was parsed
	if parseErr == nil && prURL != "" {
		if c.logger != nil {
			c.logger.Debug("PR created successfully",
				zap.String("url", prURL))
		}
		return prURL, nil
	}

	// If command succeeded but parsing failed
	if parseErr != nil {
		return "", errors.Wrap(errors.ErrTypePR, fmt.Sprintf("failed to parse PR URL from output\nOutput: %s", outputStr), parseErr)
	}

	return prURL, nil
}

// PreparedPRContext contains resolved context for PR operations
type PreparedPRContext struct {
	Options        CreateOptions
	RemoteInfo     provider.RemoteInfo
	CLIStatus      cli.CLIStatus
	CurrentBranch  string
}

// PrepareContext resolves all context needed for PR operations (creation and existence checks)
// This extracts the common logic that was duplicated between Create and CheckExists
func (c *Creator) PrepareContext(ctx context.Context, options CreateOptions) (*PreparedPRContext, error) {
	// Set default value for remote if not specified
	if options.Remote == "" {
		options.Remote = "origin"
	}

	if c.logger != nil {
		c.logger.Debug("Preparing PR context",
			zap.String("remote", options.Remote),
			zap.String("base_branch", options.BaseBranch),
			zap.Bool("draft", options.Draft))
	}

	// Get remote URL
	remoteURL, err := c.git.GetRemoteURL(ctx, options.Remote)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get remote URL", err)
	}

	if c.logger != nil {
		c.logger.Debug("Remote URL retrieved",
			zap.String("remote", options.Remote),
			zap.String("url", remoteURL))
	}

	// Detect provider
	remoteInfo, err := c.providerDetector.DetectFromRemote(ctx, remoteURL)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeProvider, "failed to detect provider", err)
	}

	if c.logger != nil {
		c.logger.Debug("Provider detected",
			zap.String("provider", remoteInfo.Provider),
			zap.String("host", remoteInfo.Host),
			zap.String("owner", remoteInfo.Owner),
			zap.String("repo", remoteInfo.Repo))
	}

	// Check if provider is supported
	if remoteInfo.Provider == "unknown" {
		return nil, errors.ErrProviderNotSupported
	}

	// Detect CLI status
	cliStatus, err := c.cliDetector.DetectCLI(ctx, remoteInfo.Provider)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypePR, "failed to detect CLI", err)
	}

	// Check if CLI is installed
	if !cliStatus.Installed {
		return nil, errors.ErrCLINotInstalled.WithSuggestion(fmt.Sprintf("Please install %s CLI tool", cliStatus.Name))
	}

	// Check if CLI is authenticated
	if !cliStatus.Authenticated {
		return nil, errors.ErrCLINotAuthed.WithSuggestion(fmt.Sprintf("Please run %s auth login to authenticate", cliStatus.Name))
	}

	// Check version requirements
	if minVersion, ok := minVersionRequirements[remoteInfo.Provider]; ok {
		meetsRequirement, err := c.cliDetector.CheckMinVersion(cliStatus.Version, minVersion)
		if err != nil {
			return nil, errors.Wrap(errors.ErrTypePR, "failed to check version", err)
		}
		if !meetsRequirement {
			return nil, errors.New(errors.ErrTypePR, fmt.Sprintf("%s version %s is below minimum required version %s",
				cliStatus.Name, cliStatus.Version, minVersion)).WithSuggestion(fmt.Sprintf("Please upgrade %s to %s or higher", cliStatus.Name, minVersion))
		}
	}

	// Get base branch (if not specified)
	if options.BaseBranch == "" {
		// First try to detect which branch the current branch is based on
		parentBranch, err := c.git.GetParentBranch(ctx, options.Remote)
		if err != nil {
			// If that fails, fall back to default branch detection
			defaultBranch, err := c.git.GetDefaultBranch(ctx, options.Remote)
			if err != nil {
				// If failed to get, use common default value
				options.BaseBranch = "main"
			} else {
				options.BaseBranch = defaultBranch
			}
		} else {
			options.BaseBranch = parentBranch
		}
	}

	// Get current branch
	currentBranch, err := c.git.GetCurrentBranch(ctx)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get current branch", err)
	}

	if c.logger != nil {
		c.logger.Debug("PR context prepared",
			zap.String("current_branch", currentBranch),
			zap.String("base_branch", options.BaseBranch),
			zap.String("provider", remoteInfo.Provider))
	}

	return &PreparedPRContext{
		Options:       options,
		RemoteInfo:    remoteInfo,
		CLIStatus:     cliStatus,
		CurrentBranch: currentBranch,
	}, nil
}

// CheckExists checks if a PR already exists for the current branch
func (c *Creator) CheckExists(ctx context.Context, options CreateOptions) (bool, string, error) {
	// Prepare context with complete provider/remote/base branch resolution
	prContext, err := c.PrepareContext(ctx, options)
	if err != nil {
		// If context preparation fails due to CLI issues, gracefully continue
		if errors.Is(err, errors.ErrCLINotInstalled) || errors.Is(err, errors.ErrCLINotAuthed) {
			if c.logger != nil {
				c.logger.Debug("CLI not available for PR existence check, continuing", zap.Error(err))
			}
			return false, "", nil
		}
		// For other errors, return them
		return false, "", err
	}

	remoteInfo := prContext.RemoteInfo
	currentBranch := prContext.CurrentBranch
	baseBranch := prContext.Options.BaseBranch

	if c.logger != nil {
		c.logger.Debug("checking PR existence with full context",
			zap.String("provider", remoteInfo.Provider),
			zap.String("current_branch", currentBranch),
			zap.String("base_branch", baseBranch),
			zap.String("remote", prContext.Options.Remote),
			zap.String("owner", remoteInfo.Owner),
			zap.String("repo", remoteInfo.Repo))
	}

	// Build and execute check command based on provider with base branch context
	switch remoteInfo.Provider {
	case "github":
		return c.checkGitHubPRWithBase(ctx, currentBranch, baseBranch, remoteInfo)
	case "gitlab":
		return c.checkGitLabMRWithBase(ctx, currentBranch, baseBranch, remoteInfo)
	case "gitea":
		return c.checkGiteaPRWithBase(ctx, currentBranch, baseBranch, remoteInfo)
	default:
		// For unknown providers, return false
		return false, "", nil
	}
}

// GitHubPullRequest represents a pull request in gh CLI JSON output
type GitHubPullRequest struct {
	URL   string `json:"url"`
	State string `json:"state"`
	Base  struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// checkGitHubPR checks if a GitHub PR exists for the current branch
func (c *Creator) checkGitHubPR(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use gh pr list to check for existing PRs
	// --head flag to filter by source branch
	// --json to get structured output
	// -R to specify the repository
	args := []string{"pr", "list", "--head", branch, "--json", "url,state",
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	output, err := c.commandRunner.Run(ctx, "gh", args...)
	if err != nil {
		// If command fails, assume no PR exists
		return false, "", nil
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "[]" || outputStr == "" {
		// No PRs found
		return false, "", nil
	}

	var prs []GitHubPullRequest
	if err := json.Unmarshal([]byte(outputStr), &prs); err != nil {
		// If JSON parsing fails, fall back to no PR exists
		return false, "", nil
	}

	// Check each PR to find one that is open
	for _, pr := range prs {
		if pr.State == "OPEN" {
			return true, pr.URL, nil
		}
	}

	return false, "", nil
}

// GitLabMergeRequest represents a merge request in glab CLI JSON output
type GitLabMergeRequest struct {
	IID    int    `json:"iid"`
	WebURL string `json:"web_url"`
	State  string `json:"state"`
	SourceBranch string `json:"source_branch"`
}

// checkGitLabMR checks if a GitLab MR exists for the current branch
func (c *Creator) checkGitLabMR(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use glab mr list with JSON output to get structured data
	// --output json for structured output
	// --source-branch to filter by source branch
	// -R to specify the repository
	args := []string{"mr", "list", "--output", "json", "--source-branch", branch,
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	output, err := c.commandRunner.Run(ctx, "glab", args...)
	if err != nil {
		// If command fails, assume no MR exists
		return false, "", nil
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		// No MRs found
		return false, "", nil
	}

	var mrs []GitLabMergeRequest
	if err := json.Unmarshal([]byte(outputStr), &mrs); err != nil {
		// If JSON parsing fails, fall back to no MR exists
		return false, "", nil
	}

	// Check each MR to find one matching the current branch and is open
	for _, mr := range mrs {
		if mr.SourceBranch == branch && mr.State == "opened" {
			return true, mr.WebURL, nil
		}
	}

	return false, "", nil
}

// checkGitHubPRWithBase checks if a GitHub PR exists for the current branch targeting a specific base branch
func (c *Creator) checkGitHubPRWithBase(ctx context.Context, branch, baseBranch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use gh pr list to check for existing PRs
	// --head flag to filter by source branch
	// --base flag to filter by base branch  
	// --json to get structured output including base ref
	// -R to specify the repository
	args := []string{"pr", "list", "--head", branch, "--base", baseBranch, "--json", "url,state,base",
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	output, err := c.commandRunner.Run(ctx, "gh", args...)
	if err != nil {
		// If command fails, assume no PR exists
		return false, "", nil
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "[]" || outputStr == "" {
		// No PRs found
		return false, "", nil
	}

	var prs []GitHubPullRequest
	if err := json.Unmarshal([]byte(outputStr), &prs); err != nil {
		// If JSON parsing fails, fall back to no PR exists
		return false, "", nil
	}

	// Check each PR to find one that is open and targets the correct base branch
	for _, pr := range prs {
		if pr.State == "OPEN" && pr.Base.Ref == baseBranch {
			return true, pr.URL, nil
		}
	}

	return false, "", nil
}

// GitLabMergeRequestWithBase represents a merge request with base info in glab CLI JSON output
type GitLabMergeRequestWithBase struct {
	IID          int    `json:"iid"`
	WebURL       string `json:"web_url"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

// checkGitLabMRWithBase checks if a GitLab MR exists for the current branch targeting a specific base branch
func (c *Creator) checkGitLabMRWithBase(ctx context.Context, branch, baseBranch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use glab mr list with JSON output to get structured data
	// --output json for structured output
	// --source-branch to filter by source branch
	// --target-branch to filter by target branch
	// -R to specify the repository
	args := []string{"mr", "list", "--output", "json", "--source-branch", branch, "--target-branch", baseBranch,
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	output, err := c.commandRunner.Run(ctx, "glab", args...)
	if err != nil {
		// If command fails, assume no MR exists
		return false, "", nil
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		// No MRs found
		return false, "", nil
	}

	var mrs []GitLabMergeRequestWithBase
	if err := json.Unmarshal([]byte(outputStr), &mrs); err != nil {
		// If JSON parsing fails, fall back to no MR exists
		return false, "", nil
	}

	// Check each MR to find one matching the current branch, base branch and is open
	for _, mr := range mrs {
		if mr.SourceBranch == branch && mr.TargetBranch == baseBranch && mr.State == "opened" {
			return true, mr.WebURL, nil
		}
	}

	return false, "", nil
}

// checkGiteaPRWithBase checks if a Gitea PR exists for the current branch targeting a specific base branch
func (c *Creator) checkGiteaPRWithBase(ctx context.Context, branch, baseBranch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// For now, use the same logic as the existing checkGiteaPR but we could enhance this
	// to include base branch filtering when Gitea CLI supports it
	// Use tea pulls list with JSON output to get structured data
	// --output json for structured output
	// --fields to get needed fields including base info
	// --state open to get only open PRs
	// --repo to specify the repository
	args := []string{"pulls", "list", "--output", "json", "--fields", "index,head,base,url", "--state", "open", "--repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}

	output, err := c.commandRunner.Run(ctx, "tea", args...)
	if err != nil {
		// If command fails, assume no PR exists
		return false, "", nil
	}

	// Parse JSON output - using the existing Gitea structures but checking base branch
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		// No PRs found
		return false, "", nil
	}

	// Parse as generic interface to handle the nested structure
	var prs []map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &prs); err != nil {
		// If JSON parsing fails, fall back to no PR exists
		return false, "", nil
	}

	// Check each PR to find one matching the current branch and base branch
	for _, pr := range prs {
		// Extract head and base information
		if head, ok := pr["head"].(map[string]interface{}); ok {
			if base, ok := pr["base"].(map[string]interface{}); ok {
				// Check source branch
				var sourceBranch string
				if name, ok := head["name"].(string); ok {
					sourceBranch = name
					// Handle cross-fork format (owner:branch)
					if strings.Contains(sourceBranch, ":") {
						parts := strings.Split(sourceBranch, ":")
						if len(parts) == 2 {
							sourceBranch = parts[1]
						}
					}
				}

				// Check target branch
				var targetBranch string
				if name, ok := base["name"].(string); ok {
					targetBranch = name
				}

				// If both branches match, return the PR
				if sourceBranch == branch && targetBranch == baseBranch {
					if url, ok := pr["url"].(string); ok {
						return true, url, nil
					}
				}
			}
		}
	}

	return false, "", nil
}