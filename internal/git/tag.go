package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
)

type BranchStatus struct {
	RemoteExists bool
	Ahead        int
	Behind       int
}

type WorktreeStatus struct {
	HasChanges         bool
	HasStagedChanges   bool
	HasUnstagedChanges bool
	HasUnmergedChanges bool
}

type TagManager interface {
	CurrentBranch(ctx context.Context) (string, error)
	HeadSHA(ctx context.Context, short bool) (string, error)
	WorktreeStatus(ctx context.Context) (WorktreeStatus, error)
	StageAll(ctx context.Context) error
	Commit(ctx context.Context, message string) error
	BranchStatus(ctx context.Context, remote, branch string) (BranchStatus, error)
	PushBranch(ctx context.Context, remote, branch string) error
	ListRemoteTags(ctx context.Context, remote string) ([]string, error)
	LocalTagExists(ctx context.Context, tagName string) (bool, error)
	RemoteTagExists(ctx context.Context, remote, tagName string) (bool, error)
	CreateAnnotatedTag(ctx context.Context, tagName, message, target string) error
	PushTag(ctx context.Context, remote, tagName string) error
	CommitMessagesSince(ctx context.Context, fromRef, toRef string) ([]string, error)
	ResolveRevision(ctx context.Context, ref string) (string, error)
}

type tagManager struct {
	runner Runner
}

func NewTagManager(runner Runner) TagManager {
	return &tagManager{runner: runner}
}

func (m *tagManager) CurrentBranch(ctx context.Context) (string, error) {
	output, err := m.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get current branch", err)
	}

	branch := strings.TrimSpace(output)
	if branch == "" || branch == "HEAD" {
		return "", errors.New(errors.ErrTypeGit, "not on any branch (detached HEAD)")
	}
	return branch, nil
}

func (m *tagManager) HeadSHA(ctx context.Context, short bool) (string, error) {
	args := []string{"rev-parse"}
	if short {
		args = append(args, "--short")
	}
	args = append(args, "HEAD")

	output, err := m.runner.Run(ctx, "git", args...)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get HEAD revision", err)
	}
	return strings.TrimSpace(output), nil
}

func (m *tagManager) WorktreeStatus(ctx context.Context) (WorktreeStatus, error) {
	output, err := m.runner.Run(ctx, "git", "status", "--porcelain")
	if err != nil {
		return WorktreeStatus{}, errors.Wrap(errors.ErrTypeGit, "failed to inspect worktree status", err)
	}
	return parseWorktreeStatus(output), nil
}

func (m *tagManager) StageAll(ctx context.Context) error {
	_, err := m.runner.Run(ctx, "git", "add", "-A")
	if err != nil {
		return errors.Wrap(errors.ErrTypeGit, "failed to stage files", err)
	}
	return nil
}

func (m *tagManager) Commit(ctx context.Context, message string) error {
	_, err := m.runner.Run(ctx, "git", "commit", "-m", message)
	if err != nil {
		return errors.Wrap(errors.ErrTypeGit, "failed to create commit", err)
	}
	return nil
}

func (m *tagManager) BranchStatus(ctx context.Context, remote, branch string) (BranchStatus, error) {
	exists, err := m.remoteBranchExists(ctx, remote, branch)
	if err != nil {
		return BranchStatus{}, err
	}
	if !exists {
		return BranchStatus{RemoteExists: false}, nil
	}

	if err := m.fetchBranch(ctx, remote, branch); err != nil {
		return BranchStatus{}, err
	}

	remoteRef := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	ahead, err := m.revisionCount(ctx, remoteRef+"..HEAD")
	if err != nil {
		return BranchStatus{}, err
	}

	behind, err := m.revisionCount(ctx, "HEAD.."+remoteRef)
	if err != nil {
		return BranchStatus{}, err
	}

	return BranchStatus{
		RemoteExists: true,
		Ahead:        ahead,
		Behind:       behind,
	}, nil
}

func (m *tagManager) remoteBranchExists(ctx context.Context, remote, branch string) (bool, error) {
	output, err := m.runner.Run(ctx, "git", "ls-remote", "--heads", remote, "refs/heads/"+branch)
	if err != nil {
		return false, wrapGitCommandError(ctx, err, "failed to inspect remote branch %s/%s", remote, branch)
	}
	return strings.TrimSpace(output) != "", nil
}

func (m *tagManager) fetchBranch(ctx context.Context, remote, branch string) error {
	remoteHeadRef := "refs/heads/" + branch
	remoteTrackingRef := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	refspec := "+" + remoteHeadRef + ":" + remoteTrackingRef

	_, err := m.runner.Run(ctx, "git", "fetch", "--quiet", "--no-tags", remote, refspec)
	if err != nil {
		return wrapGitCommandError(ctx, err, "failed to fetch remote branch %s/%s", remote, branch)
	}
	return nil
}

func (m *tagManager) PushBranch(ctx context.Context, remote, branch string) error {
	_, err := m.runner.Run(ctx, "git", "push", remote, branch)
	if err != nil {
		return wrapGitCommandError(ctx, err, "failed to push branch %s to %s", branch, remote)
	}
	return nil
}

func (m *tagManager) ListRemoteTags(ctx context.Context, remote string) ([]string, error) {
	output, err := m.runner.Run(ctx, "git", "ls-remote", "--tags", "--refs", remote)
	if err != nil {
		return nil, wrapGitCommandError(ctx, err, "failed to list remote tags from %q", remote)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	tags := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "refs/tags/")
		if name != "" && name != fields[1] {
			tags = append(tags, name)
		}
	}
	return tags, nil
}

func (m *tagManager) LocalTagExists(ctx context.Context, tagName string) (bool, error) {
	_, err := m.runner.Run(ctx, "git", "rev-parse", "--quiet", "--verify", "refs/tags/"+tagName)
	return err == nil, nil
}

func (m *tagManager) RemoteTagExists(ctx context.Context, remote, tagName string) (bool, error) {
	output, err := m.runner.Run(ctx, "git", "ls-remote", "--tags", "--refs", remote, "refs/tags/"+tagName)
	if err != nil {
		return false, wrapGitCommandError(ctx, err, "failed to check remote tag %q", tagName)
	}
	return strings.TrimSpace(output) != "", nil
}

func (m *tagManager) CreateAnnotatedTag(ctx context.Context, tagName, message, target string) error {
	if target == "" {
		target = "HEAD"
	}

	_, err := m.runner.Run(ctx, "git", "tag", "-a", tagName, "-m", message, target)
	if err != nil {
		return errors.Wrapf(errors.ErrTypeGit, "failed to create tag %q", err, tagName)
	}
	return nil
}

func (m *tagManager) PushTag(ctx context.Context, remote, tagName string) error {
	_, err := m.runner.Run(ctx, "git", "push", remote, "refs/tags/"+tagName)
	if err != nil {
		return wrapGitCommandError(ctx, err, "failed to push tag %q to %s", tagName, remote)
	}
	return nil
}

func (m *tagManager) CommitMessagesSince(ctx context.Context, fromRef, toRef string) ([]string, error) {
	if toRef == "" {
		toRef = "HEAD"
	}

	rangeSpec := toRef
	if fromRef != "" {
		rangeSpec = fromRef + ".." + toRef
	}

	output, err := m.runner.Run(ctx, "git", "log", "--format=%B%x00", rangeSpec)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to inspect commit history", err)
	}

	parts := strings.Split(output, "\x00")
	messages := make([]string, 0, len(parts))
	for _, part := range parts {
		message := strings.TrimSpace(part)
		if message != "" {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (m *tagManager) ResolveRevision(ctx context.Context, ref string) (string, error) {
	output, err := m.runner.Run(ctx, "git", "rev-parse", ref)
	if err != nil {
		return "", errors.Wrapf(errors.ErrTypeGit, "failed to resolve revision %q", err, ref)
	}
	return strings.TrimSpace(output), nil
}

func (m *tagManager) revisionCount(ctx context.Context, rangeSpec string) (int, error) {
	output, err := m.runner.Run(ctx, "git", "rev-list", "--count", rangeSpec)
	if err != nil {
		return 0, wrapGitCommandError(ctx, err, "failed to count revisions in %q", rangeSpec)
	}

	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, errors.Wrapf(errors.ErrTypeGit, "invalid revision count for %q", err, rangeSpec)
	}
	return count, nil
}

func wrapGitCommandError(ctx context.Context, err error, format string, args ...interface{}) error {
	message := fmt.Sprintf(format, args...)
	if ctxErr := ctx.Err(); ctxErr != nil {
		if ctxErr == context.DeadlineExceeded {
			return errors.WrapRetryable(errors.ErrTypeTimeout, message, ctxErr).
				WithSuggestion("Increase --timeout or check network/authentication for the remote")
		}
		return errors.Wrap(errors.ErrTypeGit, message, ctxErr)
	}
	return errors.Wrap(errors.ErrTypeGit, message, err)
}

func parseWorktreeStatus(output string) WorktreeStatus {
	var status WorktreeStatus
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if len(line) < 2 {
			continue
		}

		indexStatus := line[0]
		worktreeStatus := line[1]
		if isUnmergedStatus(indexStatus, worktreeStatus) {
			status.HasChanges = true
			status.HasUnmergedChanges = true
			continue
		}
		if indexStatus == '?' && worktreeStatus == '?' {
			status.HasChanges = true
			status.HasUnstagedChanges = true
			continue
		}

		if indexStatus != ' ' && indexStatus != '?' {
			status.HasChanges = true
			status.HasStagedChanges = true
		}
		if worktreeStatus != ' ' && worktreeStatus != '?' {
			status.HasChanges = true
			status.HasUnstagedChanges = true
		}
	}
	return status
}

func isUnmergedStatus(indexStatus, worktreeStatus byte) bool {
	switch {
	case indexStatus == 'U' || worktreeStatus == 'U':
		return true
	case indexStatus == 'A' && worktreeStatus == 'A':
		return true
	case indexStatus == 'D' && worktreeStatus == 'D':
		return true
	case indexStatus == 'D' && worktreeStatus == 'U':
		return true
	case indexStatus == 'U' && worktreeStatus == 'D':
		return true
	case indexStatus == 'A' && worktreeStatus == 'U':
		return true
	case indexStatus == 'U' && worktreeStatus == 'A':
		return true
	default:
		return false
	}
}
