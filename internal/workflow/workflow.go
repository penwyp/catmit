// Package workflow implements the main application workflow logic for catmit.
// It orchestrates the interaction between different components like collectors,
// prompt builders, LLM clients, and git operations.
package workflow

import (
	"context"
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/penwyp/catmit/pkg/output"
	"go.uber.org/zap"
)

// Workflow handles the main application workflow
type Workflow struct {
	deps   *app.Dependencies
	config *app.Config
	logger *zap.Logger
	output io.Writer
}

// New creates a new workflow instance
func New(deps *app.Dependencies, config *app.Config, output io.Writer) *Workflow {
	return &Workflow{
		deps:   deps,
		config: config,
		logger: deps.Logger,
		output: output,
	}
}

// Run executes the main workflow based on configuration
func (w *Workflow) Run(ctx context.Context) error {
	// Early check: ensure we're in a git repository
	if err := w.checkGitRepository(ctx); err != nil {
		return err
	}

	// Execute appropriate workflow based on flags
	if w.config.DryRun {
		return w.runDryRun(ctx)
	}

	if w.config.AutoConfirm {
		return w.runAutomatic(ctx)
	}

	return w.runInteractive(ctx)
}

// checkGitRepository performs a quick check to see if we're in a git repository
func (w *Workflow) checkGitRepository(ctx context.Context) error {
	runner := w.deps.GetGitRunner()
	_, err := runner.Run(ctx, "git", "rev-parse", "--git-dir")
	if err != nil {
		return errors.ErrNoGitRepo
	}
	return nil
}

// runDryRun executes the dry-run workflow
func (w *Workflow) runDryRun(ctx context.Context) error {
	// For dry-run, we want clean output without control characters
	message, err := w.generateCommitMessage(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(w.output, message)
	return nil
}

// runAutomatic executes the automatic commit workflow (-y flag)
func (w *Workflow) runAutomatic(ctx context.Context) error {
	// Show non-streaming progress for automatic mode
	fmt.Fprintln(w.output, output.RenderStatusBar("Generating commit message...", false))

	message, err := w.generateCommitMessage(ctx)
	if err != nil {
		return err
	}

	// Display the generated message
	fmt.Fprintln(w.output, message)

	// Commit the changes
	fmt.Fprintln(w.output, output.RenderStatusBar("Committing...", false))

	committer := w.deps.GetCommitter()

	// Stage all changes if flagStageAll is true
	if w.config.StageAll {
		if err := committer.StageAll(ctx); err != nil {
			return errors.Wrap(errors.ErrTypeGit, "failed to stage all files", err)
		}
	}

	if err := committer.Commit(ctx, message); err != nil {
		return errors.Wrap(errors.ErrTypeGit, "failed to commit", err)
	}
	fmt.Fprintln(w.output, output.RenderStatusBar("Committed successfully", true))

	// Push if enabled
	if w.config.Push {
		fmt.Fprintln(w.output, output.RenderStatusBar("Pushing...", false))
		if err := committer.Push(ctx); err != nil {
			return errors.Wrap(errors.ErrTypeGit, "push failed", err)
		}
		fmt.Fprintln(w.output, output.RenderStatusBar("Pushed successfully", true))
	}

	return nil
}

// runInteractive executes the interactive TUI workflow
func (w *Workflow) runInteractive(ctx context.Context) error {
	col := w.deps.GetCollector()
	promptBuilder := w.deps.GetPromptBuilder(w.config.Language)
	client := w.deps.GetClient()
	committer := w.deps.GetCommitter()

	// Create commit workflow model for regular workflow
	prConfig := ui.PRConfig{
		CreatePR: false, // No PR creation in main workflow
		Remote:   "origin",
		Base:     "",
		Draft:    false,
		Provider: "",
	}

	mainModel := ui.NewCommitWorkflowModel(
		ctx,
		col,
		promptBuilder,
		client,
		committer,
		w.config.SeedText,
		w.config.Language,
		time.Duration(w.config.Timeout)*time.Second,
		w.config.Push,
		w.config.StageAll,
		prConfig,
	)

	finalModel, err := tea.NewProgram(mainModel).Run()
	if err != nil {
		return errors.Wrap(errors.ErrTypeUnknown, "failed to run TUI", err)
	}

	m, ok := finalModel.(*ui.CommitWorkflowModel)
	if !ok {
		return errors.Newf(errors.ErrTypeUnknown, "internal error: unexpected model type, got %T", finalModel)
	}

	done, decision, _, err := m.IsDone()
	if err != nil {
		// Check if it's the "nothing to commit" error
		if errors.Is(err, gitinfo.ErrNoDiff) {
			return err
		}
		// Check if it's a git repository error
		if errors.Is(err, errors.ErrNoGitRepo) {
			return err
		}
		// If user canceled during loading (Ctrl+C), exit silently
		if err == context.Canceled {
			return nil
		}
		return errors.Wrap(errors.ErrTypeUnknown, "TUI execution failed", err)
	}

	if done {
		switch decision {
		case ui.DecisionAccept:
			// MainModel has already handled the staging, commit, and push operations
			return nil
		case ui.DecisionCancel:
			fmt.Fprintln(w.output, "Canceled.")
		}
	}

	return nil
}

// generateCommitMessage generates a commit message using the LLM
func (w *Workflow) generateCommitMessage(ctx context.Context) (string, error) {
	return GenerateCommitMessage(ctx, w.deps, CommitMessageOptions{
		Language: w.config.Language,
		Timeout:  w.config.Timeout,
		SeedText: w.config.SeedText,
		Debug:    w.config.Debug,
	})
}
