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
	"github.com/penwyp/catmit/pkg/output"
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

	// OPTIMIZATION: Early PR existence check to avoid unnecessary work including LLM calls
	// This is the key optimization - check before any expensive operations
	w.logger.Debug("Performing early PR existence check to optimize workflow")
	exists, prURL, err := w.checkPRExists(ctx)
	if err != nil {
		// Enhanced error handling with detailed logging
		w.logger.Warn("PR existence check failed, will proceed with full workflow",
			zap.Error(err),
			zap.String("error_type", fmt.Sprintf("%T", err)))
		
		// Provide user-friendly feedback based on error type
		if errors.Is(err, errors.ErrCLINotInstalled) || errors.Is(err, errors.ErrCLINotAuthed) {
			fmt.Fprintf(w.output, "%s\n", output.RenderStatusBarWithType(
				fmt.Sprintf("Note: PR existence check skipped (%s). Install and authenticate CLI tools for better optimization.", w.getProviderCLIName()),
				output.StatusInfo))
		} else if w.config.Debug {
			fmt.Fprintf(w.output, "%s\n", output.RenderStatusBarWithType(
				fmt.Sprintf("PR existence check failed: %v (continuing with full workflow)", err),
				output.StatusWarning))
		} else {
			// In non-debug mode, just show a brief note
			fmt.Fprintln(w.output, output.RenderStatusBarWithType(
				"Note: Unable to check for existing PRs, proceeding with creation workflow.",
				output.StatusInfo))
		}
	} else if exists {
		// PR already exists, display URL and exit - LLM call avoided!
		w.logger.Debug("Found existing PR, skipping LLM generation and entire creation workflow",
			zap.String("pr_url", prURL))
		fmt.Fprintln(w.output, output.RenderStatusBar("Pull request already exists", true))
		
		if prURL != "" {
			fmt.Fprintf(w.output, "PR URL: %s\n", prURL)
		} else {
			fmt.Fprintln(w.output, "Please check your Git hosting platform for the existing PR")
		}
		
		// Show optimization benefit in debug mode
		if w.config.Debug {
			fmt.Fprintln(w.output, output.RenderStatusBarWithType(
				"Optimization: Skipped LLM call and PR creation workflow",
				output.StatusInfo))
		}
		return nil
	} else {
		w.logger.Debug("No existing PR found, proceeding with PR creation workflow")
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
		w.logger.Warn("PR creator not available - missing CLI tools or dependencies")
		return false, "", errors.New(errors.ErrTypePR, "PR creator not available")
	}

	options := pr.CreateOptions{
		Remote:     w.config.PRConfig.Remote,
		BaseBranch: w.config.PRConfig.BaseBranch,
		Draft:      w.config.PRConfig.Draft,
	}

	w.logger.Debug("Checking PR existence with options",
		zap.String("remote", options.Remote),
		zap.String("base_branch", options.BaseBranch),
		zap.Bool("draft", options.Draft))

	exists, prURL, err := prCreator.CheckExists(ctx, options)
	if err != nil {
		w.logger.Error("PR existence check failed",
			zap.Error(err),
			zap.String("remote", options.Remote),
			zap.String("base_branch", options.BaseBranch))
		return false, "", err
	}

	w.logger.Debug("PR existence check completed",
		zap.Bool("exists", exists),
		zap.String("url", prURL))

	return exists, prURL, nil
}

// getProviderCLIName returns the CLI tool name for the current provider
func (w *PRWorkflow) getProviderCLIName() string {
	// Try to detect provider from the PR creator
	prCreator := w.deps.GetPRCreator()
	if prCreator == nil {
		return "CLI tools (gh/glab/tea)"
	}

	// For now, return a generic message
	// In a real implementation, we might want to detect the actual provider
	return "CLI tools (gh/glab/tea)"
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
	fmt.Fprintln(w.output, output.RenderStatusBar("Pull Request Preview (Dry Run)", true))
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
		fmt.Fprintln(w.output, output.RenderStatusBar("Pushing branch...", false))
		if err := committer.Push(ctx); err != nil {
			return errors.Wrap(errors.ErrTypeGit, "failed to push branch", err)
		}
		fmt.Fprintln(w.output, output.RenderStatusBar("Branch pushed successfully", true))
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
	fmt.Fprintln(w.output, output.RenderStatusBar("Creating pull request...", false))
	prURL, err := committer.CreatePullRequest(ctx)
	if err != nil {
		var prExists *pr.ErrPRAlreadyExists
		if errors.As(err, &prExists) {
			fmt.Fprintln(w.output, output.RenderStatusBar("Pull request already exists", true))
			if prExists.URL != "" {
				fmt.Fprintf(w.output, "PR URL: %s\n", prExists.URL)
			} else {
				fmt.Fprintln(w.output, "Please check your Git hosting platform for the existing PR")
			}
			return nil
		}
		return errors.Wrap(errors.ErrTypePR, "failed to create pull request", err)
	}
	fmt.Fprintln(w.output, output.RenderStatusBar("Pull request created successfully", true))
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
		w.deps.GetExtendedGitRunner(),
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
	// Use CommitAnalyzer to get PR-specific commits (only new commits not in base branch)
	analyzer := pr.NewCommitAnalyzer(w.deps.GetExtendedGitRunner())
	
	// Analyze commits relative to the PR base branch
	analysisData, err := analyzer.AnalyzeForPR(ctx, w.config.PRConfig.Remote, w.config.PRConfig.BaseBranch)
	if err != nil {
		// Even if analysis fails, we still have branch name to work with
		w.logger.Debug("Failed to analyze commits for PR, will use branch name", zap.Error(err))
	}

	// Build prompt for PR generation
	builder := w.deps.GetPromptBuilder(w.config.Language)
	systemPrompt := builder.BuildPRSystemPrompt()
	
	// Build user prompt based on analysis data
	var userPrompt string
	if analysisData != nil && len(analysisData.Commits) > 0 {
		// We have actual new commits for this PR
		commitMessages := make([]string, len(analysisData.Commits))
		for i, commit := range analysisData.Commits {
			commitMessages[i] = commit.Message
		}
		userPrompt = builder.BuildPRUserPrompt(commitMessages)
	} else if analysisData != nil && analysisData.BranchName != "" {
		// No new commits, use branch name to generate PR content
		// This handles the case of empty branches created for future work
		userPrompt = fmt.Sprintf("Based on the branch name '%s', generate a PR title and description. This branch has no new commits yet, so infer the purpose from the branch name.", analysisData.BranchName)
	} else {
		// Fallback: try to get branch name directly
		col := w.deps.GetCollector()
		branchName, err := col.BranchName(ctx)
		if err != nil {
			return "", "", errors.Wrap(errors.ErrTypeGit, "failed to get branch name", err)
		}
		userPrompt = fmt.Sprintf("Based on the branch name '%s', generate a PR title and description. This branch has no new commits yet, so infer the purpose from the branch name.", branchName)
	}

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
	fmt.Fprintln(w.output, output.RenderStatusBar("Pull Request Preview", true))
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