package git

import (
	"context"
	"sort"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
)

// remoteManager implements the Git remote repository manager
type remoteManager struct {
	runner Runner
}

// NewRemoteManager creates a new remote repository manager
func NewRemoteManager(runner Runner) RemoteManager {
	return &remoteManager{
		runner: runner,
	}
}

// GetRemotes retrieves all remote repositories
func (m *remoteManager) GetRemotes(ctx context.Context) ([]Remote, error) {
	output, err := m.runner.Run(ctx, "git", "remote", "-v")
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get remotes", err)
	}

	// Parse the output of 'git remote -v'
	remotes := make(map[string]*Remote)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Format: origin	https://github.com/owner/repo.git (fetch)
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		url := parts[1]
		typeStr := strings.Trim(parts[2], "()")

		if _, exists := remotes[name]; !exists {
			remotes[name] = &Remote{Name: name}
		}

		if typeStr == "fetch" {
			remotes[name].FetchURL = url
		} else if typeStr == "push" {
			remotes[name].PushURL = url
		}
	}

	// Convert to slice and sort by name to ensure consistent order
	result := make([]Remote, 0, len(remotes))
	for _, remote := range remotes {
		result = append(result, *remote)
	}

	// Sort by remote name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// SelectRemote selects a remote repository by priority
func (m *remoteManager) SelectRemote(remotes []Remote, preferredName string) (*Remote, error) {
	if len(remotes) == 0 {
		return nil, errors.New(errors.ErrTypeGit, "no remotes configured")
	}

	// If a remote name is specified, select it
	if preferredName != "" {
		for _, remote := range remotes {
			if remote.Name == preferredName {
				return &remote, nil
			}
		}
		return nil, errors.Newf(errors.ErrTypeGit, "remote '%s' not found", preferredName)
	}

	// By default, look for 'origin'
	for _, remote := range remotes {
		if remote.Name == "origin" {
			return &remote, nil
		}
	}

	return nil, errors.New(errors.ErrTypeGit, "no 'origin' remote found and no remote specified")
}

// GetCurrentBranch retrieves the current branch name
func (m *remoteManager) GetCurrentBranch(ctx context.Context) (string, error) {
	output, err := m.runner.Run(ctx, "git", "branch", "--show-current")
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get current branch", err)
	}

	branch := strings.TrimSpace(output)
	if branch == "" {
		return "", errors.New(errors.ErrTypeGit, "not on any branch (detached HEAD)")
	}

	return branch, nil
}

// HasUpstreamBranch checks if the branch has an upstream branch
func (m *remoteManager) HasUpstreamBranch(ctx context.Context, branch string) bool {
	_, err := m.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	return err == nil
}
