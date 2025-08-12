package rebase

import (
	"context"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/squash"
	"github.com/penwyp/catmit/pkg/githistory"
	"go.uber.org/zap"
)

// Config contains configuration for the rebase workflow
type Config struct {
	BaseBranch string // Base branch to compare against (e.g., "main", "master")
	Language   string // Language for commit message generation
	Logger     *zap.Logger
}

// Workflow manages the rebase squash workflow
type Workflow struct {
	history  githistory.HistoryManager
	squash   *squash.Squash
	config   Config
	logger   *zap.Logger
}

// New creates a new rebase workflow instance
func New(history githistory.HistoryManager, squashClient squash.ClientInterface, config Config) *Workflow {
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Workflow{
		history: history,
		squash:  squash.New(squashClient, config.Language),
		config:  config,
		logger:  logger,
	}
}

// AnalysisResult contains the analysis of what can be rebased
type AnalysisResult struct {
	CurrentBranch    string
	BaseBranch       string
	MergeBase        string
	UnpushedCommits  []githistory.Commit
	HasChanges       bool
	CanRebase        bool
	Message          string // User-friendly message about the analysis
}

// Analyze checks the current state and returns what can be rebased
func (w *Workflow) Analyze(ctx context.Context) (*AnalysisResult, error) {
	result := &AnalysisResult{
		BaseBranch: w.config.BaseBranch,
	}

	// Get current branch
	currentBranch, err := w.history.GetCurrentBranch(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	result.CurrentBranch = currentBranch

	// Check for uncommitted changes
	hasChanges, err := w.history.HasUncommittedChanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check uncommitted changes: %w", err)
	}
	result.HasChanges = hasChanges

	if hasChanges {
		result.CanRebase = false
		result.Message = "Cannot rebase: You have uncommitted changes. Please commit or stash them first."
		return result, nil
	}

	// Find merge base with the base branch
	mergeBase, err := w.history.FindMergeBase(ctx, w.config.BaseBranch, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base with %s: %w", w.config.BaseBranch, err)
	}
	result.MergeBase = mergeBase

	// Get unpushed commits
	unpushedCommits, err := w.history.GetUnpushedCommits(ctx, w.config.BaseBranch, "HEAD")
	if err != nil {
		// Try alternative: commits between merge base and HEAD
		w.logger.Debug("Failed to get unpushed commits, trying alternative method", zap.Error(err))
		unpushedCommits, err = w.history.GetCommitsBetween(ctx, mergeBase, "HEAD")
		if err != nil {
			return nil, fmt.Errorf("failed to get commits for rebase: %w", err)
		}
	}
	result.UnpushedCommits = unpushedCommits

	// Determine if we can rebase
	if len(unpushedCommits) == 0 {
		result.CanRebase = false
		result.Message = fmt.Sprintf("No commits to rebase. Your branch is up to date with %s.", w.config.BaseBranch)
	} else if len(unpushedCommits) == 1 {
		result.CanRebase = false
		result.Message = "Only one commit found. Nothing to squash."
	} else {
		result.CanRebase = true
		result.Message = fmt.Sprintf("Found %d commits that can be squashed.", len(unpushedCommits))
	}

	return result, nil
}

// GenerateCommitMessage generates a new commit message for the squashed commits
func (w *Workflow) GenerateCommitMessage(ctx context.Context, commits []githistory.Commit) (string, error) {
	// Extract commit messages from the commits
	var messages []string
	for _, commit := range commits {
		// Include both subject and body if available
		msg := commit.Subject
		if commit.Body != "" {
			msg += "\n" + commit.Body
		}
		messages = append(messages, msg)
	}

	// Use the squash module to generate a consolidated message
	consolidatedMsg, err := w.squash.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	return consolidatedMsg, nil
}

// ExecuteRebase performs the actual rebase operation
func (w *Workflow) ExecuteRebase(ctx context.Context, analysis *AnalysisResult, newMessage string) error {
	if !analysis.CanRebase {
		return fmt.Errorf("cannot rebase: %s", analysis.Message)
	}

	// Create backup branch
	backupBranch, err := w.history.BackupBranch(ctx, analysis.CurrentBranch)
	if err != nil {
		return fmt.Errorf("failed to create backup branch: %w", err)
	}
	w.logger.Info("Created backup branch", zap.String("branch", backupBranch))

	// Perform the rebase
	err = w.history.RebaseInteractive(ctx, analysis.MergeBase, analysis.UnpushedCommits, newMessage)
	if err != nil {
		// Log the error and provide recovery instructions
		w.logger.Error("Rebase failed", zap.Error(err))
		return fmt.Errorf("rebase failed: %w\n\nTo recover, you can:\n1. Run: git rebase --abort\n2. Or reset to backup: git reset --hard %s", err, backupBranch)
	}

	w.logger.Info("Rebase completed successfully", 
		zap.String("backup", backupBranch),
		zap.Int("commits_squashed", len(analysis.UnpushedCommits)))

	return nil
}

// FormatCommitList formats commits for display
func FormatCommitList(commits []githistory.Commit) string {
	var lines []string
	for i, commit := range commits {
		lines = append(lines, fmt.Sprintf("  %d. %s %s", i+1, commit.ShortSHA, commit.Subject))
	}
	return strings.Join(lines, "\n")
}

// GetRecoveryInstructions returns instructions for recovering from a failed rebase
func GetRecoveryInstructions(backupBranch string) string {
	return fmt.Sprintf(`If something went wrong, you can recover using one of these commands:

1. Abort the rebase (if still in progress):
   git rebase --abort

2. Reset to the backup branch:
   git reset --hard %s

3. Delete the backup branch when done:
   git branch -D %s`, backupBranch, backupBranch)
}