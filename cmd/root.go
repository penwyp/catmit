package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/client"
	"github.com/penwyp/catmit/collector"
	"github.com/penwyp/catmit/internal/logger"
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

// ErrPRAlreadyExists is returned when a PR already exists for the branch
type ErrPRAlreadyExists struct {
	URL string
}

func (e *ErrPRAlreadyExists) Error() string {
	return fmt.Sprintf("pull request already exists: %s", e.URL)
}

// 将关键依赖抽象为接口以便测试时注入 Mock。
// 若在运行时未被替换，则使用默认实现。
var (
	collectorProvider func() collectorInterface                   = defaultCollectorProvider
	promptProvider    func(lang string) promptInterface           = defaultPromptProvider
	clientProvider    func() clientInterface                      = defaultClientProvider
	committer         commitInterface                             = defaultCommitter{}
	appLogger         *zap.Logger                                 // 全局日志记录器
)

type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	Diff(ctx context.Context) (string, error)
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

type defaultCommitter struct{}

func (defaultCommitter) Commit(ctx context.Context, message string) error {
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d defaultCommitter) Push(ctx context.Context) error {
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
		return fmt.Errorf("git push failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (defaultCommitter) StageAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (defaultCommitter) HasStagedChanges(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	err := cmd.Run()
	// git diff --cached --quiet returns exit code 1 if there are staged changes
	return err != nil
}

func (d defaultCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	if appLogger != nil {
		appLogger.Debug("Creating GitHub pull request")
	}
	
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found: %w. Please install GitHub CLI: https://cli.github.com/", err)
	}
	
	// Execute gh pr create command
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--fill", "--base", "main", "--draft=false")
	output, err := cmd.CombinedOutput()
	
	if appLogger != nil {
		if err != nil {
			appLogger.Debug("GitHub PR creation failed", 
				zap.Error(err),
				zap.String("output", string(output)))
		} else {
			appLogger.Debug("GitHub PR created successfully", 
				zap.String("output", string(output)))
		}
	}
	
	if err != nil {
		// Check if PR already exists
		outputStr := string(output)
		if strings.Contains(outputStr, "already exists") {
			// Extract the existing PR URL
			existingPRURL := extractPRURL(outputStr)
			if existingPRURL != "" {
				return "", &ErrPRAlreadyExists{URL: existingPRURL}
			}
		}
		return "", fmt.Errorf("failed to create pull request: %w\nOutput: %s", err, outputStr)
	}
	
	// Extract PR URL from output
	prURL := extractPRURL(string(output))
	
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

func (defaultCommitter) NeedsPush(ctx context.Context) (bool, error) {
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
		return false, fmt.Errorf("failed to check unpushed commits: %w", err)
	}
	
	// Parse the count
	countStr := strings.TrimSpace(string(output))
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return false, fmt.Errorf("failed to parse commit count: %w", err)
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
		return collector.ErrNotGitRepository
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
	flagCreatePR bool
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
	rootCmd.Flags().BoolVar(&flagCreatePR, "create-pr", false, "create GitHub pull request after successful push")
}

func Execute() error { return rootCmd.Execute() }

func ExecuteContext(ctx context.Context) error { return rootCmd.ExecuteContext(ctx) }

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
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = appLogger.Sync() }()

	seedText := ""
	if len(args) > 0 {
		seedText = args[0]
	}

	ctx := cmd.Context()

	// Early check: ensure we're in a git repository
	if err := checkGitRepository(ctx); err != nil {
		if errors.Is(err, collector.ErrNotGitRepository) {
			// Set both SilenceUsage and SilenceErrors to prevent Cobra's error output
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			// Print user-friendly error message directly
			_, _ = fmt.Fprintln(cmd.OutOrStderr(), getGitRepositoryErrorMessage(flagLang))
			os.Exit(1) // Exit directly to avoid any additional error output
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
			if err == collector.ErrNoDiff {
				if flagCreatePR {
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
							return fmt.Errorf("failed to push branch: %w", err)
						}
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Branch pushed successfully", true))
					}
					
					// Even with no changes, allow creating a PR if explicitly requested
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Creating pull request...", false))
					prURL, err := committer.CreatePullRequest(ctx)
					if err != nil {
						var prExists *ErrPRAlreadyExists
						if errors.As(err, &prExists) {
							_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request already exists", true))
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prExists.URL)
							return nil
						}
						return fmt.Errorf("failed to create pull request: %w", err)
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
			if errors.Is(err, collector.ErrNotGitRepository) {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				_, _ = fmt.Fprintln(cmd.OutOrStderr(), getGitRepositoryErrorMessage(flagLang))
				os.Exit(1)
			}
			if flagDebug {
				appLogger.Debug("Comprehensive diff collection failed, trying fallback", zap.Error(err))
			}
			// Fallback to legacy diff for backward compatibility
			diffText, err = col.Diff(ctx)
			if err != nil {
				if err == collector.ErrNoDiff {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
					return nil
				}
				if errors.Is(err, collector.ErrNotGitRepository) {
					cmd.SilenceUsage = true
					cmd.SilenceErrors = true
					_, _ = fmt.Fprintln(cmd.OutOrStderr(), getGitRepositoryErrorMessage(flagLang))
					return fmt.Errorf("git repository required")
				}
				return fmt.Errorf("failed to collect git diff: %w", err)
			}
		}
		commits, err := col.RecentCommits(ctx, 10)
		if err != nil {
			if errors.Is(err, collector.ErrNotGitRepository) {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				_, _ = fmt.Fprintln(cmd.OutOrStderr(), getGitRepositoryErrorMessage(flagLang))
				os.Exit(1)
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
				return fmt.Errorf("push failed: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pushed successfully", true))
			
			// Create pull request if requested
			if flagCreatePR {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Creating pull request...", false))
				prURL, err := committer.CreatePullRequest(ctx)
				if err != nil {
					var prExists *ErrPRAlreadyExists
					if errors.As(err, &prExists) {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request already exists", true))
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prExists.URL)
						return nil
					}
					return fmt.Errorf("failed to create pull request: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderStatusBar("Pull request created successfully", true))
				if prURL != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", prURL)
				}
			}
		}
		return nil
	}

	// 交互模式：使用统一的MainModel
	mainModel := ui.NewMainModel(
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
		flagCreatePR,
	)
	
	finalModel, err := tea.NewProgram(mainModel).Run()
	if err != nil {
		return err
	}

	m, ok := finalModel.(*ui.MainModel)
	if !ok {
		return fmt.Errorf("internal error: unexpected model type, got %T", finalModel)
	}
	
	done, decision, _, err := m.IsDone()
	if err != nil {
		// Check if it's the "nothing to commit" error
		if err == collector.ErrNoDiff {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
			return nil
		}
		// Check if it's a git repository error
		if errors.Is(err, collector.ErrNotGitRepository) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			_, _ = fmt.Fprintln(cmd.OutOrStderr(), getGitRepositoryErrorMessage(flagLang))
			os.Exit(1)
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
