package git

import (
	"context"
	"strings"
	"testing"
	"time"

	catmitErrors "github.com/penwyp/catmit/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tagRunnerCall struct {
	command string
	args    []string
}

type tagRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   []tagRunnerCall
}

func newTagRunner() *tagRunner {
	return &tagRunner{
		outputs: make(map[string]string),
		errors:  make(map[string]error),
	}
}

func (r *tagRunner) Run(_ context.Context, command string, args ...string) (string, error) {
	key := command + " " + strings.Join(args, " ")
	r.calls = append(r.calls, tagRunnerCall{command: command, args: append([]string(nil), args...)})
	if err, ok := r.errors[key]; ok {
		return "", err
	}
	return r.outputs[key], nil
}

func TestTagManagerListRemoteTags(t *testing.T) {
	runner := newTagRunner()
	runner.outputs["git ls-remote --tags --refs origin"] = strings.Join([]string{
		"aaaaaaaa\trefs/tags/v1.0.0",
		"bbbbbbbb\trefs/tags/v1.2.0",
		"cccccccc\trefs/heads/main",
	}, "\n")

	manager := NewTagManager(runner)
	tags, err := manager.ListRemoteTags(context.Background(), "origin")

	require.NoError(t, err)
	assert.Equal(t, []string{"v1.0.0", "v1.2.0"}, tags)
}

func TestTagManagerBranchStatus(t *testing.T) {
	runner := newTagRunner()
	runner.outputs["git ls-remote --heads origin refs/heads/main"] = "abc123\trefs/heads/main\n"
	runner.outputs["git rev-list --count refs/remotes/origin/main..HEAD"] = "2\n"
	runner.outputs["git rev-list --count HEAD..refs/remotes/origin/main"] = "0\n"

	manager := NewTagManager(runner)
	status, err := manager.BranchStatus(context.Background(), "origin", "main")

	require.NoError(t, err)
	assert.True(t, status.RemoteExists)
	assert.Equal(t, 2, status.Ahead)
	assert.Equal(t, 0, status.Behind)
	require.Len(t, runner.calls, 4)
	assert.Equal(t, []string{"fetch", "--quiet", "--no-tags", "origin", "+refs/heads/main:refs/remotes/origin/main"}, runner.calls[1].args)
}

func TestTagManagerBranchStatusMissingRemoteBranch(t *testing.T) {
	runner := newTagRunner()

	manager := NewTagManager(runner)
	status, err := manager.BranchStatus(context.Background(), "origin", "feature")

	require.NoError(t, err)
	assert.False(t, status.RemoteExists)
	assert.Equal(t, 0, status.Ahead)
	assert.Equal(t, 0, status.Behind)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"ls-remote", "--heads", "origin", "refs/heads/feature"}, runner.calls[0].args)
}

type blockingTagRunner struct {
	calls []tagRunnerCall
}

func (r *blockingTagRunner) Run(ctx context.Context, command string, args ...string) (string, error) {
	r.calls = append(r.calls, tagRunnerCall{command: command, args: append([]string(nil), args...)})
	<-ctx.Done()
	return "", ctx.Err()
}

func TestTagManagerBranchStatusTimeout(t *testing.T) {
	runner := &blockingTagRunner{}
	manager := NewTagManager(runner)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	_, err := manager.BranchStatus(ctx, "origin", "main")

	require.Error(t, err)
	assert.Equal(t, catmitErrors.ErrTypeTimeout, catmitErrors.GetType(err))
}

func TestTagManagerPushTagUsesExplicitRef(t *testing.T) {
	runner := newTagRunner()
	manager := NewTagManager(runner)

	err := manager.PushTag(context.Background(), "origin", "v1.2.3")

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"push", "origin", "refs/tags/v1.2.3"}, runner.calls[0].args)
}

func TestTagManagerCommitMessagesSince(t *testing.T) {
	runner := newTagRunner()
	runner.outputs["git log --format=%B%x00 v1.0.0..HEAD"] = "feat: add tag command\x00\nfix: cleanup\x00"

	manager := NewTagManager(runner)
	messages, err := manager.CommitMessagesSince(context.Background(), "v1.0.0", "HEAD")

	require.NoError(t, err)
	assert.Equal(t, []string{"feat: add tag command", "fix: cleanup"}, messages)
}

func TestParseWorktreeStatus(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   WorktreeStatus
	}{
		{
			name:   "clean",
			output: "",
			want:   WorktreeStatus{},
		},
		{
			name:   "staged only",
			output: "M  file.go\nA  new.go\n",
			want: WorktreeStatus{
				HasChanges:       true,
				HasStagedChanges: true,
			},
		},
		{
			name:   "unstaged only",
			output: " M file.go\n?? new.go\n",
			want: WorktreeStatus{
				HasChanges:         true,
				HasUnstagedChanges: true,
			},
		},
		{
			name:   "mixed",
			output: "MM file.go\nR  old.go -> new.go\n?? draft.md\n",
			want: WorktreeStatus{
				HasChanges:         true,
				HasStagedChanges:   true,
				HasUnstagedChanges: true,
			},
		},
		{
			name:   "unmerged",
			output: "UU file.go\nAA both-added.go\n",
			want: WorktreeStatus{
				HasChanges:         true,
				HasUnmergedChanges: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseWorktreeStatus(tt.output))
		})
	}
}
