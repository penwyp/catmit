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
	// Using ASCII control characters as separators to handle multi-line content
	// \x1f (Unit Separator) for fields, \x1e (Record Separator) for records
	args := []string{
		"log",
		"--format=%H%x1f%h%x1f%s%x1f%b%x1f%an <%ae>%x1f%aI%x1f%cn <%ce>%x1f%cI%x1f%P%x1e",
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
		"--format=%H%x1f%h%x1f%s%x1f%b%x1f%an <%ae>%x1f%aI%x1f%cn <%ce>%x1f%cI%x1f%P%x1e",
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
		"--format=%H%x1f%h%x1f%s%x1f%b%x1f%an <%ae>%x1f%aI%x1f%cn <%ce>%x1f%cI%x1f%P%x1e",
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
	// Split by record separator (\x1e)
	records := strings.Split(strings.TrimSpace(output), "\x1e")
	
	for _, record := range records {
		if record == "" {
			continue
		}
		
		// Split by field separator (\x1f)
		parts := strings.Split(record, "\x1f")
		if len(parts) < 9 {
			continue // Skip malformed records
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

