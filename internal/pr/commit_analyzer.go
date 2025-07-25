package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
)

// CommitInfo represents information about a single commit
type CommitInfo struct {
	SHA     string
	Message string
	Author  string
	Date    string
}

// PRAnalysisData contains all data needed for PR generation
type PRAnalysisData struct {
	Commits      []CommitInfo
	DiffStats    string
	ChangedFiles []string
	BranchName   string
	HasTests     bool
	HasDocs      bool
	FullDiff     string
}

// CommitAnalyzer analyzes commits for PR generation
type CommitAnalyzer struct {
	git ExtendedGitRunner
	log logger.Logger
}

// NewCommitAnalyzer creates a new commit analyzer
func NewCommitAnalyzer(git ExtendedGitRunner) *CommitAnalyzer {
	return &CommitAnalyzer{
		git: git,
		log: logger.NewDefault(),
	}
}

// AnalyzeForPR performs fork-aware analysis for PR generation
func (a *CommitAnalyzer) AnalyzeForPR(ctx context.Context, prRemote, prBase string) (*PRAnalysisData, error) {
	a.log.Debugf("Analyzing commits for PR: remote=%s, base=%s", prRemote, prBase)

	// Get current branch name
	branchName, err := a.git.GetCurrentBranch(ctx)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get current branch", err)
	}

	// Try to fetch relevant commits with fallback strategy
	commits, diff, err := a.getRelevantCommits(ctx, prRemote, prBase)

	// Create analysis data
	data := &PRAnalysisData{
		BranchName: branchName,
		Commits:    commits,
	}

	// If we have diff data, analyze it
	if diff != "" {
		data.FullDiff = diff

		// Extract diff stats
		if stats, err := a.extractDiffStats(diff); err == nil {
			data.DiffStats = stats
		}

		// Extract changed files
		if files, err := a.extractChangedFiles(diff); err == nil {
			data.ChangedFiles = files

			// Check for test and doc files
			for _, file := range files {
				lowerFile := strings.ToLower(file)
				if strings.Contains(lowerFile, "test") || strings.Contains(lowerFile, "_test.") {
					data.HasTests = true
				}
				if strings.Contains(lowerFile, "readme") || strings.Contains(lowerFile, ".md") ||
					strings.Contains(lowerFile, "doc") {
					data.HasDocs = true
				}
			}
		}
	}

	// If no commits found but no error, it's okay - we'll use branch name
	if len(commits) == 0 && err != nil {
		a.log.Debugf("No commits found, will use branch name for generation: %v", err)
	}

	return data, nil
}

// getRelevantCommits implements the fork-aware fallback strategy
func (a *CommitAnalyzer) getRelevantCommits(ctx context.Context, prRemote, prBase string) ([]CommitInfo, string, error) {
	// Strategy 1: Try upstream repository (fork workflow)
	if prRemote != "origin" {
		a.log.Debugf("Trying upstream repository: %s/%s", prRemote, prBase)

		// Check if remote exists
		if err := a.checkRemoteExists(ctx, prRemote); err == nil {
			upstreamBase := fmt.Sprintf("%s/%s", prRemote, prBase)
			commits, diff, err := a.getCommitsSince(ctx, upstreamBase, "HEAD")
			if err == nil && len(commits) > 0 {
				a.log.Debugf("Found %d commits from upstream", len(commits))
				return commits, diff, nil
			}
			a.log.Debugf("Failed to get commits from upstream: %v", err)
		}
	}

	// Strategy 2: Fallback to origin's base branch
	a.log.Debugf("Trying origin/%s", prBase)
	originBase := fmt.Sprintf("origin/%s", prBase)
	commits, diff, err := a.getCommitsSince(ctx, originBase, "HEAD")
	if err == nil && len(commits) > 0 {
		a.log.Debugf("Found %d commits from origin", len(commits))
		return commits, diff, nil
	}
	a.log.Debugf("Failed to get commits from origin: %v", err)

	// Strategy 3: Try common base branches
	commonBases := []string{"main", "master", "develop"}
	for _, base := range commonBases {
		if base == prBase {
			continue // Skip if we already tried this
		}

		originBase := fmt.Sprintf("origin/%s", base)
		commits, diff, err := a.getCommitsSince(ctx, originBase, "HEAD")
		if err == nil && len(commits) > 0 {
			a.log.Debugf("Found %d commits from origin/%s", len(commits), base)
			return commits, diff, nil
		}
	}

	// No commits found - return empty but not an error
	// This allows fallback to branch name generation
	return nil, "", errors.New(errors.ErrTypeGit, "no base branch found for comparison")
}

// checkRemoteExists checks if a git remote exists
func (a *CommitAnalyzer) checkRemoteExists(ctx context.Context, remote string) error {
	// Try to get remote URL to check if it exists
	_, err := a.git.GetRemoteURL(ctx, remote)
	return err
}

// getCommitsSince gets commits between base and head
func (a *CommitAnalyzer) getCommitsSince(ctx context.Context, base, head string) ([]CommitInfo, string, error) {
	// Get commit SHAs
	output, err := a.git.Run(ctx, "git", "rev-list", "--reverse", fmt.Sprintf("%s..%s", base, head))
	if err != nil {
		return nil, "", err
	}

	// Parse commit SHAs
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, "", errors.New(errors.ErrTypeGit, "no commits found")
	}

	commits := make([]CommitInfo, 0, len(lines))

	// Get detailed info for each commit
	for _, sha := range lines {
		if sha == "" {
			continue
		}

		// Get commit message
		msgOutput, err := a.git.Run(ctx, "git", "log", "-1", "--pretty=format:%B", sha)
		if err != nil {
			continue
		}

		// Get commit author and date
		authorOutput, err := a.git.Run(ctx, "git", "log", "-1", "--pretty=format:%an", sha)
		if err != nil {
			continue
		}

		dateOutput, err := a.git.Run(ctx, "git", "log", "-1", "--pretty=format:%ai", sha)
		if err != nil {
			continue
		}

		commits = append(commits, CommitInfo{
			SHA:     sha,
			Message: strings.TrimSpace(string(msgOutput)),
			Author:  strings.TrimSpace(string(authorOutput)),
			Date:    strings.TrimSpace(string(dateOutput)),
		})
	}

	// Get full diff
	diffOutput, err := a.git.Run(ctx, "git", "diff", fmt.Sprintf("%s..%s", base, head))
	if err != nil {
		// Diff error is not fatal - we still have commits
		return commits, "", nil
	}

	return commits, string(diffOutput), nil
}

// extractDiffStats extracts statistics from diff
func (a *CommitAnalyzer) extractDiffStats(diff string) (string, error) {
	// Get diff stat
	lines := strings.Split(diff, "\n")
	var statsLines []string
	inStats := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			inStats = false
		}
		if inStats && line != "" {
			statsLines = append(statsLines, line)
		}
		if strings.Contains(line, "files changed") || strings.Contains(line, "file changed") {
			statsLines = append(statsLines, line)
			inStats = true
		}
	}

	if len(statsLines) > 0 {
		return strings.Join(statsLines, "\n"), nil
	}

	// If no stats found, try to generate them
	// This would need to parse the diff manually
	return "", nil
}

// extractChangedFiles extracts list of changed files from diff
func (a *CommitAnalyzer) extractChangedFiles(diff string) ([]string, error) {
	files := make(map[string]bool)
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			// Extract file paths from diff --git a/path b/path
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				// Remove a/ or b/ prefix
				for i := 2; i < len(parts); i++ {
					file := parts[i]
					if strings.HasPrefix(file, "a/") || strings.HasPrefix(file, "b/") {
						file = file[2:]
					}
					files[file] = true
				}
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(files))
	for file := range files {
		result = append(result, file)
	}

	return result, nil
}
