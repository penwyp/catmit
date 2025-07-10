package collector

import (
	"context"
)

// GitReader provides pure git command execution interface.
// This interface focuses on raw git operations without business logic.
//
// Methods in this interface directly correspond to git commands:
// - StagedDiff: `git diff --cached`
// - UnstagedDiff: `git diff`
// - GitStatus: `git status --porcelain`
// - RecentCommits: `git log --pretty=format:%s -n<count>`
// - BranchName: `git rev-parse --abbrev-ref HEAD`
//
// Example usage:
//
//	reader := collector.New(runner)
//	diff, err := reader.StagedDiff(ctx)
//	if err != nil {
//		return fmt.Errorf("failed to get staged diff: %w", err)
//	}
type GitReader interface {
	// StagedDiff returns staged changes (equivalent to `git diff --cached`)
	// Returns ErrNoDiff if no staged changes are found
	StagedDiff(ctx context.Context) (string, error)
	
	// UnstagedDiff returns unstaged changes (equivalent to `git diff`)
	// Returns empty string if no unstaged changes are found
	UnstagedDiff(ctx context.Context) (string, error)
	
	// GitStatus returns git status in porcelain format
	// Format: XY filename, where X is index status, Y is worktree status
	GitStatus(ctx context.Context) (string, error)
	
	// RecentCommits returns the last n commit messages (subject only)
	// Returns error if n <= 0 or n > 1000 (performance protection)
	RecentCommits(ctx context.Context, n int) ([]string, error)
	
	// BranchName returns the current branch name
	// Returns error if not in a git repository or branch name is invalid
	BranchName(ctx context.Context) (string, error)
}

// ChangeMagnitude represents the scale of changes in the commit
type ChangeMagnitude string

const (
	ChangeMagnitudeSmall  ChangeMagnitude = "small"  // 1-3 files changed
	ChangeMagnitudeMedium ChangeMagnitude = "medium" // 4-10 files changed
	ChangeMagnitudeLarge  ChangeMagnitude = "large"  // 11+ files changed
)

// ChangesSummary represents a high-level summary of repository changes
// This struct provides processed information ready for commit message generation
type ChangesSummary struct {
	// Existing fields
	HasStagedChanges bool
	HasUnstagedChanges bool
	TotalFiles int
	ChangeTypes map[string]int
	PrimaryChangeType string
	AffectedAreas []string
	
	// New enhanced fields for Phase 2
	UntrackedFiles    []FileStatus    // New untracked files
	HasUntrackedFiles bool             // Quick check for untracked files
	TotalChangedFiles int             // Total files including untracked
	Magnitude         ChangeMagnitude // Scale of changes
	Priority          int             // Overall change priority (1-100)
	SuggestedPrefix   string          // Suggested commit prefix (feat, fix, etc.)
	FilesByPriority   []FileStatus    // Files sorted by priority
}

// ChangeAnalyzer provides high-level analysis of repository changes.
// This interface focuses on understanding and categorizing changes
// to help generate better commit messages.
//
// The analyzer processes raw git data to extract meaningful insights:
// - Categorizes changes by type (feature, fix, refactor, etc.)
// - Identifies affected areas of the codebase
// - Determines change priority and significance
//
// Example usage:
//
//	analyzer := collector.New(runner)
//	summary, err := analyzer.AnalyzeChanges(ctx)
//	if err != nil {
//		return fmt.Errorf("failed to analyze changes: %w", err)
//	}
//	fmt.Printf("Primary change type: %s\n", summary.PrimaryChangeType)
type ChangeAnalyzer interface {
	// AnalyzeChanges provides a high-level summary of all changes
	// Combines staged and unstaged changes into a comprehensive analysis
	AnalyzeChanges(ctx context.Context) (*ChangesSummary, error)
	
	// GetPriorityFiles returns files ordered by importance for commit message generation
	// Files are sorted by: change type priority, file type importance, and path
	GetPriorityFiles(ctx context.Context) ([]FileStatus, error)
}

// FileContentProvider provides access to file contents and metadata.
// This interface focuses on reading file contents, especially for untracked files
// that might need to be included in commit message generation.
//
// This interface is particularly useful for:
// - Reading content of new files to understand their purpose
// - Accessing file metadata for better change analysis
// - Providing content-based insights for commit message generation
//
// Example usage:
//
//	provider := collector.New(runner)
//	untracked, err := provider.UntrackedFiles(ctx)
//	if err != nil {
//		return fmt.Errorf("failed to get untracked files: %w", err)
//	}
//	for _, file := range untracked {
//		content, err := provider.UntrackedFileContent(ctx, file)
//		if err != nil {
//			continue // Skip files that can't be read
//		}
//		// Analyze content for commit message hints
//	}
type FileContentProvider interface {
	// UntrackedFiles returns list of untracked files that are not ignored
	// Files are filtered to exclude build artifacts, dependencies, and binary files
	UntrackedFiles(ctx context.Context) ([]string, error)
	
	// UntrackedFileContent returns the content of an untracked file
	// Returns error if file doesn't exist, is binary, or can't be read
	// Content is truncated if file is too large (>10KB by default)
	UntrackedFileContent(ctx context.Context, path string) (string, error)
	
	// UntrackedFileAsDiff formats an untracked file's content as a diff-like output
	// This is essential for including untracked files in commit message generation
	UntrackedFileAsDiff(ctx context.Context, path string) (string, error)
}

// EnhancedDiffProvider provides comprehensive diff information including untracked files.
// This interface addresses the core issue of including untracked files in commit message generation.
type EnhancedDiffProvider interface {
	// ComprehensiveDiff returns a complete diff including staged, unstaged, and untracked files
	// This is the primary method for getting all changes for commit message generation
	ComprehensiveDiff(ctx context.Context) (string, error)
	
	// CombinedDiff returns staged and unstaged diffs combined (legacy behavior)
	CombinedDiff(ctx context.Context) (string, error)
}

// LegacyCollectorInterface represents the current monolithic collector interface.
// This interface is maintained for backward compatibility during the transition period.
// 
// DEPRECATED: Use the focused interfaces (GitReader, ChangeAnalyzer, FileContentProvider)
// instead of this monolithic interface. This will be removed in a future version.
//
// Migration guide:
// - For git operations: use GitReader
// - For change analysis: use ChangeAnalyzer  
// - For file content access: use FileContentProvider
// - For comprehensive diff: use EnhancedDiffProvider
type LegacyCollectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	Diff(ctx context.Context) (string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
	FileStatusSummary(ctx context.Context) (*FileStatusSummary, error)
}