package githistory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Runner is the interface for executing git commands
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (string, error)
}

// reader implements HistoryReader interface
type reader struct {
	runner Runner
}

// NewReader creates a new HistoryReader instance
func NewReader(runner Runner) HistoryReader {
	return &reader{runner: runner}
}

// FindMergeBase finds the common ancestor between two refs
func (r *reader) FindMergeBase(ctx context.Context, ref1, ref2 string) (string, error) {
	output, err := r.runner.Run(ctx, "git", "merge-base", ref1, ref2)
	if err != nil {
		return "", fmt.Errorf("failed to find merge base between %s and %s: %w", ref1, ref2, err)
	}
	return strings.TrimSpace(output), nil
}

// GetUnpushedCommits returns commits that exist locally but not on remote
func (r *reader) GetUnpushedCommits(ctx context.Context, base, head string) ([]Commit, error) {
	// First, find commits that are in head but not in base
	// Using rev-list with format to get all needed information in one call
	args := []string{
		"log",
		"--format=%H|%h|%s|%b|%an <%ae>|%aI|%cn <%ce>|%cI|%P",
		"--no-merges", // Exclude merge commits for simplicity
		fmt.Sprintf("%s..%s", base, head),
	}
	
	output, err := r.runner.Run(ctx, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get unpushed commits: %w", err)
	}

	return r.parseCommits(output)
}

// GetCommit retrieves detailed information about a specific commit
func (r *reader) GetCommit(ctx context.Context, ref string) (*Commit, error) {
	args := []string{
		"log",
		"-1", // Only one commit
		"--format=%H|%h|%s|%b|%an <%ae>|%aI|%cn <%ce>|%cI|%P",
		ref,
	}
	
	output, err := r.runner.Run(ctx, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", ref, err)
	}

	commits, err := r.parseCommits(output)
	if err != nil {
		return nil, err
	}
	
	if len(commits) == 0 {
		return nil, fmt.Errorf("commit %s not found", ref)
	}
	
	return &commits[0], nil
}

// GetCommitsBetween returns all commits between two refs
func (r *reader) GetCommitsBetween(ctx context.Context, base, head string) ([]Commit, error) {
	// Get all commits from base (exclusive) to head (inclusive)
	args := []string{
		"log",
		"--format=%H|%h|%s|%b|%an <%ae>|%aI|%cn <%ce>|%cI|%P",
		"--reverse", // Chronological order (oldest first)
		fmt.Sprintf("%s..%s", base, head),
	}
	
	output, err := r.runner.Run(ctx, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits between %s and %s: %w", base, head, err)
	}

	return r.parseCommits(output)
}

// HasUncommittedChanges checks if there are any uncommitted changes
func (r *reader) HasUncommittedChanges(ctx context.Context) (bool, error) {
	// Check both staged and unstaged changes
	output, err := r.runner.Run(ctx, "git", "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check uncommitted changes: %w", err)
	}
	
	// If output is empty, there are no changes
	return strings.TrimSpace(output) != "", nil
}

// GetCurrentBranch returns the name of the current branch
func (r *reader) GetCurrentBranch(ctx context.Context) (string, error) {
	output, err := r.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	
	branch := strings.TrimSpace(output)
	if branch == "HEAD" {
		return "", fmt.Errorf("not on a branch (detached HEAD state)")
	}
	
	return branch, nil
}

// parseCommits parses the output of git log command into Commit structs
func (r *reader) parseCommits(output string) ([]Commit, error) {
	if strings.TrimSpace(output) == "" {
		return []Commit{}, nil
	}

	var commits []Commit
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "|")
		if len(parts) < 9 {
			continue // Skip malformed lines
		}
		
		// Parse dates
		authorDate, err := time.Parse(time.RFC3339, parts[5])
		if err != nil {
			authorDate = time.Now() // Fallback
		}
		
		commitDate, err := time.Parse(time.RFC3339, parts[7])
		if err != nil {
			commitDate = time.Now() // Fallback
		}
		
		// Parse parent SHAs
		var parentSHAs []string
		if parts[8] != "" {
			parentSHAs = strings.Split(parts[8], " ")
		}
		
		commit := Commit{
			SHA:        parts[0],
			ShortSHA:   parts[1],
			Subject:    parts[2],
			Body:       parts[3],
			Author:     parts[4],
			AuthorDate: authorDate,
			Committer:  parts[6],
			CommitDate: commitDate,
			ParentSHAs: parentSHAs,
		}
		
		commits = append(commits, commit)
	}
	
	return commits, nil
}

// GetCommitRange returns a formatted range for git commands
func GetCommitRange(commits []Commit) string {
	if len(commits) == 0 {
		return ""
	}
	
	// Get the parent of the first commit as the base
	if len(commits[0].ParentSHAs) > 0 {
		return fmt.Sprintf("%s..%s", commits[0].ParentSHAs[0], commits[len(commits)-1].SHA)
	}
	
	// Fallback to using the first commit with ~1
	return fmt.Sprintf("%s~1..%s", commits[0].SHA, commits[len(commits)-1].SHA)
}

// FormatCommitList formats a list of commits for display
func FormatCommitList(commits []Commit) string {
	var lines []string
	for i, commit := range commits {
		lines = append(lines, fmt.Sprintf("%d. %s %s", i+1, commit.ShortSHA, commit.Subject))
	}
	return strings.Join(lines, "\n")
}