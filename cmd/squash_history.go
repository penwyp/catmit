package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/penwyp/catmit/pkg/output"
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

	// Create base context without timeout for workflow creation
	baseCtx := cmd.Context()

	// Create rebase workflow with base context
	workflow, err := createRebaseWorkflow(baseCtx, deps.llmClient, historyLang, historyDebug, deps.logger)
	if err != nil {
		return fmt.Errorf("failed to create rebase workflow: %w", err)
	}

	// Handle dry-run mode
	if historyDryRun {
		// Create context with timeout for analysis
		analysisCtx, analysisCancel := createContext(cmd, historyTimeout)
		defer analysisCancel()

		// Analyze the repository
		analysis, err := workflow.Analyze(analysisCtx)
		if err != nil {
			return fmt.Errorf("failed to analyze repository: %w", err)
		}

		if !analysis.CanRebase {
			fmt.Println(analysis.Message)
			return nil
		}

		// Create new context with timeout for message generation
		genCtx, genCancel := createContext(cmd, historyTimeout)
		defer genCancel()

		// Generate commit message
		message, err := workflow.GenerateCommitMessage(genCtx, analysis.UnpushedCommits)
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
		// Create context with timeout for analysis
		analysisCtx, analysisCancel := createContext(cmd, historyTimeout)
		defer analysisCancel()

		// Analyze the repository
		analysis, err := workflow.Analyze(analysisCtx)
		if err != nil {
			return fmt.Errorf("failed to analyze repository: %w", err)
		}

		if !analysis.CanRebase {
			fmt.Println(analysis.Message)
			return nil
		}

		// Create new context with timeout for message generation
		genCtx, genCancel := createContext(cmd, historyTimeout)
		defer genCancel()

		// Generate commit message
		message, err := workflow.GenerateCommitMessage(genCtx, analysis.UnpushedCommits)
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		fmt.Println("Generated commit message:")
		fmt.Println(message)

		// Create new context with timeout for rebase execution
		rebaseCtx, rebaseCancel := createContext(cmd, historyTimeout)
		defer rebaseCancel()

		// Execute rebase
		fmt.Println("\nExecuting rebase...")
		if err := workflow.ExecuteRebase(rebaseCtx, analysis, message); err != nil {
			return fmt.Errorf("rebase failed: %w", err)
		}

		fmt.Println(output.RenderStatusBar("Rebase completed successfully", true))
		return nil
	}

	// TUI mode - pass historyTimeout so the model can create its own contexts
	model := ui.NewRebaseWorkflowModel(baseCtx, workflow, historyTimeout)
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
		fmt.Println("\n" + output.RenderStatusBar("Rebase completed successfully", true))
		if backupBranch := rebaseModel.GetBackupBranch(); backupBranch != "" {
			fmt.Printf("Backup branch: %s\n", backupBranch)
		}
	}

	return nil
}