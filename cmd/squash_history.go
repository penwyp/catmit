package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	historyYes     bool   // skip confirmation and output directly
	historyLang    string // output language (en/zh)
	historyTimeout int    // timeout in seconds
	historyDebug   bool   // enable debug output for troubleshooting
	historyDryRun  bool   // preview without executing rebase
)

var squashHistoryCmd = &cobra.Command{
	Use:   "squash-history",
	Short: "Squash unpushed commits with AI-generated message",
	Long: `Squash unpushed commits into a single commit with an AI-generated message.

Analyzes your unpushed commits and consolidates them through interactive rebase.
Creates a backup branch before modifying history for safety.`,
	Example: `  # Interactive mode with TUI
  catmit squash-history

  # Auto-squash without confirmation
  catmit squash-history --yes

  # Preview what would be squashed (dry run)
  catmit squash-history --dry-run

  # Chinese output with custom timeout
  catmit squash-history --lang zh --timeout 60`,
	RunE: runSquashHistory,
}

func init() {
	rootCmd.AddCommand(squashHistoryCmd)

	squashHistoryCmd.Flags().BoolVar(&historyDebug, "debug", false, "enable debug output for troubleshooting")
	squashHistoryCmd.Flags().BoolVar(&historyDryRun, "dry-run", false, "preview without executing")
	squashHistoryCmd.Flags().StringVarP(&historyLang, "lang", "l", "en", "output language (en/zh)")
	squashHistoryCmd.Flags().IntVarP(&historyTimeout, "timeout", "t", 30, "timeout in seconds")
	squashHistoryCmd.Flags().BoolVarP(&historyYes, "yes", "y", false, "skip confirmation and execute directly")
}

func runSquashHistory(cmd *cobra.Command, args []string) error {
	// Initialize dependencies
	deps, err := initSquashDependencies(historyDebug)
	if err != nil {
		return err
	}
	defer func() { _ = deps.logger.Sync() }()

	// Create context with timeout
	ctx, cancel := createContext(cmd, historyTimeout)
	defer cancel()

	// Create rebase workflow
	workflow, err := createRebaseWorkflow(ctx, deps.llmClient, historyLang, historyDebug, deps.logger)
	if err != nil {
		return fmt.Errorf("failed to create rebase workflow: %w", err)
	}

	// Handle dry-run mode
	if historyDryRun {
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

		fmt.Println("=== DRY RUN MODE ===")
		fmt.Printf("Would squash %d commits from branch '%s'\n", len(analysis.UnpushedCommits), analysis.CurrentBranch)
		fmt.Println("\nGenerated commit message:")
		fmt.Println(message)
		fmt.Println("\n=== END DRY RUN ===")
		return nil
	}

	// Handle yes mode
	if historyYes {
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
	model := ui.NewRebaseWorkflowModel(ctx, workflow)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check if it's the expected model type
	rebaseModel, ok := finalModel.(*ui.RebaseWorkflowModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	// If user accepted and rebase was successful
	if rebaseModel.IsAccepted() {
		fmt.Println("\n✓ Rebase completed successfully")
		if backupBranch := rebaseModel.GetBackupBranch(); backupBranch != "" {
			fmt.Printf("Backup branch: %s\n", backupBranch)
		}
	}

	return nil
}