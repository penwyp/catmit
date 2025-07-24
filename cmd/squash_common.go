package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/rebase"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/penwyp/catmit/pkg/llm"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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

// squashDependencies holds common dependencies for squash commands
type squashDependencies struct {
	logger    *zap.Logger
	deps      *app.Dependencies
	llmClient squash.ClientInterface
}

// initSquashDependencies initializes common dependencies for squash commands
func initSquashDependencies(debug bool) (*squashDependencies, error) {
	// Initialize logger
	appLogger, err := logger.New(debug)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Create dependencies
	deps := app.NewDependencies(appLogger, debug)

	// Create LLM client adapter
	llmClient := &clientAdapter{client: deps.GetClient()}

	return &squashDependencies{
		logger:    appLogger,
		deps:      deps,
		llmClient: llmClient,
	}, nil
}

// createContext creates a context with timeout and proper timeout handling
func createContext(cmd *cobra.Command, timeout int) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(timeout)*time.Second)
	
	// Wrap the cancel function to handle timeout explicitly
	wrappedCancel := func() {
		cancel()
		// Check if the context was cancelled due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(cmd.ErrOrStderr(), "\nOperation timed out after %d seconds\n", timeout)
		}
	}
	
	return ctx, wrappedCancel
}

// createRebaseWorkflow creates a rebase workflow with all necessary components
func createRebaseWorkflow(ctx context.Context, llmClient squash.ClientInterface, lang string, debug bool, logger *zap.Logger) (*rebase.Workflow, error) {
	// Create git runner
	runner := git.NewRunnerWithLogger(debug, logger)

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
		Language:   lang,
		Logger:     logger,
	}

	// Create rebase workflow
	return rebase.New(history, llmClient, config), nil
}