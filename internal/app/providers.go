package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/config"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/penwyp/catmit/internal/template"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/penwyp/catmit/pkg/llm"
	"github.com/penwyp/catmit/pkg/prompt"
	"go.uber.org/zap"
)

// defaultCollectorProvider returns the default collector implementation
func defaultCollectorProvider(debug bool, logger *zap.Logger) *gitinfo.Collector {
	runner := git.NewRunnerWithLogger(debug, logger)
	return gitinfo.New(&GitRunnerAdapter{runner: runner, debug: debug, logger: logger})
}

// defaultPromptProvider returns the default prompt builder
func defaultPromptProvider(lang string) *prompt.Builder {
	return prompt.NewBuilder(lang, 0)
}

// defaultClientProvider returns the default LLM client
func defaultClientProvider(logger *zap.Logger) *llm.Client {
	return llm.NewClient(logger)
}

// defaultCommitterProvider returns the default committer implementation
func defaultCommitterProvider(debug bool, logger *zap.Logger) git.Committer {
	return newDefaultCommitter(debug, logger, false, "", "", false, false)
}

// defaultPRCreatorProvider returns the default PR creator
func defaultPRCreatorProvider(debug bool, logger *zap.Logger) *pr.Creator {
	gitRunner := &GitRunnerAdapter{runner: git.NewRunnerWithLogger(debug, logger), debug: debug, logger: logger}
	providerDetector := newDefaultProviderDetector()
	cliDetector := &DefaultCLIDetector{}

	commandBuilder := pr.NewCommandBuilder()
	commandRunner := &DefaultCommandRunner{debug: debug, logger: logger}

	prCreator := pr.NewCreator(
		gitRunner,
		providerDetector,
		cliDetector,
		commandBuilder,
		commandRunner,
	)

	// Set logger for PR creator
	if logger != nil {
		prCreator = prCreator.WithLogger(logger)
	}

	return prCreator
}

// enhancedPRCreatorProvider returns the enhanced PR creator with LLM support
func enhancedPRCreatorProvider(debug bool, logger *zap.Logger, llmClient pr.LLMInterface) *pr.EnhancedCreator {
	gitRunner := &GitRunnerAdapter{runner: git.NewRunnerWithLogger(debug, logger), debug: debug, logger: logger}
	providerDetector := newDefaultProviderDetector()
	cliDetector := &DefaultCLIDetector{}

	commandBuilder := pr.NewCommandBuilder()
	commandRunner := &DefaultCommandRunner{debug: debug, logger: logger}

	enhancedCreator := pr.NewEnhancedCreator(
		gitRunner,
		providerDetector,
		cliDetector,
		commandBuilder,
		commandRunner,
		llmClient,
	)

	// Set logger for PR creator
	if logger != nil {
		enhancedCreator.WithLogger(logger)
	}

	return enhancedCreator
}

// GitRunnerAdapter adapts internal/git.Runner to collector.Runner interface
// Exported for testing
type GitRunnerAdapter struct {
	runner git.Runner
	debug  bool
	logger *zap.Logger
}

func (a *GitRunnerAdapter) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := a.runner.Run(ctx, name, args...)
	return []byte(output), err
}

// defaultCommitter implements git.Committer interface
type defaultCommitter struct {
	prCreator  pr.CreatorInterface
	ctx        context.Context
	message    string
	debug      bool
	prEnabled  bool
	prRemote   string
	prBase     string
	prDraft    bool
	prTemplate bool
}

// newDefaultCommitter creates a new defaultCommitter with PR support
func newDefaultCommitter(debug bool, logger *zap.Logger, prEnabled bool, prRemote string, prBase string, prDraft bool, prTemplate bool) *defaultCommitter {
	gitRunner := &GitRunnerAdapter{runner: git.NewRunnerWithLogger(debug, logger), debug: debug, logger: logger}
	providerDetector := newDefaultProviderDetector()
	cliDetector := &DefaultCLIDetector{}

	commandBuilder := pr.NewCommandBuilder()
	commandRunner := &DefaultCommandRunner{debug: debug, logger: logger}

	prCreator := pr.NewCreator(
		gitRunner,
		providerDetector,
		cliDetector,
		commandBuilder,
		commandRunner,
	)

	// If PR template is enabled, add template manager
	if prTemplate {
		repoRoot, err := template.FindRepositoryRoot()
		if err == nil {
			templateManager := template.NewDefaultManager(repoRoot)
			prCreator.WithTemplateManager(templateManager)
		}
	}

	// Set logger for PR creator
	if logger != nil {
		prCreator = prCreator.WithLogger(logger)
	}

	return &defaultCommitter{
		prCreator:  prCreator,
		debug:      debug,
		prEnabled:  prEnabled,
		prRemote:   prRemote,
		prBase:     prBase,
		prDraft:    prDraft,
		prTemplate: prTemplate,
	}
}

// newEnhancedCommitter creates a new defaultCommitter with enhanced PR support
func newEnhancedCommitter(debug bool, logger *zap.Logger, prEnabled bool, prRemote string, prBase string, prDraft bool, prTemplate bool, llmClient pr.LLMInterface) *defaultCommitter {
	enhancedCreator := enhancedPRCreatorProvider(debug, logger, llmClient)

	return &defaultCommitter{
		prCreator:  enhancedCreator,
		debug:      debug,
		prEnabled:  prEnabled,
		prRemote:   prRemote,
		prBase:     prBase,
		prDraft:    prDraft,
		prTemplate: prTemplate,
	}
}

func (d *defaultCommitter) Commit(ctx context.Context, message string) error {
	// Save message and context for potential PR creation
	d.ctx = ctx
	d.message = message
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *defaultCommitter) Push(ctx context.Context) error {
	// 获取当前分支名
	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchOutput, err := branchCmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(errors.ErrTypeGit, "failed to get current branch name\nOutput: %s", err, string(branchOutput))
	}
	branchName := strings.TrimSpace(string(branchOutput))
	
	// 执行 git push origin <branch_name>
	cmd := exec.CommandContext(ctx, "git", "push", "origin", branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(errors.ErrTypeGit, "git push failed\nOutput: %s", err, string(output))
	}
	return nil
}

func (d *defaultCommitter) StageAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	return cmd.Run()
}

func (d *defaultCommitter) HasStagedChanges(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	err := cmd.Run()
	// git diff --cached --quiet returns exit code 1 if there are staged changes
	return err != nil
}

func (d *defaultCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	if d.prCreator == nil {
		return "", errors.New(errors.ErrTypePR, "PR creator not initialized")
	}

	// Prepare template data if enabled
	var templateData *template.TemplateData
	if d.prTemplate && d.message != "" {
		// This would need access to collector to get full data
		// For now, create basic template data
		templateData = template.CreateTemplateData(d.message, "", nil)
	}

	// Build PR options
	options := pr.CreateOptions{
		Remote:       d.prRemote,
		BaseBranch:   d.prBase,
		Draft:        d.prDraft,
		Fill:         true,
		UseTemplate:  d.prTemplate,
		TemplateData: templateData,
	}

	return d.prCreator.Create(ctx, options)
}

func (d *defaultCommitter) NeedsPush(ctx context.Context) (bool, error) {
	// Check if the current branch has unpushed commits
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	_, err := cmd.CombinedOutput()
	if err != nil {
		// No upstream branch set, so we need to push
		return true, nil
	}

	// Check if there are commits to push
	cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", "@{u}..HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, errors.Wrap(errors.ErrTypeGit, "failed to check unpushed commits", err)
	}

	// Parse the count
	countStr := strings.TrimSpace(string(output))
	count := 0
	if countStr != "" && countStr != "0" {
		count = 1 // Any non-zero value means we need to push
	}

	return count > 0, nil
}

// GitRunnerAdapter also implements pr.GitRunner interface
func (a *GitRunnerAdapter) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	// First check if we have a catmit-specific remote URL for testing
	catmitKey := "catmit.remote." + remote + ".url"
	output, err := a.runner.Run(ctx, "git", "config", "--get", catmitKey)
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(output), nil
	}

	// Fall back to normal remote URL
	output, err = a.runner.Run(ctx, "git", "remote", "get-url", remote)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (a *GitRunnerAdapter) GetCurrentBranch(ctx context.Context) (string, error) {
	output, err := a.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (a *GitRunnerAdapter) GetCommitMessage(ctx context.Context, ref string) (string, error) {
	output, err := a.runner.Run(ctx, "git", "log", "-1", "--pretty=%B", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (a *GitRunnerAdapter) GetDefaultBranch(ctx context.Context, remote string) (string, error) {
	// Implementation similar to what was in cmd/root.go
	// Try to get default branch from remote
	output, err := a.runner.Run(ctx, "git", "ls-remote", "--symref", remote, "HEAD")
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
		_, err = a.runner.Run(ctx, "git", "ls-remote", "--heads", remote, branch)
		if err == nil {
			return branch, nil
		}
	}

	return "main", nil
}

func (a *GitRunnerAdapter) GetParentBranch(ctx context.Context, remote string) (string, error) {
	// Get the current branch
	currentBranch, err := a.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	currentBranch = strings.TrimSpace(currentBranch)
	
	// Get all remote branches
	output, err := a.runner.Run(ctx, "git", "branch", "-r", "--format=%(refname:short)")
	if err != nil {
		return "", err
	}
	
	remoteBranches := strings.Split(strings.TrimSpace(output), "\n")
	
	// Check common base branches first
	commonBranches := []string{"main", "master", "develop", "trunk"}
	for _, branch := range commonBranches {
		remoteBranch := remote + "/" + branch
		for _, rb := range remoteBranches {
			if strings.TrimSpace(rb) == remoteBranch {
				// Check if there's a merge-base
				_, err := a.runner.Run(ctx, "git", "merge-base", remoteBranch, "HEAD")
				if err == nil {
					return branch, nil
				}
			}
		}
	}
	
	// If no common branches found, try to find any branch with merge-base
	for _, rb := range remoteBranches {
		rb = strings.TrimSpace(rb)
		if !strings.HasPrefix(rb, remote+"/") {
			continue
		}
		
		// Skip HEAD
		if strings.HasSuffix(rb, "/HEAD") {
			continue
		}
		
		// Check if there's a merge-base
		_, err := a.runner.Run(ctx, "git", "merge-base", rb, "HEAD")
		if err == nil {
			// Extract branch name
			branch := strings.TrimPrefix(rb, remote+"/")
			return branch, nil
		}
	}
	
	// Fallback to default branch detection
	return a.GetDefaultBranch(ctx, remote)
}

// defaultProviderDetector implements provider detection
type defaultProviderDetector struct {
	configDetector   *provider.ConfigDetector
	hotReloadManager *config.HotReloadManager
}

func newDefaultProviderDetector() *defaultProviderDetector {
	configDir := os.Getenv("HOME")
	if configDir == "" {
		configDir = "."
	}
	configDir = filepath.Join(configDir, ".config")

	configPath := filepath.Join(configDir, "catmit", "providers.yaml")

	configManager, err := config.NewYAMLConfigManager(configPath)
	if err != nil {
		configManager = nil
	} else {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if createErr := configManager.CreateDefaultConfig(); createErr != nil {
				configManager = nil
			}
		}
	}

	var hotReloadManager *config.HotReloadManager
	if configManager != nil {
		hotReloadManager, err = config.NewHotReloadManager(configManager, configPath)
		if err != nil {
			hotReloadManager = nil
		} else {
			configManager = hotReloadManager
		}
	}

	return &defaultProviderDetector{
		configDetector:   provider.NewConfigDetector(configManager),
		hotReloadManager: hotReloadManager,
	}
}

func (d *defaultProviderDetector) DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error) {
	return d.configDetector.DetectFromRemote(ctx, remoteURL)
}

// DefaultCLIDetector implements CLI detection
// Exported for testing
type DefaultCLIDetector struct {
	detector *cli.Detector
}

func (d *DefaultCLIDetector) DetectCLI(ctx context.Context, providerName string) (cli.CLIStatus, error) {
	if d.detector == nil {
		d.detector = cli.NewDetector(nil)
	}
	return d.detector.DetectCLI(ctx, providerName)
}

func (d *DefaultCLIDetector) CheckMinVersion(current, minimum string) (bool, error) {
	if d.detector == nil {
		d.detector = cli.NewDetector(nil)
	}
	return d.detector.CheckMinVersion(current, minimum)
}

func (d *DefaultCLIDetector) SuggestInstallCommand(cliName string) []string {
	if d.detector == nil {
		d.detector = cli.NewDetector(nil)
	}
	return d.detector.SuggestInstallCommand(cliName)
}

// DefaultCommandRunner implements pr.CommandRunner
// Exported for testing
type DefaultCommandRunner struct {
	debug  bool
	logger *zap.Logger
}

func (r *DefaultCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.debug && r.logger != nil {
		r.logger.Debug("Executing command",
			zap.String("command", name),
			zap.Strings("args", args))
	}
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if r.debug && r.logger != nil {
		r.logger.Debug("Command completed",
			zap.String("command", name),
			zap.Int("output_length", len(output)),
			zap.Error(err))
		if len(output) > 0 && len(output) < 2000 {
			r.logger.Debug("Command output",
				zap.String("output", string(output)))
		}
	}
	return output, err
}

// GetDefaultProviderDetector returns a default provider detector instance
// This is exported for use in auth commands
func GetDefaultProviderDetector() pr.ProviderDetector {
	return newDefaultProviderDetector()
}

// LLMClientAdapter adapts llm.Client to pr.LLMInterface
type LLMClientAdapter struct {
	client *llm.Client
}

// GetCommitMessage implements pr.LLMInterface
func (a *LLMClientAdapter) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return a.client.GetCommitMessage(ctx, systemPrompt, userPrompt)
}

// Test helpers for accessing internal components


// GetTestGitRunner returns a test git runner
func GetTestGitRunner(debug bool) *GitRunnerAdapter {
	return &GitRunnerAdapter{runner: git.NewRunner(debug), debug: debug}
}

