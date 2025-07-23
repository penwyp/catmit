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

// clientAdapter adapts llm.Client to squash.ClientInterface
type clientAdapter struct {
	client *llm.Client
}

func (a *clientAdapter) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	// The squash module passes the full prompt, which needs to be split into system and user parts.
	// For simplicity, we use the entire prompt as the user prompt and leave the system prompt empty.
	return a.client.GetCommitMessage(ctx, "", prompt)
}

var (
	squashYes        bool   // skip confirmation and output directly
	squashLang       string // output language (en/zh)
	squashTimeout    int    // timeout in seconds
	squashAppendMode bool   // use append mode (non-clearing console)
	squashRebase     bool   // modify local commit history by squashing unpushed commits
	squashDebug      bool   // enable debug output for troubleshooting
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
  catmit squash --yes --lang zh

  # Custom timeout
  catmit squash --timeout 60`,
	RunE: runSquash,
}

func init() {
	rootCmd.AddCommand(squashCmd)

	squashCmd.Flags().BoolVarP(&squashYes, "yes", "y", false, "Skip confirmation and output directly")
	squashCmd.Flags().StringVarP(&squashLang, "lang", "l", "en", "Output language (en/zh)")
	squashCmd.Flags().IntVarP(&squashTimeout, "timeout", "t", 30, "Timeout in seconds")
	squashCmd.Flags().BoolVar(&squashAppendMode, "append-mode", false, "Use append mode (non-clearing console)")
	squashCmd.Flags().BoolVarP(&squashRebase, "rebase", "r", false, "Modify local commit history by squashing unpushed commits")
	squashCmd.Flags().BoolVar(&squashDebug, "debug", false, "Enable debug output for troubleshooting")
}

func runSquash(cmd *cobra.Command, args []string) error {
	// Initialize logger first
	appLogger, err := logger.New(squashDebug)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = appLogger.Sync() }()

	// Create dependencies
	deps := app.NewDependencies(appLogger, squashDebug)

	// Create LLM client adapter
	llmClient := &clientAdapter{client: deps.GetClient()}

	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(squashTimeout)*time.Second)
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

	// Handle yes mode
	if squashYes {
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
	// Get the default editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "catmit-squash-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpfile.Name())

	// Write prompt information
	_, err = tmpfile.WriteString("# Enter commit messages, one per line\n# Lines starting with # will be ignored\n# Save and exit when done\n\n")
	if err != nil {
		return nil, err
	}
	tmpfile.Close()

	// Open the editor
	editorCmd := exec.Command(editor, tmpfile.Name())
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return nil, err
	}

	// Read file content
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	// Parse content
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
	runner := git.NewRunnerWithLogger(squashDebug, logger)

	// Create git remote manager for branch detection
	remoteManager := git.NewRemoteManager(runner)

	// Get default branch name
	baseBranch := "main" // fallback
	remotes, err := remoteManager.GetRemotes(ctx)
	if err == nil {
		// Select origin remote or first available remote
		selectedRemote, err := remoteManager.SelectRemote(remotes, "origin")
		if err == nil {
			// Try to detect the default branch
			if detectedBranch, err := remoteManager.GetDefaultBranch(ctx, selectedRemote.Name); err == nil {
				baseBranch = detectedBranch
			}
		}
	}

	// Create git history manager
	history := githistory.New(runner)

	// Create rebase workflow config
	config := rebase.Config{
		BaseBranch: baseBranch,
		Language:   squashLang,
		Logger:     logger,
	}

	// Create rebase workflow
	workflow := rebase.New(history, llmClient, config)

	// Handle yes mode differently
	if squashYes {
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
