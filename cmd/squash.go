package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/rebase"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/penwyp/catmit/pkg/llm"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/term"
)

// clientAdapter 适配 llm.Client 到 squash.ClientInterface
type clientAdapter struct {
	client *llm.Client
}

func (a *clientAdapter) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	// squash 模块传入的是完整的 prompt，需要分成 system 和 user 部分
	// 简单起见，我们将整个 prompt 作为 user prompt，system prompt 为空
	return a.client.GetCommitMessage(ctx, "", prompt)
}

var (
	squashNoConfirm  bool
	squashLang       string
	squashTimeout    int
	squashAppendMode bool
	squashRebase     bool
)

var squashCmd = &cobra.Command{
	Use:   "squash",
	Short: "Consolidate multiple commit messages into one",
	Long: `Squash command helps you consolidate multiple commit messages into a single,
comprehensive commit message using LLM.

Opens your default editor to input commit messages (one per line).
The generated message will be automatically copied to your clipboard.`,
	Example: `  # Default mode (opens editor)
  catmit squash
  
  # No confirmation with Chinese output
  catmit squash --no-confirm --lang zh
  
  # Custom timeout
  catmit squash --timeout 60`,
	RunE: runSquash,
}

func init() {
	rootCmd.AddCommand(squashCmd)

	squashCmd.Flags().BoolVarP(&squashNoConfirm, "no-confirm", "n", false, "Skip confirmation and output directly")
	squashCmd.Flags().StringVarP(&squashLang, "lang", "l", "en", "Output language (en/zh)")
	squashCmd.Flags().IntVarP(&squashTimeout, "timeout", "t", 30, "Timeout in seconds")
	squashCmd.Flags().BoolVar(&squashAppendMode, "append-mode", false, "Use append mode (non-clearing console)")
	squashCmd.Flags().BoolVarP(&squashRebase, "rebase", "r", false, "Modify local commit history by squashing unpushed commits")
}

func runSquash(cmd *cobra.Command, args []string) error {
	// Initialize logger first
	appLogger, err := logger.New(flagDebug)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = appLogger.Sync() }()

	// Create dependencies
	deps := app.NewDependencies(appLogger, flagDebug)

	// Create LLM client adapter
	llmClient := &clientAdapter{client: deps.GetClient()}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(squashTimeout)*time.Second)
	defer cancel()

	// Handle rebase mode
	if squashRebase {
		return runRebaseSquash(ctx, llmClient, appLogger)
	}

	// Default editor mode
	messages, err := getMessagesFromEditor()
	if err != nil {
		return fmt.Errorf("failed to get messages from editor: %w", err)
	}

	if len(messages) < 2 {
		return fmt.Errorf("at least 2 commit messages are required")
	}

	// Create squash instance
	squashInstance := squash.New(llmClient, squashLang)

	// Handle no-confirm mode
	if squashNoConfirm {
		result, err := squashInstance.Generate(ctx, messages)
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		fmt.Println(result)

		// Try to copy to clipboard
		if err := clipboard.WriteAll(result); err == nil {
			fmt.Fprintln(os.Stderr, "✓ Copied to clipboard")
		}

		return nil
	}

	// TUI mode
	model := ui.NewSquashModel(squashInstance, messages, squashAppendMode)
	if err := model.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// If user accepted the result, print it
	if model.IsAccepted() {
		fmt.Println(model.GetResult())
		if model.IsCopySuccess() {
			fmt.Fprintln(os.Stderr, "✓ Copied to clipboard")
		}
	}

	return nil
}

func getMessagesFromEditor() ([]string, error) {
	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// stdin is piped, read from it directly
		return readMessagesFromStdin()
	}

	// Interactive mode - use editor
	// 获取默认编辑器
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "catmit-squash-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpfile.Name())

	// 写入提示信息
	_, err = tmpfile.WriteString("# Enter commit messages, one per line\n# Lines starting with # will be ignored\n# Save and exit when done\n\n")
	if err != nil {
		return nil, err
	}
	tmpfile.Close()

	// 打开编辑器
	editorCmd := exec.Command(editor, tmpfile.Name())
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return nil, err
	}

	// 读取文件内容
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	// 解析内容
	var messages []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			messages = append(messages, line)
		}
	}

	return messages, scanner.Err()
}

func readMessagesFromStdin() ([]string, error) {
	var messages []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			messages = append(messages, line)
		}
	}
	return messages, scanner.Err()
}

func runRebaseSquash(ctx context.Context, llmClient squash.ClientInterface, logger *zap.Logger) error {
	// Create git runner
	runner := git.NewRunnerWithLogger(flagDebug, logger)
	
	// Create git history manager
	history := githistory.New(runner)
	
	// Create rebase workflow config
	config := rebase.Config{
		BaseBranch: "main", // TODO: Make this configurable or auto-detect
		Language:   squashLang,
		Logger:     logger,
	}
	
	// Create rebase workflow
	workflow := rebase.New(history, llmClient, config)
	
	// Handle no-confirm mode differently
	if squashNoConfirm {
		// Analyze the repository
		analysis, err := workflow.Analyze(ctx)
		if err != nil {
			return fmt.Errorf("failed to analyze repository: %w", err)
		}
		
		if !analysis.CanRebase {
			fmt.Println(analysis.Message)
			return nil
		}
		
		// Generate commit message
		message, err := workflow.GenerateCommitMessage(ctx, analysis.UnpushedCommits)
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}
		
		fmt.Println("Generated commit message:")
		fmt.Println(message)
		
		// Execute rebase
		fmt.Println("\nExecuting rebase...")
		if err := workflow.ExecuteRebase(ctx, analysis, message); err != nil {
			return fmt.Errorf("rebase failed: %w", err)
		}
		
		fmt.Println("✓ Rebase completed successfully")
		return nil
	}
	
	// TUI mode
	model := ui.NewRebaseModel(workflow, squashAppendMode)
	if err := model.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	
	// If user accepted and rebase was successful
	if model.IsAccepted() {
		fmt.Println("\n✓ Rebase completed successfully")
		if backupBranch := model.GetBackupBranch(); backupBranch != "" {
			fmt.Printf("Backup branch: %s\n", backupBranch)
		}
	}
	
	return nil
}
