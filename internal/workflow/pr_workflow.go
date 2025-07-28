package workflow

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/internal/ui"
	"go.uber.org/zap"
)

// PRWorkflow handles the PR-only workflow
type PRWorkflow struct {
	deps   *app.Dependencies
	config *app.PROnlyConfig
	logger *zap.Logger
	output io.Writer
}

// NewPRWorkflow creates a new PR workflow instance
func NewPRWorkflow(deps *app.Dependencies, config *app.PROnlyConfig, output io.Writer) *PRWorkflow {
	return &PRWorkflow{
		deps:   deps,
		config: config,
		logger: deps.Logger,
		output: output,
	}
}

// Run executes the PR workflow
func (w *PRWorkflow) Run(ctx context.Context) error {
	// Early check: ensure we're in a git repository
	if err := w.checkGitRepository(ctx); err != nil {
		return err
	}

	// Check if PR already exists
	exists, prURL, err := w.checkPRExists(ctx)
	if err != nil {
		// Log error but continue - PR check is not critical
		if w.config.Debug {
			w.logger.Debug("Failed to check PR existence, continuing", zap.Error(err))
		}
	} else if exists {
		// PR already exists, display URL and exit
		fmt.Fprintln(w.output, RenderStatusBar("Pull request already exists", true))
		if prURL != "" {
			fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
		} else {
			fmt.Fprintln(w.output, "Please check your Git hosting platform for the existing PR")
		}
		return nil
	}

	// Execute appropriate workflow based on flags
	if w.config.DryRun {
		return w.runDryRun(ctx)
	}

	if w.config.Yes {
		return w.runAutomatic(ctx)
	}

	return w.runInteractive(ctx)
}

// checkGitRepository performs a quick check to see if we're in a git repository
func (w *PRWorkflow) checkGitRepository(ctx context.Context) error {
	runner := w.deps.GetGitRunner()
	_, err := runner.Run(ctx, "git", "rev-parse", "--git-dir")
	if err != nil {
		return errors.ErrNoGitRepo
	}
	return nil
}

// checkPRExists checks if a PR already exists for the current branch
func (w *PRWorkflow) checkPRExists(ctx context.Context) (bool, string, error) {
	prCreator := w.deps.GetPRCreator()
	if prCreator == nil {
		if w.config.Debug {
			w.logger.Debug("PR creator not available, cannot check PR existence")
		}
		return false, "", nil
	}

	options := pr.CreateOptions{
		Remote:     w.config.PRConfig.Remote,
		BaseBranch: w.config.PRConfig.BaseBranch,
		Draft:      w.config.PRConfig.Draft,
	}

	exists, prURL, err := prCreator.CheckExists(ctx, options)
	if err != nil {
		if w.config.Debug {
			w.logger.Debug("prCreator.CheckExists returned error", zap.Error(err))
		}
		return false, "", err
	}

	return exists, prURL, nil
}

// runDryRun executes the dry-run workflow
func (w *PRWorkflow) runDryRun(ctx context.Context) error {
	// Get branch and commit information
	col := w.deps.GetCollector()
	branchName, err := col.BranchName(ctx)
	if err != nil {
		return errors.Wrap(errors.ErrTypeGit, "failed to get branch name", err)
	}

	// Generate PR title and body
	title, body, err := w.generatePRContent(ctx)
	if err != nil {
		return err
	}

	// Display PR preview
	fmt.Fprintln(w.output, RenderStatusBar("Pull Request Preview (Dry Run)", true))
	fmt.Fprintf(w.output, "  Branch: %s → %s\n", branchName, w.config.PRConfig.BaseBranch)
	fmt.Fprintf(w.output, "  Title: %s\n", title)
	if body != "" {
		fmt.Fprintln(w.output, "  Body:")
		bodyLines := strings.Split(body, "\n")
		for _, line := range bodyLines {
			fmt.Fprintf(w.output, "    %s\n", line)
		}
	}
	if w.config.PRConfig.Draft {
		fmt.Fprintln(w.output, "  Draft: Yes")
	}

	return nil
}

// runAutomatic executes the automatic PR workflow
func (w *PRWorkflow) runAutomatic(ctx context.Context) error {
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
			return errors.Wrap(errors.ErrTypeGit, "failed to push branch", err)
		}
		fmt.Fprintln(w.output, RenderStatusBar("Branch pushed successfully", true))
	}

	// Show PR preview
	title, body, err := w.generatePRContent(ctx)
	if err != nil {
		return err
	}

	if err := w.showPRPreview(ctx, title, body); err != nil {
		return err
	}

	// Create pull request
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

// runInteractive executes the interactive TUI workflow for PR creation
func (w *PRWorkflow) runInteractive(ctx context.Context) error {
	col := w.deps.GetCollector()
	promptBuilder := w.deps.GetPromptBuilder(w.config.Language)
	client := w.deps.GetClient()
	committer := w.deps.GetCommitterWithPRConfig(w.config.PRConfig)

	prConfig := ui.PRConfig{
		CreatePR:    true, // Always true for PR workflow
		Remote:      w.config.PRConfig.Remote,
		Base:        w.config.PRConfig.BaseBranch,
		Draft:       w.config.PRConfig.Draft,
		Provider:    w.config.PRConfig.Provider,
		UseTemplate: w.config.PRConfig.UseTemplate,
	}

	// Create a PR workflow model
	prModel := ui.NewPRWorkflowModel(
		ctx,
		col,
		promptBuilder,
		client,
		committer,
		w.config.Language,
		time.Duration(w.config.Timeout)*time.Second,
		prConfig,
	)

	finalModel, err := tea.NewProgram(prModel).Run()
	if err != nil {
		return errors.Wrap(errors.ErrTypeUnknown, "failed to run TUI", err)
	}

	// Check if the model is the expected type
	if _, ok := finalModel.(*ui.PRWorkflowModel); !ok {
		return errors.Newf(errors.ErrTypeUnknown, "internal error: unexpected model type, got %T", finalModel)
	}

	return nil
}

// generatePRContent generates PR title and body from commits
func (w *PRWorkflow) generatePRContent(ctx context.Context) (string, string, error) {
	col := w.deps.GetCollector()

	// Get recent commits to generate PR content
	commits, err := col.RecentCommits(ctx, 20)
	if err != nil {
		return "", "", errors.Wrap(errors.ErrTypeGit, "failed to get recent commits", err)
	}

	if len(commits) == 0 {
		return "", "", errors.New(errors.ErrTypeGit, "no commits found on current branch")
	}

	// Build prompt for PR generation
	builder := w.deps.GetPromptBuilder(w.config.Language)
	systemPrompt := builder.BuildPRSystemPrompt()
	userPrompt := builder.BuildPRUserPrompt(commits)

	client := w.deps.GetClient()
	apiCtx, apiCancel := context.WithTimeout(ctx, time.Duration(w.config.Timeout)*time.Second)
	defer apiCancel()

	// Get PR content from LLM
	prContent, err := client.GetCommitMessage(apiCtx, systemPrompt, userPrompt)
	if err != nil {
		if errors.Is(err, errors.ErrLLMTimeout) {
			return "", "", errors.Wrap(errors.ErrTypeTimeout, "failed to generate PR content", err)
		}
		return "", "", errors.Wrap(errors.ErrTypeLLM, "failed to generate PR content", err)
	}

	// Parse the response to extract title and body
	lines := strings.Split(prContent, "\n")
	title := strings.TrimSpace(lines[0])
	body := ""
	if len(lines) > 1 {
		body = strings.Join(lines[1:], "\n")
		body = strings.TrimSpace(body)
	}

	return title, body, nil
}

// showPRPreview displays PR details before creation
func (w *PRWorkflow) showPRPreview(ctx context.Context, title, body string) error {
	col := w.deps.GetCollector()
	branchName, err := col.BranchName(ctx)
	if err != nil {
		branchName = "current branch"
	}

	fmt.Fprintln(w.output, "")
	fmt.Fprintln(w.output, RenderStatusBar("Pull Request Preview", true))
	fmt.Fprintf(w.output, "  Branch: %s → %s\n", branchName, w.config.PRConfig.BaseBranch)
	fmt.Fprintf(w.output, "  Title: %s\n", title)
	if body != "" {
		fmt.Fprintln(w.output, "  Body:")
		bodyLines := strings.Split(body, "\n")
		for _, line := range bodyLines {
			fmt.Fprintf(w.output, "    %s\n", line)
		}
	}
	if w.config.PRConfig.Draft {
		fmt.Fprintln(w.output, "  Draft: Yes")
	}
	fmt.Fprintln(w.output, "")

	return nil
}