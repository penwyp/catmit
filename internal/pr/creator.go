package pr

import (
	"context"
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
	// Set default value for remote if not specified
	if options.Remote == "" {
		options.Remote = "origin"
	}

	if c.logger != nil {
		c.logger.Debug("Starting PR creation",
			zap.String("remote", options.Remote),
			zap.String("base_branch", options.BaseBranch),
			zap.Bool("draft", options.Draft))
	}

	// Get remote URL
	remoteURL, err := c.git.GetRemoteURL(ctx, options.Remote)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get remote URL", err)
	}

	if c.logger != nil {
		c.logger.Debug("Remote URL retrieved",
			zap.String("remote", options.Remote),
			zap.String("url", remoteURL))
	}

	// Detect provider
	remoteInfo, err := c.providerDetector.DetectFromRemote(ctx, remoteURL)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeProvider, "failed to detect provider", err)
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
		return "", errors.ErrProviderNotSupported
	}

	// Detect CLI status
	cliStatus, err := c.cliDetector.DetectCLI(ctx, remoteInfo.Provider)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypePR, "failed to detect CLI", err)
	}

	// Check if CLI is installed
	if !cliStatus.Installed {
		return "", errors.ErrCLINotInstalled.WithSuggestion(fmt.Sprintf("Please install %s CLI tool", cliStatus.Name))
	}

	// Check if CLI is authenticated
	if !cliStatus.Authenticated {
		return "", errors.ErrCLINotAuthed.WithSuggestion(fmt.Sprintf("Please run %s auth login to authenticate", cliStatus.Name))
	}

	// Check version requirements
	if minVersion, ok := minVersionRequirements[remoteInfo.Provider]; ok {
		meetsRequirement, err := c.cliDetector.CheckMinVersion(cliStatus.Version, minVersion)
		if err != nil {
			return "", errors.Wrap(errors.ErrTypePR, "failed to check version", err)
		}
		if !meetsRequirement {
			return "", errors.New(errors.ErrTypePR, fmt.Sprintf("%s version %s is below minimum required version %s",
				cliStatus.Name, cliStatus.Version, minVersion)).WithSuggestion(fmt.Sprintf("Please upgrade %s to %s or higher", cliStatus.Name, minVersion))
		}
	}

	// Get base branch (if not specified)
	if options.BaseBranch == "" {
		defaultBranch, err := c.git.GetDefaultBranch(ctx, options.Remote)
		if err != nil {
			// If failed to get, use common default value
			options.BaseBranch = "main"
		} else {
			options.BaseBranch = defaultBranch
		}
	}

	// Get current branch (if needed)
	var headBranch string
	if options.HeadBranch == "" {
		headBranch, err = c.git.GetCurrentBranch(ctx)
		if err != nil {
			return "", errors.Wrap(errors.ErrTypeGit, "failed to get current branch", err)
		}
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

// CheckExists checks if a PR already exists for the current branch
func (c *Creator) CheckExists(ctx context.Context, options CreateOptions) (bool, string, error) {
	// 1. Detect provider and remote info
	remoteURL, err := c.git.GetRemoteURL(ctx, options.Remote)
	if err != nil {
		return false, "", errors.Wrapf(errors.ErrTypePR, "failed to get remote URL", err)
	}

	remoteInfo, err := c.providerDetector.DetectFromRemote(ctx, remoteURL)
	if err != nil {
		return false, "", errors.Wrapf(errors.ErrTypePR, "failed to detect provider", err)
	}

	if c.logger != nil {
		c.logger.Debug("checking PR existence",
			zap.String("provider", remoteInfo.Provider),
			zap.String("host", remoteInfo.Host),
			zap.String("owner", remoteInfo.Owner),
			zap.String("repo", remoteInfo.Repo))
	}

	// 2. Check if CLI is available
	cliStatus, err := c.cliDetector.DetectCLI(ctx, remoteInfo.Provider)
	if err != nil {
		return false, "", errors.Wrapf(errors.ErrTypePR, "failed to detect CLI", err)
	}

	if !cliStatus.Installed {
		// If CLI is not installed, we cannot check PR existence
		// Return false to allow the workflow to continue
		return false, "", nil
	}

	if !cliStatus.Authenticated {
		// If CLI is not authenticated, we cannot check PR existence
		// Return false to allow the workflow to continue
		return false, "", nil
	}

	// 3. Get current branch
	currentBranch, err := c.git.GetCurrentBranch(ctx)
	if err != nil {
		return false, "", errors.Wrapf(errors.ErrTypePR, "failed to get current branch", err)
	}

	// 4. Build and execute check command based on provider
	switch remoteInfo.Provider {
	case "github":
		return c.checkGitHubPR(ctx, currentBranch, remoteInfo)
	case "gitlab":
		return c.checkGitLabMR(ctx, currentBranch, remoteInfo)
	case "gitea":
		return c.checkGiteaPR(ctx, currentBranch, remoteInfo)
	default:
		// For unknown providers, return false
		return false, "", nil
	}
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

	// Simple JSON parsing for PR URL
	// Look for the first "open" PR
	if strings.Contains(outputStr, `"state":"OPEN"`) && strings.Contains(outputStr, `"url":"`) {
		// Extract URL
		startIdx := strings.Index(outputStr, `"url":"`) + 7
		endIdx := strings.Index(outputStr[startIdx:], `"`)
		if endIdx > 0 {
			prURL := outputStr[startIdx : startIdx+endIdx]
			return true, prURL, nil
		}
	}

	return false, "", nil
}

// checkGitLabMR checks if a GitLab MR exists for the current branch
func (c *Creator) checkGitLabMR(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use glab mr list to check for existing MRs
	// --source-branch flag to filter by source branch
	// -R to specify the repository
	args := []string{"mr", "list", "--source-branch", branch,
		"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}
	output, err := c.commandRunner.Run(ctx, "glab", args...)
	if err != nil {
		// If command fails, assume no MR exists
		return false, "", nil
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || strings.Contains(outputStr, "No merge requests match your search") {
		// No MRs found
		return false, "", nil
	}

	// Parse the output to find an open MR
	// GitLab CLI output format: "!123  Title  (branch -> target)"
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Extract MR number from the first column
		parts := strings.Fields(line)
		if len(parts) > 0 && strings.HasPrefix(parts[0], "!") {
			// Get MR details to extract URL
			mrNumber := strings.TrimPrefix(parts[0], "!")
			detailOutput, err := c.commandRunner.Run(ctx, "glab", "mr", "view", mrNumber, "--output", "json",
				"-R", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo))
			if err == nil {
				// Extract web_url from JSON
				detailStr := string(detailOutput)
				if strings.Contains(detailStr, `"web_url":"`) {
					startIdx := strings.Index(detailStr, `"web_url":"`) + 11
					endIdx := strings.Index(detailStr[startIdx:], `"`)
					if endIdx > 0 {
						mrURL := detailStr[startIdx : startIdx+endIdx]
						return true, mrURL, nil
					}
				}
			}
		}
	}

	return false, "", nil
}