package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/client"
	"github.com/penwyp/catmit/collector"
	"github.com/penwyp/catmit/prompt"
	"github.com/penwyp/catmit/ui"
	"github.com/spf13/cobra"
)

// 将关键依赖抽象为接口以便测试时注入 Mock。
// 若在运行时未被替换，则使用默认实现。
var (
	collectorProvider func() collectorInterface                   = defaultCollectorProvider
	promptProvider    func(lang string) promptInterface           = defaultPromptProvider
	clientProvider    func(timeout time.Duration) clientInterface = defaultClientProvider
	committer         commitInterface                             = defaultCommitter{}
)

type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	Diff(ctx context.Context) (string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
}

type promptInterface interface {
	Build(seed, diff string, commits []string, branch string, files []string) string
	BuildSystemPrompt() string
	BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string
}

type clientInterface interface {
	GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	GetCommitMessageLegacy(ctx context.Context, prompt string) (string, error)
}

type commitInterface interface {
	Commit(message string) error
	Push() error
}

// ---------------- 默认实现 ------------------
func defaultCollectorProvider() collectorInterface {
	// 使用真实 Runner（os/exec）实现，后续补充。
	return collector.New(realRunner{debug: flagDebug})
}

func defaultPromptProvider(lang string) promptInterface {
	return prompt.NewBuilder(lang, 0)
}

func defaultClientProvider(timeout time.Duration) clientInterface {
	baseURL := os.Getenv("DEEPSEEK_API_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	return client.NewClient(baseURL, apiKey, timeout)
}

// realRunner 实际执行系统命令；仅在生产模式使用。
type realRunner struct {
	debug bool
}

func (r realRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Running: %s %v\n", name, args)
	}
	output, err := cmd.CombinedOutput()
	if r.debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Output length: %d bytes\n", len(output))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[DEBUG] Error: %v\n", err)
		}
		if len(output) > 0 && len(output) < 1000 {
			fmt.Fprintf(os.Stderr, "[DEBUG] Output: %q\n", string(output))
		}
	}
	return output, err
}

// defaultCommitter 使用 git commit -m 执行提交。

type defaultCommitter struct{}

func (defaultCommitter) Commit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (defaultCommitter) Push() error {
	cmd := exec.Command("git", "push")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// -------------------------------------------------

var rootCmd = &cobra.Command{
	Use:   "catmit [SEED_TEXT]",
	Short: "AI-powered commit message generator",
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
)

func init() {
	rootCmd.Flags().StringVarP(&flagLang, "lang", "l", "en", "commit message language (ISO 639-1)")
	rootCmd.Flags().IntVarP(&flagTimeout, "timeout", "t", 20, "API timeout in seconds")
	rootCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation and commit immediately")
	rootCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "print message but do not commit")
	rootCmd.Flags().BoolVar(&flagDebug, "debug", false, "enable debug output for troubleshooting")
	rootCmd.Flags().BoolVarP(&flagPush, "push", "p", false, "automatically push after successful commit")
	rootCmd.Flags().BoolVar(&flagStageAll, "stage-all", true, "automatically stage all changes if none are staged")
}

func Execute() error { return rootCmd.Execute() }

func run(cmd *cobra.Command, args []string) error {
	seedText := ""
	if len(args) > 0 {
		seedText = args[0]
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(flagTimeout)*time.Second)
	defer cancel()

	// Dry-run 与 -y 快速路径，保留同步逻辑
	if flagDryRun || flagYes {
		// 执行同步流程
		col := collectorProvider()
		diffText, err := col.Diff(ctx)
		if err != nil {
			if err == collector.ErrNoDiff {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
				if flagDebug {
					fmt.Fprintln(cmd.ErrOrStderr(), "[DEBUG] No staged or unstaged changes detected by git commands")
				}
				return nil
			}
			if flagDebug {
				fmt.Fprintf(cmd.ErrOrStderr(), "[DEBUG] Diff collection failed: %v\n", err)
			}
			return fmt.Errorf("failed to collect git diff: %w", err)
		}
		commits, err := col.RecentCommits(ctx, 10)
		if err != nil {
			return err
		}
		builder := promptProvider(flagLang)
		systemPrompt := builder.BuildSystemPrompt()
		userPrompt := builder.BuildUserPrompt(seedText, diffText, commits, "", []string{})
		cli := clientProvider(time.Duration(flagTimeout) * time.Second)
		message, err := cli.GetCommitMessage(ctx, systemPrompt, userPrompt)
		if err != nil {
			return err
		}

		if flagDryRun {
			fmt.Fprintln(cmd.OutOrStdout(), message)
			return nil
		}

		// yes = commit
		fmt.Fprintln(cmd.OutOrStdout(), "Committing...")
		// Only stage all if there are no staged changes and flagStageAll is true
		if flagStageAll && !hasStagedChanges() {
			if err := stageAll(); err != nil {
				return err
			}
		}
		if err := committer.Commit(message); err != nil {
			return err
		}
		if flagPush {
			fmt.Fprintln(cmd.OutOrStdout(), "Pushing...")
			if err := committer.Push(); err != nil {
				return fmt.Errorf("push failed: %w", err)
			}
		}
		return nil
	}

	// 交互模式：显示进度 Spinner，然后进入 Review TUI
	lm := ui.NewLoadingModel(ctx, collectorProvider(), promptProvider(flagLang), clientProvider(time.Duration(flagTimeout)*time.Second), seedText, flagLang)
	finalLM, errProgram := tea.NewProgram(&lm).Run()
	if errProgram != nil {
		return errProgram
	}
	if flm, ok := finalLM.(*ui.LoadingModel); ok {
		lm = *flm
	}
	msg, err := lm.IsDone()
	if err != nil {
		// Check if it's the "nothing to commit" error
		if err == collector.ErrNoDiff {
			fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
			return nil
		}
		return err
	}

	// 无 diff
	if msg == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to commit.")
		return nil
	}

	reviewModel := ui.NewReviewModel(msg)
	finalModel, err := tea.NewProgram(reviewModel).Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(ui.ReviewModel); ok {
		_, decision, finalMsg := m.IsDone()
		switch decision {
		case ui.DecisionAccept:
			// Only stage all if there are no staged changes and flagStageAll is true
			if flagStageAll && !hasStagedChanges() {
				if err := stageAll(); err != nil {
					return err
				}
			}
			if err := committer.Commit(finalMsg); err != nil {
				return err
			}
			if flagPush {
				fmt.Fprintln(cmd.OutOrStdout(), "Pushing...")
				if err := committer.Push(); err != nil {
					return fmt.Errorf("push failed: %w", err)
				}
			}
			return nil
		case ui.DecisionCancel:
			fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
		}
	}
	return nil
}

// stage all changes (tracked and untracked)
func stageAll() error {
	cmd := exec.Command("git", "add", "-A")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// hasStagedChanges checks if there are any staged changes
func hasStagedChanges() bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	err := cmd.Run()
	// git diff --cached --quiet returns exit code 1 if there are staged changes
	return err != nil
}
