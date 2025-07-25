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
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/penwyp/catmit/pkg/gitinfo"
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

	// Early PR existence check if --pr is requested
	if w.config.CreatePR && !w.config.DryRun {
		if w.config.Debug {
			w.logger.Debug("Starting early PR existence check",
				zap.String("remote", w.config.PRConfig.Remote),
				zap.String("baseBranch", w.config.PRConfig.BaseBranch))
		}
		if exists, prURL, err := w.checkPRExists(ctx); err != nil {
			// Log error but continue - PR check is not critical
			if w.config.Debug {
				w.logger.Debug("Failed to check PR existence", zap.Error(err))
			}
		} else {
			if w.config.Debug {
				w.logger.Debug("Early PR check result",
					zap.Bool("exists", exists),
					zap.String("prURL", prURL))
			}
			if exists {
				// PR already exists, display URL and exit
				fmt.Fprintln(w.output, RenderStatusBar("Pull request already exists", true))
				if prURL != "" {
					fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
				}
				return nil
			}
		}
	}

	// Special case: if --pr is requested without -y and no changes, just create PR
	if w.config.CreatePR && !w.config.AutoConfirm && !w.config.DryRun {
		if handled, err := w.handlePROnlyCase(ctx); handled {
			return err
		}
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

// handlePROnlyCase handles the special case where user wants to create PR without changes
func (w *Workflow) handlePROnlyCase(ctx context.Context) (bool, error) {
	col := w.deps.GetCollector()
	_, err := col.ComprehensiveDiff(ctx)
	if err != nil && errors.Is(err, gitinfo.ErrNoDiff) {
		// No changes, but user wants to create PR
		committer := w.deps.GetCommitterWithPRConfig(w.config.PRConfig)

		// Check if we need to push first
		needsPush, err := committer.NeedsPush(ctx)
		if err != nil {
			if w.config.Debug {
				w.logger.Debug("Failed to check if push is needed", zap.Error(err))
			}
			needsPush = false
		}

		if needsPush {
			fmt.Fprintln(w.output, RenderStatusBar("Pushing branch...", false))
			if err := committer.Push(ctx); err != nil {
				return true, errors.Wrap(errors.ErrTypeGit, "failed to push branch", err)
			}
			fmt.Fprintln(w.output, RenderStatusBar("Branch pushed successfully", true))
		}

		// Check if PR already exists before creating
		if exists, prURL, err := w.checkPRExists(ctx); err != nil {
			if w.config.Debug {
				w.logger.Debug("Failed to check PR existence in handlePROnlyCase", zap.Error(err))
			}
			// Continue with PR creation if check fails
		} else if exists {
			// PR already exists, display URL and exit
			fmt.Fprintln(w.output, RenderStatusBar("Pull request already exists", true))
			if prURL != "" {
				fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
			}
			return true, nil
		}

		// Create PR even with no changes
		fmt.Fprintln(w.output, RenderStatusBar("Creating pull request...", false))
		prURL, err := committer.CreatePullRequest(ctx)
		if err != nil {
			var prExists *pr.ErrPRAlreadyExists
			if errors.As(err, &prExists) {
				fmt.Fprintln(w.output, RenderStatusBar("Pull request already exists", true))
				if prExists.URL != "" {
					fmt.Fprintf(w.output, "PR URL: %s\n", prExists.URL)
				} else {
					fmt.Fprintln(w.output, "Please check your Git hosting platform for the existing PR")
				}
				return true, nil
			}
			return true, errors.Wrap(errors.ErrTypePR, "failed to create pull request", err)
		}
		fmt.Fprintln(w.output, RenderStatusBar("Pull request created successfully", true))
		if prURL != "" {
			fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
		}
		return true, nil
	}

	// If there's an error other than NoDiff or there are changes, continue normal flow
	return false, nil
}

// runDryRun executes the dry-run workflow
func (w *Workflow) runDryRun(ctx context.Context) error {
	message, err := w.generateCommitMessage(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(w.output, message)
	return nil
}

// runAutomatic executes the automatic commit workflow (-y flag)
func (w *Workflow) runAutomatic(ctx context.Context) error {
	message, err := w.generateCommitMessage(ctx)
	if err != nil {
		// Special case: if --pr is requested and there are no changes
		if errors.Is(err, gitinfo.ErrNoDiff) && w.config.CreatePR {
			// Try to create PR without changes
			committer := w.deps.GetCommitterWithPRConfig(w.config.PRConfig)

			// Check if we need to push first
			needsPush, pushErr := committer.NeedsPush(ctx)
			if pushErr != nil {
				if w.config.Debug {
					w.logger.Debug("Failed to check if push is needed", zap.Error(pushErr))
				}
				needsPush = false
			}

			if needsPush {
				fmt.Fprintln(w.output, RenderStatusBar("Pushing branch...", false))
				if pushErr := committer.Push(ctx); pushErr != nil {
					return errors.Wrap(errors.ErrTypeGit, "failed to push branch", pushErr)
				}
				fmt.Fprintln(w.output, RenderStatusBar("Branch pushed successfully", true))
			}

			// Create PR even with no changes
			fmt.Fprintln(w.output, RenderStatusBar("Creating pull request...", false))
			prURL, prErr := committer.CreatePullRequest(ctx)
			if prErr != nil {
				var prExists *pr.ErrPRAlreadyExists
				if errors.As(prErr, &prExists) {
					fmt.Fprintln(w.output, RenderStatusBar("Pull request already exists", true))
					if prExists.URL != "" {
						fmt.Fprintf(w.output, "PR URL: %s\n", prExists.URL)
					} else {
						fmt.Fprintln(w.output, "Please check your Git hosting platform for the existing PR")
					}
					return nil
				}
				return errors.Wrap(errors.ErrTypePR, "failed to create pull request", prErr)
			}
			fmt.Fprintln(w.output, RenderStatusBar("Pull request created successfully", true))
			if prURL != "" {
				fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
			}
			return nil
		}
		return err
	}

	// Commit the changes
	fmt.Fprintln(w.output, RenderStatusBar("Committing...", false))

	var committer git.Committer
	if w.config.CreatePR {
		committer = w.deps.GetCommitterWithPRConfig(w.config.PRConfig)
	} else {
		committer = w.deps.GetCommitter()
	}

	// Only stage all if there are no staged changes and flagStageAll is true
	if w.config.StageAll && !committer.HasStagedChanges(ctx) {
		if err := committer.StageAll(ctx); err != nil {
			return errors.Wrap(errors.ErrTypeGit, "failed to stage all files", err)
		}
	}

	if err := committer.Commit(ctx, message); err != nil {
		return errors.Wrap(errors.ErrTypeGit, "failed to commit", err)
	}
	fmt.Fprintln(w.output, RenderStatusBar("Committed successfully", true))

	// Push if enabled
	if w.config.Push {
		fmt.Fprintln(w.output, RenderStatusBar("Pushing...", false))
		if err := committer.Push(ctx); err != nil {
			return errors.Wrap(errors.ErrTypeGit, "push failed", err)
		}
		fmt.Fprintln(w.output, RenderStatusBar("Pushed successfully", true))
	}

	// Create pull request if requested
	if w.config.CreatePR {
		if err := w.createPullRequest(ctx, committer); err != nil {
			return err
		}
	}

	return nil
}

// runInteractive executes the interactive TUI workflow
func (w *Workflow) runInteractive(ctx context.Context) error {
	col := w.deps.GetCollector()
	promptBuilder := w.deps.GetPromptBuilder(w.config.Language)
	client := w.deps.GetClient()
	var committer git.Committer
	if w.config.CreatePR {
		committer = w.deps.GetCommitterWithPRConfig(w.config.PRConfig)
	} else {
		committer = w.deps.GetCommitter()
	}

	prConfig := ui.PRConfig{
		CreatePR:    w.config.CreatePR,
		Remote:      w.config.PRConfig.Remote,
		Base:        w.config.PRConfig.BaseBranch,
		Draft:       w.config.PRConfig.Draft,
		Provider:    w.config.PRConfig.Provider,
		UseTemplate: w.config.PRConfig.UseTemplate,
	}

	mainModel := ui.NewMainModelWithPRConfig(
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

	m, ok := finalModel.(*ui.MainModel)
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
	col := w.deps.GetCollector()

	// Use ComprehensiveDiff to include untracked files
	diffText, err := col.ComprehensiveDiff(ctx)
	if err != nil {
		if errors.Is(err, gitinfo.ErrNoDiff) {
			if w.config.CreatePR {
				// Special handling for PR creation without changes
				// Return early to handle in the calling function
				return "", err
			}
			if w.config.Debug {
				w.logger.Debug("No staged, unstaged, or untracked changes detected")
			}
			return "", err
		}
		return "", errors.Wrap(errors.ErrTypeGit, "failed to collect git diff", err)
	}

	commits, err := col.RecentCommits(ctx, 10)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to process diff", err)
	}

	builder := w.deps.GetPromptBuilder(w.config.Language)
	systemPrompt := builder.BuildSystemPrompt()

	// Try to use the new BuildUserPromptWithBudget method
	userPrompt, err := builder.BuildUserPromptWithBudget(ctx, col, w.config.SeedText)
	if err != nil {
		if w.config.Debug {
			w.logger.Debug("Smart prompt building failed, falling back to traditional method", zap.Error(err))
		}
		// Fallback to traditional method
		branch, _ := col.BranchName(ctx)
		files, _ := col.ChangedFiles(ctx)
		userPrompt = builder.BuildUserPrompt(w.config.SeedText, diffText, commits, branch, files)
	}

	client := w.deps.GetClient()
	// Create timeout context only for API call
	apiCtx, apiCancel := context.WithTimeout(ctx, time.Duration(w.config.Timeout)*time.Second)
	defer apiCancel()

	message, err := client.GetCommitMessage(apiCtx, systemPrompt, userPrompt)
	if err != nil {
		// Preserve timeout error type
		if errors.Is(err, errors.ErrLLMTimeout) {
			return "", errors.Wrap(errors.ErrTypeTimeout, "failed to get commit message from LLM", err)
		}
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to get commit message from LLM", err)
	}

	return message, nil
}

// createPullRequest handles PR creation logic
func (w *Workflow) createPullRequest(ctx context.Context, committer git.Committer) error {
	// Check if we need to push first
	if !w.config.Push {
		needsPush, err := committer.NeedsPush(ctx)
		if err != nil {
			if w.config.Debug {
				w.logger.Debug("Failed to check if push is needed", zap.Error(err))
			}
			needsPush = false
		}

		if needsPush {
			fmt.Fprintln(w.output, RenderStatusBar("Pushing branch for PR...", false))
			if err := committer.Push(ctx); err != nil {
				return errors.Wrap(errors.ErrTypeGit, "failed to push branch", err)
			}
			fmt.Fprintln(w.output, RenderStatusBar("Branch pushed successfully", true))
		}
	}

	fmt.Fprintln(w.output, RenderStatusBar("Creating pull request...", false))
	prURL, err := committer.CreatePullRequest(ctx)
	if err != nil {
		var prExists *pr.ErrPRAlreadyExists
		if errors.As(err, &prExists) {
			fmt.Fprintln(w.output, RenderStatusBar("Pull request already exists", true))
			if prExists.URL != "" {
				fmt.Fprintf(w.output, "PR URL: %s\n", prExists.URL)
			} else {
				fmt.Fprintln(w.output, "Please check your Git hosting platform for the existing PR")
			}
			return nil
		}
		return errors.Wrap(errors.ErrTypePR, "failed to create pull request", err)
	}
	fmt.Fprintln(w.output, RenderStatusBar("Pull request created successfully", true))
	if prURL != "" {
		fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
	}
	return nil
}

// checkPRExists checks if a PR already exists for the current branch
func (w *Workflow) checkPRExists(ctx context.Context) (bool, string, error) {
	// Get the PR creator from dependencies
	prCreator := w.deps.GetPRCreator()
	if prCreator == nil {
		// PR creator not available, cannot check
		if w.config.Debug {
			w.logger.Debug("PR creator not available, cannot check PR existence")
		}
		return false, "", nil
	}

	// Create PR options for checking
	remote := w.config.PRConfig.Remote
	if remote == "" {
		remote = "origin"
	}
	
	options := pr.CreateOptions{
		Remote:     remote,
		BaseBranch: w.config.PRConfig.BaseBranch,
		Draft:      w.config.PRConfig.Draft,
	}

	if w.config.Debug {
		w.logger.Debug("Calling prCreator.CheckExists",
			zap.String("remote", options.Remote),
			zap.String("baseBranch", options.BaseBranch),
			zap.Bool("draft", options.Draft))
	}

	// Check if PR exists
	exists, prURL, err := prCreator.CheckExists(ctx, options)
	if err != nil {
		if w.config.Debug {
			w.logger.Debug("prCreator.CheckExists returned error", zap.Error(err))
		}
		return false, "", err
	}

	if w.config.Debug {
		w.logger.Debug("prCreator.CheckExists result",
			zap.Bool("exists", exists),
			zap.String("prURL", prURL))
	}

	return exists, prURL, nil
}
