package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/client"
	"github.com/penwyp/catmit/collector"
	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/config"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/penwyp/catmit/internal/template"
	"github.com/penwyp/catmit/prompt"
	"github.com/penwyp/catmit/ui"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// version holds the current version of catmit
// This will be set at build time via ldflags
var version = "dev"

// GetVersionString returns a formatted version string
func GetVersionString() string {
	return fmt.Sprintf("catmit version %s", version)
}


// 将关键依赖抽象为接口以便测试时注入 Mock。
// 若在运行时未被替换，则使用默认实现。
var (
	collectorProvider func() collectorInterface                   = defaultCollectorProvider
	promptProvider    func(lang string) promptInterface           = defaultPromptProvider
	clientProvider    func() clientInterface                      = defaultClientProvider
	committer         commitInterface                             // Will be initialized in init()
	appLogger         *zap.Logger                                 // 全局日志记录器
)

type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
	FileStatusSummary(ctx context.Context) (*collector.FileStatusSummary, error)
	// Enhanced methods for comprehensive diff support
	ComprehensiveDiff(ctx context.Context) (string, error)
	AnalyzeChanges(ctx context.Context) (*collector.ChangesSummary, error)
}

type promptInterface interface {
	Build(seed, diff string, commits []string, branch string, files []string) string
	BuildSystemPrompt() string
	BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string
	BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error)
}

type clientInterface interface {
	GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type commitInterface interface {
	Commit(ctx context.Context, message string) error
	Push(ctx context.Context) error
	StageAll(ctx context.Context) error
	HasStagedChanges(ctx context.Context) bool
	CreatePullRequest(ctx context.Context) (string, error)
	NeedsPush(ctx context.Context) (bool, error)
}

// ---------------- 默认实现 ------------------
func defaultCollectorProvider() collectorInterface {
	// 使用真实 Runner（os/exec）实现，后续补充。
	return collector.New(realRunner{debug: flagDebug})
}

func defaultPromptProvider(lang string) promptInterface {
	return prompt.NewBuilder(lang, 0)
}

func defaultClientProvider() clientInterface {
	// 使用新的通用 Client，它会自动从环境变量读取配置
	return client.NewClient(appLogger)
}

// realRunner 实际执行系统命令；仅在生产模式使用。
type realRunner struct {
	debug bool
}

func (r realRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.debug {
		appLogger.Debug("Running command",
			zap.String("command", name),
			zap.Strings("args", args))
	}
	output, err := cmd.CombinedOutput()
	if r.debug {
		appLogger.Debug("Command output",
			zap.Int("output_length", len(output)),
			zap.Error(err),
			zap.String("output", func() string {
				if len(output) > 0 && len(output) < 1000 {
					return string(output)
				}
				return fmt.Sprintf("<%d bytes>", len(output))
			}()))
	}
	return output, err
}

// defaultCommitter 使用 git commit -m 执行提交。
type defaultCommitter struct {
	prCreator *pr.Creator
	ctx       context.Context
	message   string   // 保存commit message用于模板
}

// newDefaultCommitter creates a new defaultCommitter with PR support
func newDefaultCommitter() *defaultCommitter {
	// Initialize PR creator with default implementations
	gitRunner := &defaultGitRunner{}
	providerDetector := newDefaultProviderDetector()
	cliDetector := &defaultCLIDetector{}
	
	// Create command builder and runner
	commandBuilder := pr.NewCommandBuilder()
	commandRunner := &defaultCommandRunner{debug: flagDebug}
	
	prCreator := pr.NewCreator(
		gitRunner,
		providerDetector,
		cliDetector,
		commandBuilder,
		commandRunner,
	)
	
	// 如果启用了模板支持，添加模板管理器
	if flagPRTemplate {
		// 获取仓库根目录
		repoRoot, err := template.FindRepositoryRoot()
		if err == nil {
			templateManager := template.NewDefaultManager(repoRoot)
			prCreator.WithTemplateManager(templateManager)
		}
	}
	
	return &defaultCommitter{
		prCreator: prCreator,
	}
}

// defaultCommandRunner implements pr.CommandRunner
type defaultCommandRunner struct {
	debug bool
}

func (r *defaultCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.debug {
		appLogger.Debug("Running command for PR",
			zap.String("command", name),
			zap.Strings("args", args))
	}
	output, err := cmd.CombinedOutput()
	if r.debug {
		appLogger.Debug("PR command output",
			zap.Int("output_length", len(output)),
			zap.Error(err))
	}
	return output, err
}

func (d *defaultCommitter) Commit(ctx context.Context, message string) error {
	// 保存message和context用于后续PR创建
	d.ctx = ctx
	d.message = message
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *defaultCommitter) Push(ctx context.Context) error {
	if appLogger != nil {
		appLogger.Debug("Executing git push command")
	}
	cmd := exec.CommandContext(ctx, "git", "push")
	// Capture output instead of connecting directly to terminal
	// This prevents error messages from bypassing the TUI
	output, err := cmd.CombinedOutput()
	if appLogger != nil {
		if err != nil {
			appLogger.Debug("Git push failed", 
				zap.Error(err),
				zap.String("output", string(output)))
		} else {
			appLogger.Debug("Git push succeeded", 
				zap.String("output", string(output)))
		}
	}
	if err != nil {
		// Include the git output in the error for better error reporting
		return errors.Wrapf(errors.ErrTypeGit, "git push failed\nOutput: %s", err, string(output))
	}
	return nil
}

func (d *defaultCommitter) StageAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (d *defaultCommitter) HasStagedChanges(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	err := cmd.Run()
	// git diff --cached --quiet returns exit code 1 if there are staged changes
	return err != nil
}

func (d *defaultCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	if appLogger != nil {
		appLogger.Debug("Creating pull request with new PR creator")
	}
	
	if d.prCreator == nil {
		return "", errors.New(errors.ErrTypePR, "PR creator not initialized")
	}
	
	// 准备模板数据（如果启用了模板）
	var templateData *template.TemplateData
	if flagPRTemplate && d.message != "" {
		// 收集文件变更信息
		col := collectorProvider()
		changedFiles, _ := col.ChangedFiles(ctx)
		branch, _ := col.BranchName(ctx)
		changesSummary, _ := col.AnalyzeChanges(ctx)
		
		// 创建模板数据
		templateData = template.CreateTemplateData(d.message, branch, changedFiles)
		
		// 丰富模板数据
		if changesSummary != nil {
			// TODO: Map changesSummary fields to templateData when template.TemplateData is updated
			// For now, we have basic data from CreateTemplateData
			templateData.FilesCount = changesSummary.TotalChangedFiles
		}
	}
	
	// Build PR options from flags
	options := pr.CreateOptions{
		Remote:       flagPRRemote,
		BaseBranch:   flagPRBase,
		Draft:        flagPRDraft,
		Fill:         true, // Always use fill for now
		UseTemplate:  flagPRTemplate,
		TemplateData: templateData,
	}
	
	// Create the PR
	prURL, err := d.prCreator.Create(ctx, options)
	if err != nil {
		return "", err
	}
	
	return prURL, nil
}

// extractPRURL extracts the PR URL from gh command output
func extractPRURL(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for GitHub PR URLs in the output
		if strings.Contains(line, "github.com") && strings.Contains(line, "/pull/") {
			return line
		}
		// Also check for just the URL part if it's at the end of a line
		if strings.HasPrefix(line, "https://github.com/") && strings.Contains(line, "/pull/") {
			return line
		}
	}
	return ""
}

func (d *defaultCommitter) NeedsPush(ctx context.Context) (bool, error) {
	// Check if the current branch has unpushed commits
	// First, check if we have an upstream branch
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	_, err := cmd.CombinedOutput()
	if err != nil {
		// No upstream branch set, so we need to push
		return true, nil
	}
	
	// Check if there are commits to push
	// git rev-list --count @{u}..HEAD
	cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", "@{u}..HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, errors.Wrap(errors.ErrTypeGit, "failed to check unpushed commits", err)
	}
	
	// Parse the count
	countStr := strings.TrimSpace(string(output))
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return false, errors.Wrap(errors.ErrTypeGit, "failed to parse commit count", err)
	}
	
	return count > 0, nil
}

// renderStatusBar 渲染带样式的状态条
func renderStatusBar(message string, isSuccess bool) string {
	var style lipgloss.Style
	if isSuccess {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).  // Green
			Background(lipgloss.Color("22")).  // Dark green
			Bold(true).
			Padding(0, 1)
	} else {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).  // Blue
			Background(lipgloss.Color("19")).  // Dark blue
			Bold(true).
			Padding(0, 1)
	}
	
	// 创建进度指示符
	indicator := "▶"
	if isSuccess {
		indicator = "✓"
	}
	
	styledMessage := style.Render(indicator + " " + message)
	return styledMessage
}

// checkGitRepository performs a quick check to see if we're in a git repository
// Returns a user-friendly error if not
func checkGitRepository(ctx context.Context) error {
	// Quick test: try to run a simple git command that would fail in non-git directories
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	if err := cmd.Run(); err != nil {
		return errors.ErrNoGitRepo
	}
	
	return nil
}

// getGitRepositoryErrorMessage returns a user-friendly error message for non-git directories
// based on the language setting
func getGitRepositoryErrorMessage(lang string) string {
	if lang == "zh" || lang == "zh-CN" || lang == "zh-TW" {
		return `catmit 需要在 Git 仓库中运行。

请确保您在 Git 仓库目录中，或运行 'git init' 创建一个新仓库。`
	}
	
	return `catmit requires a Git repository to work.

Please make sure you're in a Git repository, or run 'git init' to create one.`
}

// -------------------------------------------------

var rootCmd = &cobra.Command{
	Use:   "catmit [SEED_TEXT]",
	Short: "AI-powered commit message generator with comprehensive change analysis",
	Long: `catmit is an AI-powered tool that generates high-quality Git commit messages 
by analyzing your staged changes, unstaged modifications, and untracked files.

Features:
- Analyzes all types of changes including untracked files
- Follows Conventional Commits specification
- Smart token budgeting for large changesets
- Interactive review and editing capabilities
- Multiple language support (English/Chinese)`,
	RunE:  run,
}

var (
	flagLang     string
	flagTimeout  int
	flagYes      bool
	flagDryRun   bool
	flagDebug    bool
	flagPush     bool
	flagStageAll bool
	flagVersion  bool
	flagCreatePR bool  // Deprecated: use flagPR instead
	flagPR       bool  // New PR flag
	flagSeed     string  // Seed text for commit message generation
	
	// PR-specific flags
	flagPRRemote   string
	flagPRBase     string
	flagPRDraft    bool
	flagPRProvider string
	flagPRTemplate bool  // Enable PR template support
)

func init() {
	rootCmd.Flags().StringVarP(&flagLang, "lang", "l", "en", "commit message language (ISO 639-1)")
	rootCmd.Flags().IntVarP(&flagTimeout, "timeout", "t", 20, "API timeout in seconds")
	rootCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation and commit immediately")
	rootCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "print message but do not commit")
	rootCmd.Flags().BoolVar(&flagDebug, "debug", false, "enable debug output for troubleshooting")
	rootCmd.Flags().BoolVarP(&flagPush, "push", "p", true, "automatically push after successful commit")
	rootCmd.Flags().BoolVar(&flagStageAll, "stage-all", true, "automatically stage all changes (tracked and untracked) if none are staged")
	rootCmd.Flags().BoolVar(&flagVersion, "version", false, "show version information")
	rootCmd.Flags().BoolVar(&flagCreatePR, "create-pr", false, "create GitHub pull request after successful push (deprecated, use --pr)")
	rootCmd.Flags().BoolVarP(&flagPR, "pr", "c", false, "create pull request after successful push")
	rootCmd.Flags().StringVarP(&flagSeed, "seed", "s", "", "seed text for commit message generation")
	
	// PR-specific flags
	rootCmd.Flags().StringVar(&flagPRRemote, "pr-remote", "origin", "remote to use for pull request")
	rootCmd.Flags().StringVar(&flagPRBase, "pr-base", "", "base branch for pull request (defaults to provider's default branch)")
	rootCmd.Flags().BoolVar(&flagPRDraft, "pr-draft", false, "create pull request as draft")
	rootCmd.Flags().StringVar(&flagPRProvider, "pr-provider", "", "override detected provider (github, gitlab, gitea, bitbucket)")
	rootCmd.Flags().BoolVar(&flagPRTemplate, "pr-template", true, "use PR template if available")
	
	// Mark create-pr as deprecated
	rootCmd.Flags().MarkDeprecated("create-pr", "use --pr instead")
	
	// Add auth subcommand
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication related commands",
		Long:  `Manage authentication for PR creation with various git hosting providers`,
	}
	
	// Create auth status command with default implementations
	authStatusCmd := NewAuthStatusCommand(
		&defaultGitRunner{},
		newDefaultProviderDetector(),
		&defaultCLIDetector{},
	)
	
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

func Execute() error { return rootCmd.Execute() }

func ExecuteContext(ctx context.Context) error { return rootCmd.ExecuteContext(ctx) }

// defaultGitRunner implements GitRunner for auth command
type defaultGitRunner struct{}

func (d *defaultGitRunner) GetRemotes(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var remotes []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes, nil
}

func (d *defaultGitRunner) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remote)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *defaultGitRunner) GetCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *defaultGitRunner) GetCommitMessage(ctx context.Context, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--pretty=%B", ref)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *defaultGitRunner) GetDefaultBranch(ctx context.Context, remote string) (string, error) {
	// Try to get the default branch from the remote
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", fmt.Sprintf("refs/remotes/%s/HEAD", remote))
	output, err := cmd.Output()
	if err != nil {
		// Fallback to main/master
		return "main", nil
	}
	// Extract branch name from refs/remotes/origin/HEAD -> refs/remotes/origin/main
	parts := strings.Split(strings.TrimSpace(string(output)), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1], nil
	}
	return "main", nil
}

// defaultProviderDetector implements ProviderDetector for auth command
type defaultProviderDetector struct {
	configDetector *provider.ConfigDetector
	hotReloadManager *config.HotReloadManager
}

// newDefaultProviderDetector creates a provider detector with config support
func newDefaultProviderDetector() *defaultProviderDetector {
	// Always use ~/.config for consistency across platforms
	configDir := os.Getenv("HOME")
	if configDir == "" {
		configDir = "."
	}
	configDir = filepath.Join(configDir, ".config")
	
	// Use YAML format for better readability
	configPath := filepath.Join(configDir, "catmit", "providers.yaml")
	
	// Create YAML config manager
	configManager, err := config.NewYAMLConfigManager(configPath)
	if err != nil {
		// If we can't create the manager, work without config
		configManager = nil
	} else {
		// Check if config file exists, create default if not
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			// Create default config for user convenience
			if createErr := configManager.CreateDefaultConfig(); createErr != nil {
				// If creation fails, just work without config
				configManager = nil
			}
		}
	}
	
	var hotReloadManager *config.HotReloadManager
	if configManager != nil {
		// Wrap with hot reload capability
		hotReloadManager, err = config.NewHotReloadManager(configManager, configPath)
		if err != nil {
			// Fall back to regular config manager
			log.Printf("Failed to enable config hot reload: %v", err)
			hotReloadManager = nil
		} else {
			// Set up a callback to log config changes
			hotReloadManager.OnConfigChange(func(cfg *config.Config) {
				log.Printf("Configuration reloaded from %s", configPath)
			})
			// Use hot reload manager as the config manager
			configManager = hotReloadManager
		}
	}
	
	return &defaultProviderDetector{
		configDetector: provider.NewConfigDetector(configManager),
		hotReloadManager: hotReloadManager,
	}
}

func (d *defaultProviderDetector) DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error) {
	return d.configDetector.DetectFromRemote(ctx, remoteURL)
}


// defaultCLIDetector implements CLIDetector for auth command
type defaultCLIDetector struct{
	detector *cli.Detector
}

func (d *defaultCLIDetector) DetectCLI(ctx context.Context, providerName string) (cli.CLIStatus, error) {
	// Use the proper detector from internal/cli package
	if d.detector == nil {
		d.detector = cli.NewDetector(nil)
	}
	return d.detector.DetectCLI(ctx, providerName)
}

func (d *defaultCLIDetector) CheckMinVersion(current, minimum string) (bool, error) {
	if d.detector == nil {
		d.detector = cli.NewDetector(nil)
	}
	return d.detector.CheckMinVersion(current, minimum)
}

func (d *defaultCLIDetector) SuggestInstallCommand(cliName string) []string {
	if d.detector == nil {
		d.detector = cli.NewDetector(nil)
	}
	return d.detector.SuggestInstallCommand(cliName)
}

// isPRRequested returns true if user requested PR creation via either flag
func isPRRequested() bool {
	return flagPR || flagCreatePR
}

func run(cmd *cobra.Command, args []string) error {
	// Handle version flag
	if flagVersion {
		fmt.Println(GetVersionString())
		return nil
	}

	// Initialize logger
	var err error
	appLogger, err = logger.New(flagDebug)
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to initialize logger", err)
	}
	defer func() { _ = appLogger.Sync() }()
	
	// Initialize committer with PR support after logger is available
	if committer == nil {
		committer = newDefaultCommitter()
	}

	// Prioritize --seed flag over positional argument
	seedText := flagSeed
	if seedText == "" && len(args) > 0 {
		seedText = args[0]
	}

	ctx := cmd.Context()

	// Show deprecation warning if --create-pr is used
	if flagCreatePR {
		_, _ = fmt.Fprintln(cmd.OutOrStderr(), "⚠️  Warning: --create-pr is deprecated, please use --pr instead")
	}

	// Early check: ensure we're in a git repository
	if err := checkGitRepository(ctx); err != nil {
		if errors.Is(err, errors.ErrNoGitRepo) {
			// Set both SilenceUsage and SilenceErrors to prevent Cobra's error output
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			// Use error handler for proper exit code
			errors.HandleFatal(err)
		}
		// For other errors, let the normal error handling proceed
		if flagDebug {
			appLogger.Debug("Git repository check failed", zap.Error(err))
		}
	}

	// Dry-run 与 -y 快速路径，保留同步逻辑
	if flagDryRun || flagYes {
		// 执行同步流程
		col := collectorProvider()
		
		// Use ComprehensiveDiff to include untracked files
		diffText, err := col.ComprehensiveDiff(ctx)
		if err != nil {
			if errors.Is(err, collector.ErrNoDiff) {
				if isPRRequested() {
					// Check if we need to push first
					needsPush, err := committer.NeedsPush(ctx)
					if err != nil {
						if flagDebug {
							appLogger.Debug("Failed to check if push is needed", zap.Error(err))
						}
						// Continue anyway, let the PR creation fail if needed
						needsPush = false
					}
					
					if needsPush {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pushing branch...", false))
						if err := committer.Push(ctx); err != nil {
							return errors.Wrap(errors.ErrTypeGit, "failed to push branch", err)
						}
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Branch pushed successfully", true))
					}
					
					// Even with no changes, allow creating a PR if explicitly requested
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Creating pull request...", false))
					prURL, err := committer.CreatePullRequest(ctx)
					if err != nil {
						var prExists *pr.ErrPRAlreadyExists
						if errors.As(err, &prExists) {
							_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request already exists", true))
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prExists.URL)
							return nil
						}
						return errors.Wrap(errors.ErrTypePR, "failed to create pull request", err)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request created successfully", true))
					if prURL != "" {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prURL)
					}
					return nil
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
				if flagDebug {
					appLogger.Debug("No staged, unstaged, or untracked changes detected")
				}
				return nil
			}
			if errors.Is(err, errors.ErrNoGitRepo) {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				errors.HandleFatal(err)
			}
			return errors.Wrap(errors.ErrTypeGit, "failed to collect git diff", err)
		}
		commits, err := col.RecentCommits(ctx, 10)
		if err != nil {
			if errors.Is(err, errors.ErrNoGitRepo) {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				errors.HandleFatal(err)
			}
			return err
		}
		builder := promptProvider(flagLang)
		systemPrompt := builder.BuildSystemPrompt()
		
		// Try to use the new BuildUserPromptWithBudget method
		userPrompt, err := builder.BuildUserPromptWithBudget(ctx, col, seedText)
		if err != nil {
			if flagDebug {
				appLogger.Debug("Smart prompt building failed, falling back to traditional method", zap.Error(err))
			}
			// Fallback to traditional method
			branch, _ := col.BranchName(ctx)
			files, _ := col.ChangedFiles(ctx)
			userPrompt = builder.BuildUserPrompt(seedText, diffText, commits, branch, files)
		}
		
		cli := clientProvider()
		// Create timeout context only for API call
		apiCtx, apiCancel := context.WithTimeout(ctx, time.Duration(flagTimeout)*time.Second)
		defer apiCancel()
		message, err := cli.GetCommitMessage(apiCtx, systemPrompt, userPrompt)
		if err != nil {
			return err
		}

		if flagDryRun {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), message)
			return nil
		}

		// yes = commit
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Committing...", false))
		// Only stage all if there are no staged changes and flagStageAll is true
		if flagStageAll && !hasStagedChanges(ctx) {
			if err := stageAll(ctx); err != nil {
				return err
			}
		}
		if err := committer.Commit(ctx, message); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Committed successfully", true))
		if flagPush {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pushing...", false))
			if err := committer.Push(ctx); err != nil {
				return errors.Wrap(errors.ErrTypeGit, "push failed", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pushed successfully", true))
		}
		
		// Create pull request if requested (after push or commit)
		if isPRRequested() {
			// Check if we need to push first
			if !flagPush {
				needsPush, err := committer.NeedsPush(ctx)
				if err != nil {
					if flagDebug {
						appLogger.Debug("Failed to check if push is needed", zap.Error(err))
					}
					// Continue anyway, let the PR creation fail if needed
					needsPush = false
				}
				
				if needsPush {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pushing branch for PR...", false))
					if err := committer.Push(ctx); err != nil {
						return errors.Wrap(errors.ErrTypeGit, "failed to push branch", err)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Branch pushed successfully", true))
				}
			}
			
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Creating pull request...", false))
			prURL, err := committer.CreatePullRequest(ctx)
			if err != nil {
				var prExists *pr.ErrPRAlreadyExists
				if errors.As(err, &prExists) {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request already exists", true))
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prExists.URL)
					return nil
				}
				return errors.Wrap(errors.ErrTypePR, "failed to create pull request", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request created successfully", true))
			if prURL != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prURL)
			}
		}
		return nil
	}

	// 交互模式：使用统一的MainModel
	prConfig := ui.PRConfig{
		CreatePR:    isPRRequested(),
		Remote:      flagPRRemote,
		Base:        flagPRBase,
		Draft:       flagPRDraft,
		Provider:    flagPRProvider,
		UseTemplate: flagPRTemplate,
	}
	
	mainModel := ui.NewMainModelWithPRConfig(
		ctx, 
		collectorProvider(), 
		promptProvider(flagLang), 
		clientProvider(), 
		committer,
		seedText, 
		flagLang, 
		time.Duration(flagTimeout)*time.Second,
		flagPush,
		flagStageAll,
		prConfig,
	)
	
	finalModel, err := tea.NewProgram(mainModel).Run()
	if err != nil {
		return err
	}

	m, ok := finalModel.(*ui.MainModel)
	if !ok {
		return errors.Newf(errors.ErrTypeUnknown, "internal error: unexpected model type, got %T", finalModel)
	}
	
	done, decision, _, err := m.IsDone()
	if err != nil {
		// Check if it's the "nothing to commit" error
		if errors.Is(err, collector.ErrNoDiff) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
			return nil
		}
		// Check if it's a git repository error
		if errors.Is(err, errors.ErrNoGitRepo) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			errors.HandleFatal(err)
		}
		// 如果用户在加载时按 Ctrl+C 取消，则静默退出
		if err == context.Canceled {
			return nil
		}
		return err
	}
	
	if done {
		switch decision {
		case ui.DecisionAccept:
			// MainModel has already handled the staging, commit, and push operations
			return nil
		case ui.DecisionCancel:
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
		}
	}
	
	return nil
}

// stage all changes (tracked and untracked)
func stageAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// hasStagedChanges checks if there are any staged changes
func hasStagedChanges(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	err := cmd.Run()
	// git diff --cached --quiet returns exit code 1 if there are staged changes
	return err != nil
}
