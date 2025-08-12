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
	if c.logger != nil {
		c.logger.Debug("Starting PR existence check",
			zap.String("remote", options.Remote),
			zap.String("base_branch", options.BaseBranch))
	}

	// Prepare context with complete provider/remote/base branch resolution
	prContext, err := c.PrepareContext(ctx, options)
	if err != nil {
		// Enhanced error classification for better handling
		if errors.Is(err, errors.ErrCLINotInstalled) {
			if c.logger != nil {
				c.logger.Info("CLI not installed for PR existence check",
					zap.Error(err),
					zap.String("suggestion", "Install CLI tools for better PR detection"))
			}
			return false, "", nil
		}
		
		if errors.Is(err, errors.ErrCLINotAuthed) {
			if c.logger != nil {
				c.logger.Info("CLI not authenticated for PR existence check",
					zap.Error(err),
					zap.String("suggestion", "Authenticate CLI tools for better PR detection"))
			}
			return false, "", nil
		}
		
		if errors.Is(err, errors.ErrProviderNotSupported) {
			if c.logger != nil {
				c.logger.Info("Provider not supported for PR existence check",
					zap.Error(err))
			}
			return false, "", nil
		}

		// For other errors (network, git, etc.), log and return error to let caller decide
		if c.logger != nil {
			c.logger.Warn("Context preparation failed in PR existence check",
				zap.Error(err),
				zap.String("error_type", fmt.Sprintf("%T", err)))
		}
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
	if c.logger != nil {
		c.logger.Debug("Executing provider-specific PR existence check",
			zap.String("provider", remoteInfo.Provider),
			zap.String("owner", remoteInfo.Owner),
			zap.String("repo", remoteInfo.Repo),
			zap.String("current_branch", currentBranch),
			zap.String("base_branch", baseBranch))
	}

	switch remoteInfo.Provider {
	case "github":
		exists, url, err := c.checkGitHubPRWithBase(ctx, currentBranch, baseBranch, remoteInfo)
		if err != nil && c.logger != nil {
			c.logger.Error("GitHub PR check failed", 
				zap.Error(err),
				zap.String("branch", currentBranch),
				zap.String("base", baseBranch))
		}
		return exists, url, err
	case "gitlab":
		exists, url, err := c.checkGitLabMRWithBase(ctx, currentBranch, baseBranch, remoteInfo)
		if err != nil && c.logger != nil {
			c.logger.Error("GitLab MR check failed", 
				zap.Error(err),
				zap.String("branch", currentBranch),
				zap.String("base", baseBranch))
		}
		return exists, url, err
	case "gitea":
		exists, url, err := c.checkGiteaPRWithBase(ctx, currentBranch, baseBranch, remoteInfo)
		if err != nil && c.logger != nil {
			c.logger.Error("Gitea PR check failed", 
				zap.Error(err),
				zap.String("branch", currentBranch),
				zap.String("base", baseBranch))
		}
		return exists, url, err
	default:
		if c.logger != nil {
			c.logger.Warn("Unknown provider for PR existence check",
				zap.String("provider", remoteInfo.Provider))
		}
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


// GitLabMergeRequest represents a merge request in glab CLI JSON output
type GitLabMergeRequest struct {
	IID    int    `json:"iid"`
	WebURL string `json:"web_url"`
	State  string `json:"state"`
	SourceBranch string `json:"source_branch"`
}


// CandidatePR represents a potential PR found during the first stage filtering
type CandidatePR struct {
	ID           string
	URL          string
	State        string
	SourceBranch string
	TargetBranch string
	Provider     string
	RepoOwner    string
	RepoName     string
	Raw          interface{} // Original data for further validation
}

// getGitHubCandidatePRs gets all PRs for the current branch (first stage filtering)
func (c *Creator) getGitHubCandidatePRs(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) ([]CandidatePR, error) {
	// Use gh pr list to get all PRs for the current branch (no base filter yet)
	// --head flag to filter by source branch
	// --json to get structured output including base ref
	// -R to specify the repository
	args := []string{"pr", "list", "--head", branch, "--json", "number,url,state,baseRefName,headRefName",
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	
	if c.logger != nil {
		c.logger.Debug("Getting GitHub candidate PRs", 
			zap.String("command", "gh "+strings.Join(args, " ")),
			zap.String("branch", branch),
			zap.String("repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)))
	}
	
	output, err := c.commandRunner.Run(ctx, "gh", args...)
	if err != nil {
		if c.logger != nil {
			c.logger.Debug("GitHub PR list command failed", zap.Error(err))
		}
		return nil, err
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "[]" || outputStr == "" {
		if c.logger != nil {
			c.logger.Debug("No GitHub PRs found for branch", zap.String("branch", branch))
		}
		return []CandidatePR{}, nil
	}

	var rawPRs []map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &rawPRs); err != nil {
		if c.logger != nil {
			c.logger.Debug("Failed to parse GitHub PR JSON", zap.Error(err), zap.String("output", outputStr))
		}
		return nil, err
	}

	var candidates []CandidatePR
	for _, rawPR := range rawPRs {
		candidate := CandidatePR{
			Provider:  "github",
			RepoOwner: remoteInfo.Owner,
			RepoName:  remoteInfo.Repo,
			Raw:       rawPR,
		}

		// Extract basic fields
		if number, ok := rawPR["number"].(float64); ok {
			candidate.ID = fmt.Sprintf("%.0f", number)
		}
		if url, ok := rawPR["url"].(string); ok {
			candidate.URL = url
		}
		if state, ok := rawPR["state"].(string); ok {
			candidate.State = state
		}
		if baseRef, ok := rawPR["baseRefName"].(string); ok {
			candidate.TargetBranch = baseRef
		}
		if headRef, ok := rawPR["headRefName"].(string); ok {
			candidate.SourceBranch = headRef
		}

		candidates = append(candidates, candidate)
	}

	if c.logger != nil {
		c.logger.Debug("Found GitHub PR candidates",
			zap.Int("count", len(candidates)),
			zap.String("branch", branch))
	}

	return candidates, nil
}

// checkGitHubPRWithBase checks if a GitHub PR exists for the current branch targeting a specific base branch
func (c *Creator) checkGitHubPRWithBase(ctx context.Context, branch, baseBranch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Stage 1: Get all candidate PRs for this branch
	candidates, err := c.getGitHubCandidatePRs(ctx, branch, remoteInfo)
	if err != nil {
		return false, "", err
	}

	// Stage 2: Filter and validate candidates
	return c.filterCandidatePRs(candidates, baseBranch)
}

// filterCandidatePRs filters candidate PRs based on criteria and returns the first match
func (c *Creator) filterCandidatePRs(candidates []CandidatePR, baseBranch string) (bool, string, error) {
	if c.logger != nil {
		c.logger.Debug("Filtering candidate PRs",
			zap.Int("candidates", len(candidates)),
			zap.String("target_base", baseBranch))
	}

	for _, candidate := range candidates {
		if c.logger != nil {
			c.logger.Debug("Evaluating PR candidate",
				zap.String("id", candidate.ID),
				zap.String("url", candidate.URL),
				zap.String("state", candidate.State),
				zap.String("source_branch", candidate.SourceBranch),
				zap.String("target_branch", candidate.TargetBranch),
				zap.String("provider", candidate.Provider))
		}

		// Check if PR is in open state - be strict about state checking
		isOpen := false
		switch candidate.Provider {
		case "github":
			// GitHub uses uppercase "OPEN" for open PRs
			isOpen = strings.ToUpper(candidate.State) == "OPEN"
		case "gitlab":
			// GitLab uses lowercase "opened" for open MRs
			isOpen = strings.ToLower(candidate.State) == "opened"
		case "gitea":
			// Gitea uses lowercase "open" for open PRs
			isOpen = strings.ToLower(candidate.State) == "open"
		default:
			// For unknown providers, be conservative - only exact matches
			isOpen = candidate.State == "OPEN" || candidate.State == "opened" || candidate.State == "open"
		}

		if c.logger != nil {
			c.logger.Debug("PR state evaluation",
				zap.String("id", candidate.ID),
				zap.String("provider", candidate.Provider),
				zap.String("raw_state", candidate.State),
				zap.Bool("is_open", isOpen))
		}

		if !isOpen {
			if c.logger != nil {
				c.logger.Debug("Skipping PR - not in open state", 
					zap.String("id", candidate.ID),
					zap.String("state", candidate.State))
			}
			continue
		}

		// Check if target branch matches (with flexibility)
		if c.isBaseBranchMatch(candidate.TargetBranch, baseBranch) {
			if c.logger != nil {
				c.logger.Debug("Found matching PR",
					zap.String("id", candidate.ID),
					zap.String("url", candidate.URL),
					zap.String("target_branch", candidate.TargetBranch),
					zap.String("expected_base", baseBranch))
			}
			return true, candidate.URL, nil
		} else {
			if c.logger != nil {
				c.logger.Debug("Skipping PR - base branch mismatch",
					zap.String("id", candidate.ID),
					zap.String("pr_base", candidate.TargetBranch),
					zap.String("expected_base", baseBranch))
			}
		}
	}

	if c.logger != nil {
		c.logger.Debug("No matching PR found after filtering")
	}
	return false, "", nil
}

// isBaseBranchMatch checks if two base branch names match, with controlled flexibility
func (c *Creator) isBaseBranchMatch(prBaseBranch, expectedBase string) bool {
	// Direct match is always preferred
	if prBaseBranch == expectedBase {
		if c.logger != nil {
			c.logger.Debug("Base branch exact match",
				zap.String("pr_base", prBaseBranch),
				zap.String("expected", expectedBase))
		}
		return true
	}

	// Only allow variations for known default branches to avoid false positives
	// This is more restrictive than before to reduce incorrect matches
	defaultBranchVariations := map[string][]string{
		"main":   {"master"},  // Only allow main <-> master
		"master": {"main"},    // Only allow master <-> main
		// Removed develop/dev variations as they're more specific branches
	}

	// Check if expected base has allowed variations that might match PR base
	if variations, exists := defaultBranchVariations[expectedBase]; exists {
		for _, variation := range variations {
			if prBaseBranch == variation {
				if c.logger != nil {
					c.logger.Debug("Base branch matched via controlled variation",
						zap.String("pr_base", prBaseBranch),
						zap.String("expected", expectedBase),
						zap.String("matched_via", variation))
				}
				return true
			}
		}
	}

	if c.logger != nil {
		c.logger.Debug("Base branch did not match",
			zap.String("pr_base", prBaseBranch),
			zap.String("expected", expectedBase))
	}

	return false
}

// GitLabMergeRequestWithBase represents a merge request with base info in glab CLI JSON output
type GitLabMergeRequestWithBase struct {
	IID          int    `json:"iid"`
	WebURL       string `json:"web_url"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

// getGitLabCandidateMRs gets all MRs for the current branch (first stage filtering)
func (c *Creator) getGitLabCandidateMRs(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) ([]CandidatePR, error) {
	// Use glab mr list to get all MRs for the current branch (no target filter yet)
	// --output json for structured output
	// --source-branch to filter by source branch
	// -R to specify the repository
	args := []string{"mr", "list", "--output", "json", "--source-branch", branch,
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	
	if c.logger != nil {
		c.logger.Debug("Getting GitLab candidate MRs", 
			zap.String("command", "glab "+strings.Join(args, " ")),
			zap.String("branch", branch),
			zap.String("repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)))
	}
	
	output, err := c.commandRunner.Run(ctx, "glab", args...)
	if err != nil {
		if c.logger != nil {
			c.logger.Debug("GitLab MR list command failed", zap.Error(err))
		}
		return nil, err
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		if c.logger != nil {
			c.logger.Debug("No GitLab MRs found for branch", zap.String("branch", branch))
		}
		return []CandidatePR{}, nil
	}

	var rawMRs []map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &rawMRs); err != nil {
		if c.logger != nil {
			c.logger.Debug("Failed to parse GitLab MR JSON", zap.Error(err), zap.String("output", outputStr))
		}
		return nil, err
	}

	var candidates []CandidatePR
	for _, rawMR := range rawMRs {
		candidate := CandidatePR{
			Provider:  "gitlab",
			RepoOwner: remoteInfo.Owner,
			RepoName:  remoteInfo.Repo,
			Raw:       rawMR,
		}

		// Extract basic fields
		if iid, ok := rawMR["iid"].(float64); ok {
			candidate.ID = fmt.Sprintf("%.0f", iid)
		}
		if webURL, ok := rawMR["web_url"].(string); ok {
			candidate.URL = webURL
		}
		if state, ok := rawMR["state"].(string); ok {
			candidate.State = state
		}
		if sourceBranch, ok := rawMR["source_branch"].(string); ok {
			candidate.SourceBranch = sourceBranch
		}
		if targetBranch, ok := rawMR["target_branch"].(string); ok {
			candidate.TargetBranch = targetBranch
		}

		candidates = append(candidates, candidate)
	}

	if c.logger != nil {
		c.logger.Debug("Found GitLab MR candidates",
			zap.Int("count", len(candidates)),
			zap.String("branch", branch))
	}

	return candidates, nil
}

// checkGitLabMRWithBase checks if a GitLab MR exists for the current branch targeting a specific base branch
func (c *Creator) checkGitLabMRWithBase(ctx context.Context, branch, baseBranch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Stage 1: Get all candidate MRs for this branch
	candidates, err := c.getGitLabCandidateMRs(ctx, branch, remoteInfo)
	if err != nil {
		return false, "", err
	}

	// Stage 2: Filter and validate candidates
	return c.filterCandidatePRs(candidates, baseBranch)
}

// getGiteaCandidatePRs gets all PRs for the current branch (first stage filtering)
func (c *Creator) getGiteaCandidatePRs(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) ([]CandidatePR, error) {
	// Use tea pulls list to get all PRs (no branch filter initially)
	// --output json for structured output
	// --fields to get needed fields including head and base info
	// --state open to get only open PRs
	// --repo to specify the repository
	args := []string{"pulls", "list", "--output", "json", "--fields", "index,head,base,url", "--state", "open", "--repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}

	if c.logger != nil {
		c.logger.Debug("Getting Gitea candidate PRs", 
			zap.String("command", "tea "+strings.Join(args, " ")),
			zap.String("branch", branch),
			zap.String("repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)))
	}

	output, err := c.commandRunner.Run(ctx, "tea", args...)
	if err != nil {
		if c.logger != nil {
			c.logger.Debug("Gitea PR list command failed", zap.Error(err))
		}
		return nil, err
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		if c.logger != nil {
			c.logger.Debug("No Gitea PRs found", zap.String("branch", branch))
		}
		return []CandidatePR{}, nil
	}

	// Parse as generic interface to handle the nested structure
	var rawPRs []map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &rawPRs); err != nil {
		if c.logger != nil {
			c.logger.Debug("Failed to parse Gitea PR JSON", zap.Error(err), zap.String("output", outputStr))
		}
		return nil, err
	}

	var candidates []CandidatePR
	for _, rawPR := range rawPRs {
		candidate := CandidatePR{
			Provider:  "gitea",
			RepoOwner: remoteInfo.Owner,
			RepoName:  remoteInfo.Repo,
			State:     "open", // Gitea returns only open PRs due to --state filter
			Raw:       rawPR,
		}

		// Extract index as ID
		if index, ok := rawPR["index"].(float64); ok {
			candidate.ID = fmt.Sprintf("%.0f", index)
		}

		// Extract URL
		if url, ok := rawPR["url"].(string); ok {
			candidate.URL = url
		}

		// Extract head (source branch) and base (target branch) information
		// Handle both string format and nested object format
		switch head := rawPR["head"].(type) {
		case string:
			// Direct string format: "owner:branch" or just "branch"
			sourceBranch := head
			if strings.Contains(sourceBranch, ":") {
				parts := strings.Split(sourceBranch, ":")
				if len(parts) == 2 {
					sourceBranch = parts[1]
				}
			}
			candidate.SourceBranch = sourceBranch
		case map[string]interface{}:
			// Nested object format: {"name": "branch"}
			if name, ok := head["name"].(string); ok {
				sourceBranch := name
				// Handle cross-fork format (owner:branch)
				if strings.Contains(sourceBranch, ":") {
					parts := strings.Split(sourceBranch, ":")
					if len(parts) == 2 {
						sourceBranch = parts[1]
					}
				}
				candidate.SourceBranch = sourceBranch
			}
		}

		// Handle base field similarly
		switch base := rawPR["base"].(type) {
		case string:
			// Direct string format: "main"
			candidate.TargetBranch = base
		case map[string]interface{}:
			// Nested object format: {"name": "main"}
			if name, ok := base["name"].(string); ok {
				candidate.TargetBranch = name
			}
		}

		// Only include PRs that match the current branch
		if candidate.SourceBranch == branch {
			candidates = append(candidates, candidate)
		}
	}

	if c.logger != nil {
		c.logger.Debug("Found Gitea PR candidates",
			zap.Int("count", len(candidates)),
			zap.String("branch", branch))
	}

	return candidates, nil
}

// checkGiteaPRWithBase checks if a Gitea PR exists for the current branch targeting a specific base branch
func (c *Creator) checkGiteaPRWithBase(ctx context.Context, branch, baseBranch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Stage 1: Get all candidate PRs for this branch
	candidates, err := c.getGiteaCandidatePRs(ctx, branch, remoteInfo)
	if err != nil {
		return false, "", err
	}

	// Stage 2: Filter and validate candidates
	return c.filterCandidatePRs(candidates, baseBranch)
}