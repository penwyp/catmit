package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	squashYes     bool   // skip confirmation and output directly
	squashLang    string // output language (en/zh)
	squashTimeout int    // timeout in seconds
	squashDebug   bool   // enable debug output for troubleshooting
	squashDryRun  bool   // preview without copying to clipboard
)

var squashDraftCmd = &cobra.Command{
	Use:   "squash-draft",
	Short: "Consolidate commit messages using AI",
	Long: `Merge multiple commit messages into a single, comprehensive commit message using AI.

Opens your editor to input commit messages, then generates a unified message
that captures all changes. Automatically copies the result to your clipboard.`,
	Example: `  # Default mode (opens editor)
  catmit squash-draft

  # No confirmation with Chinese output
  catmit squash-draft --yes --lang zh

  # Custom timeout
  catmit squash-draft --timeout 60`,
	RunE: runSquash,
}

func init() {
	rootCmd.AddCommand(squashDraftCmd)

	squashDraftCmd.Flags().BoolVar(&squashDebug, "debug", false, "enable debug output for troubleshooting")
	squashDraftCmd.Flags().BoolVar(&squashDryRun, "dry-run", false, "preview without executing")
	squashDraftCmd.Flags().StringVarP(&squashLang, "lang", "l", "en", "output language (en/zh)")
	squashDraftCmd.Flags().IntVarP(&squashTimeout, "timeout", "t", 30, "timeout in seconds")
	squashDraftCmd.Flags().BoolVarP(&squashYes, "yes", "y", false, "skip confirmation and output directly")
}

func runSquash(cmd *cobra.Command, args []string) error {
	// Initialize dependencies
	deps, err := initSquashDependencies(squashDebug)
	if err != nil {
		return err
	}
	defer func() { _ = deps.logger.Sync() }()

	// Create context with timeout
	ctx, cancel := createContext(cmd, squashTimeout)
	defer cancel()

	// Get messages from editor
	messages, err := getMessagesFromEditor()
	if err != nil {
		return fmt.Errorf("failed to get messages from editor: %w", err)
	}

	if len(messages) < 2 {
		return fmt.Errorf("at least 2 commit messages are required")
	}

	// Create squash instance
	squashInstance := squash.New(deps.llmClient, squashLang)

	// Handle dry-run mode
	if squashDryRun {
		result, err := squashInstance.Generate(ctx, messages)
		if err != nil {
			// Check if error is due to context timeout
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("operation timed out after %d seconds", squashTimeout)
			}
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		fmt.Println("=== DRY RUN MODE ===")
		fmt.Println("Generated commit message:")
		fmt.Println(result)
		fmt.Println("\n(Message not copied to clipboard)")
		fmt.Println("=== END DRY RUN ===")
		return nil
	}

	// Handle yes mode
	if squashYes {
		result, err := squashInstance.Generate(ctx, messages)
		if err != nil {
			// Check if error is due to context timeout
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("operation timed out after %d seconds", squashTimeout)
			}
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
	model := ui.NewSquashWorkflowModel(ctx, squashInstance, messages)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check if it's the expected model type
	squashModel, ok := finalModel.(*ui.SquashWorkflowModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	// If user accepted the result, print it
	if squashModel.IsAccepted() {
		fmt.Println(squashModel.GetResult())
		if squashModel.IsCopySuccess() {
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
	// Get the default editor with fallback chain: CATMIT_EDITOR -> EDITOR -> vim
	editor := os.Getenv("CATMIT_EDITOR")
	if editor == "" {
		editor = os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
	}

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "catmit-squash-*.txt")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.Remove(tmpfile.Name()); err != nil {
			// Log warning if temp file cleanup fails
			// Note: Without access to logger in this scope, we'll silently ignore cleanup errors
			// The OS will clean up temp files eventually
			_ = err
		}
	}()

	// Write prompt information
	_, err = tmpfile.WriteString("# Enter commit messages, one per line, Lines starting with # will be ignored, Save and exit when done\n\n")
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
