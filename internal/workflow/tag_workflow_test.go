package workflow

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/penwyp/catmit/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tagWorkflowRunner struct{}

func (r tagWorkflowRunner) Run(_ context.Context, command string, args ...string) (string, error) {
	if command == "git" && strings.Join(args, " ") == "rev-parse --git-dir" {
		return ".git\n", nil
	}
	return "", nil
}

type fakeTagManager struct {
	mu              sync.Mutex
	branch          string
	head            string
	worktreeStatus  git.WorktreeStatus
	branchStatus    git.BranchStatus
	remoteTags      []string
	commitMessages  []string
	historyFrom     string
	unreachable     map[string]bool
	createdTarget   string
	localTagExists  bool
	remoteTagExists bool
	remoteTagChecks []bool
	calls           []string
}

func (m *fakeTagManager) appendCall(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

func (m *fakeTagManager) popRemoteTagCheck() (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.remoteTagChecks) == 0 {
		return false, false
	}
	result := m.remoteTagChecks[0]
	m.remoteTagChecks = m.remoteTagChecks[1:]
	return result, true
}

func (m *fakeTagManager) Calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

func (m *fakeTagManager) CurrentBranch(context.Context) (string, error) {
	m.appendCall("current-branch")
	return m.branch, nil
}

func (m *fakeTagManager) HeadSHA(context.Context, bool) (string, error) {
	m.appendCall("head")
	return m.head, nil
}

func (m *fakeTagManager) WorktreeStatus(context.Context) (git.WorktreeStatus, error) {
	m.appendCall("status")
	return m.worktreeStatus, nil
}

func (m *fakeTagManager) StageAll(context.Context) error {
	m.appendCall("stage")
	return nil
}

func (m *fakeTagManager) Commit(_ context.Context, message string) error {
	m.appendCall("commit:" + message)
	return nil
}

func (m *fakeTagManager) BranchStatus(context.Context, string, string) (git.BranchStatus, error) {
	m.appendCall("branch-status")
	return m.branchStatus, nil
}

func (m *fakeTagManager) PushBranch(_ context.Context, remote, branch string) error {
	m.appendCall("push-branch:" + remote + "/" + branch)
	return nil
}

func (m *fakeTagManager) ListRemoteTags(context.Context, string) ([]string, error) {
	m.appendCall("list-tags")
	return m.remoteTags, nil
}

func (m *fakeTagManager) LocalTagExists(context.Context, string) (bool, error) {
	m.appendCall("local-tag-exists")
	return m.localTagExists, nil
}

func (m *fakeTagManager) RemoteTagExists(context.Context, string, string) (bool, error) {
	m.appendCall("remote-tag-exists")
	if result, ok := m.popRemoteTagCheck(); ok {
		return result, nil
	}
	return m.remoteTagExists, nil
}

func (m *fakeTagManager) CreateAnnotatedTag(_ context.Context, tagName, _, target string) error {
	m.createdTarget = target
	m.appendCall("create-tag:" + tagName)
	return nil
}

func (m *fakeTagManager) PushTag(_ context.Context, remote, tagName string) error {
	m.appendCall("push-tag:" + remote + "/" + tagName)
	return nil
}

func (m *fakeTagManager) CommitMessagesSince(_ context.Context, from, _ string) ([]string, error) {
	m.historyFrom = from
	m.appendCall("commit-messages")
	return m.commitMessages, nil
}

func (m *fakeTagManager) FetchRemoteTag(_ context.Context, _, tag string) (string, error) {
	m.appendCall("fetch-tag:" + tag)
	return "sha:" + tag, nil
}

func (m *fakeTagManager) IsAncestor(_ context.Context, ancestor, _ string) (bool, error) {
	return !m.unreachable[ancestor], nil
}

func TestTagWorkflowPlanAutoMinor(t *testing.T) {
	manager := &fakeTagManager{
		branch:         "main",
		head:           "abc123",
		branchStatus:   git.BranchStatus{RemoteExists: true},
		remoteTags:     []string{"v1.2.3"},
		commitMessages: []string{"feat: add tag command"},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)

	plan, err := workflow.Plan(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "main", plan.Branch)
	assert.False(t, plan.PushBranch)
	assert.Equal(t, "v1.2.3", plan.LatestTag)
	assert.Equal(t, "v1.3.0-dev.1+20261105T153000UTC8", plan.NextTag)
}

func TestTagWorkflowPlanRejectsBehindBranch(t *testing.T) {
	manager := &fakeTagManager{
		branch:       "main",
		head:         "abc123",
		branchStatus: git.BranchStatus{RemoteExists: true, Behind: 1},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)

	_, err := workflow.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "behind origin/main")
}

func TestTagWorkflowPlanRejectsUnstagedChanges(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "abc123",
		worktreeStatus: git.WorktreeStatus{
			HasChanges:         true,
			HasUnstagedChanges: true,
		},
		branchStatus: git.BranchStatus{RemoteExists: true},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)

	_, err := workflow.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestTagWorkflowPlanRejectsUnmergedChanges(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "abc123",
		worktreeStatus: git.WorktreeStatus{
			HasChanges:         true,
			HasUnmergedChanges: true,
		},
		branchStatus: git.BranchStatus{RemoteExists: true},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)

	_, err := workflow.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved merge conflicts")
}

func TestTagWorkflowPlanRejectsMixedChanges(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "abc123",
		worktreeStatus: git.WorktreeStatus{
			HasChanges:         true,
			HasStagedChanges:   true,
			HasUnstagedChanges: true,
		},
		branchStatus: git.BranchStatus{RemoteExists: true},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)

	_, err := workflow.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestTagWorkflowExecutePushesBranchThenPinnedTag(t *testing.T) {
	manager := &fakeTagManager{branch: "main", head: "abc123"}
	workflow := newTestTagWorkflow(manager, nil, nil)
	plan := &ReleasePlan{Remote: "origin", Branch: "main", Head: "abc123", PushBranch: true, NextTag: "v1.3.0"}
	require.NoError(t, workflow.Execute(context.Background(), plan))
	calls := manager.Calls()
	assert.NotContains(t, calls, "stage")
	assert.NotContains(t, strings.Join(calls, ","), "commit:")
	assert.Equal(t, 2, countCalls(calls, "remote-tag-exists"))
	assert.Equal(t, 2, countCalls(calls, "local-tag-exists"))
	assert.Equal(t, plan.Head, manager.createdTarget)
	assertCallOrder(t, calls, "push-branch:origin/main", "create-tag:v1.3.0", "push-tag:origin/v1.3.0")
}

func TestTagWorkflowExecuteSkipsSecondTagAvailabilityCheckWhenNothingMutates(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "abc123",
	}
	workflow := newTestTagWorkflow(manager, nil, nil)
	plan := &ReleasePlan{
		Remote:  "origin",
		Branch:  "main",
		Head:    "abc123",
		NextTag: "v1.3.0",
	}

	err := workflow.Execute(context.Background(), plan)

	require.NoError(t, err)
	calls := manager.Calls()
	assert.Equal(t, 1, countCalls(calls, "remote-tag-exists"))
	assert.Equal(t, 1, countCalls(calls, "local-tag-exists"))
	assert.NotContains(t, strings.Join(calls, ","), "push-branch")
	assert.NotContains(t, strings.Join(calls, ","), "commit:")
	assertCallOrder(t, calls, "create-tag:v1.3.0", "push-tag:origin/v1.3.0")
}

func TestTagWorkflowExecuteRechecksRemoteTagBeforeCreatingLocalTag(t *testing.T) {
	manager := &fakeTagManager{
		branch:          "main",
		head:            "abc123",
		remoteTagChecks: []bool{false, true},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)
	plan := &ReleasePlan{
		Remote:     "origin",
		Branch:     "main",
		Head:       "abc123",
		PushBranch: true,
		NextTag:    "v1.3.0",
	}

	err := workflow.Execute(context.Background(), plan)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote tag v1.3.0 already exists")
	assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag")
}

func TestTagWorkflowExecuteRejectsChangedHead(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "def456",
	}
	workflow := newTestTagWorkflow(manager, nil, nil)
	plan := &ReleasePlan{
		Remote:  "origin",
		Branch:  "main",
		Head:    "abc123",
		NextTag: "v1.3.0",
	}

	err := workflow.Execute(context.Background(), plan)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "current HEAD changed")
	assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag")
}

func TestTagWorkflowExecuteRejectsDirtyWorktreeWhenPlanSkippedCommit(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "abc123",
		worktreeStatus: git.WorktreeStatus{
			HasChanges:         true,
			HasUnstagedChanges: true,
		},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)
	plan := &ReleasePlan{
		Remote:  "origin",
		Branch:  "main",
		Head:    "abc123",
		NextTag: "v1.3.0",
	}

	err := workflow.Execute(context.Background(), plan)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
	assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag")
}

func TestTagWorkflowRunCancelDoesNotExecute(t *testing.T) {
	manager := &fakeTagManager{
		branch:         "main",
		head:           "abc123",
		branchStatus:   git.BranchStatus{RemoteExists: true},
		remoteTags:     []string{"v1.2.3"},
		commitMessages: []string{"fix: release bug"},
	}
	var out bytes.Buffer
	workflow := newTestTagWorkflow(manager, &out, strings.NewReader("n\n"))

	err := workflow.Run(context.Background())

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Canceled.")
	assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag")
}

func TestTagWorkflowRunTreatsEOFConfirmationAsCancel(t *testing.T) {
	manager := &fakeTagManager{
		branch:         "main",
		head:           "abc123",
		branchStatus:   git.BranchStatus{RemoteExists: true},
		remoteTags:     []string{"v1.2.3"},
		commitMessages: []string{"fix: release bug"},
	}
	var out bytes.Buffer
	workflow := newTestTagWorkflow(manager, &out, strings.NewReader(""))

	err := workflow.Run(context.Background())

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Canceled.")
	assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag")
}

func countCalls(calls []string, want string) int {
	count := 0
	for _, call := range calls {
		if call == want {
			count++
		}
	}
	return count
}

func assertCallOrder(t *testing.T, calls []string, ordered ...string) {
	t.Helper()

	lastIndex := -1
	for _, want := range ordered {
		index := -1
		for i := lastIndex + 1; i < len(calls); i++ {
			if calls[i] == want {
				index = i
				break
			}
		}
		require.NotEqual(t, -1, index, "call %q not found after index %d in %v", want, lastIndex, calls)
		lastIndex = index
	}
}

func newTestTagWorkflow(manager *fakeTagManager, output *bytes.Buffer, input *strings.Reader) *TagWorkflow {
	if output == nil {
		output = &bytes.Buffer{}
	}
	if input == nil {
		input = strings.NewReader("y\n")
	}

	workflow := NewTagWorkflow(tagWorkflowRunner{}, TagOptions{
		Timeout: 30, Remote: "origin", Bump: "auto", InitialVersion: "v0.1.0", Timezone: "Asia/Shanghai",
	}, output, input)
	workflow.manager = manager
	workflow.now = func() time.Time { return time.Date(2026, 11, 5, 7, 30, 0, 0, time.UTC) }
	return workflow
}

func TestTagWorkflowReleaseTargets(t *testing.T) {
	for _, tt := range []struct {
		name     string
		tags     []string
		messages []string
		pre      string
		bump     string
		release  bool
		want     string
	}{
		{name: "initial", want: "v0.1.0-dev.1"},
		{name: "initial stable", release: true, want: "v0.1.0"},
		{name: "unprefixed", tags: []string{"1.2.3"}, messages: []string{"fix: bug"}, want: "1.2.4-dev.1"},
		{name: "continue counter", tags: []string{"v1.2.3", "v1.2.4-dev.1+20261104T010000UTC8"}, messages: []string{"fix: bug"}, want: "v1.2.4-dev.2"},
		{name: "rc channel", tags: []string{"v1.2.3", "v1.2.4-dev.8"}, messages: []string{"fix: bug"}, pre: "rc", want: "v1.2.4-rc.1"},
		{name: "promote same commit", tags: []string{"v1.2.3", "v1.2.4-rc.1"}, messages: []string{"fix: bug"}, release: true, want: "v1.2.4"},
		{name: "promote first cycle", tags: []string{"1.3.0-rc.1"}, messages: []string{"initial commit"}, release: true, want: "1.3.0"},
		{name: "preserve explicit higher target", tags: []string{"v1.2.3", "v1.3.0-dev.1"}, messages: []string{"fix: bug"}, want: "v1.3.0-dev.2"},
		{name: "new feature raises core", tags: []string{"v1.2.3", "v1.2.4-dev.9"}, messages: []string{"fix: bug", "feat: new feature"}, want: "v1.3.0-dev.1"},
		{name: "breaking raises core", tags: []string{"v1.2.3", "v1.3.0-dev.1"}, messages: []string{"feat!: breaking api"}, want: "v2.0.0-dev.1"},
		{name: "explicit bump selects maintenance", tags: []string{"v1.2.3", "v1.3.0-dev.1"}, messages: []string{"feat: feature"}, bump: "patch", want: "v1.2.4-dev.1"},
		{name: "new cycle", tags: []string{"v1.2.4", "v1.2.4-dev.9"}, messages: []string{"fix: next bug"}, want: "v1.2.5-dev.1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeTagManager{branch: "main", head: "abc123", remoteTags: tt.tags, commitMessages: tt.messages}
			wf := newTestTagWorkflow(manager, nil, nil)
			wf.options.NoTimestamp = true
			wf.options.Pre, wf.options.Release = tt.pre, tt.release
			if tt.bump != "" {
				wf.options.Bump = tt.bump
			}
			plan, err := wf.Plan(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, plan.NextTag)
			if plan.StableTag != "" {
				assert.Equal(t, "sha:"+plan.StableTag, manager.historyFrom)
			}
		})
	}
}

func TestTagWorkflowIgnoresUnreachableTags(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main", head: "abc123", remoteTags: []string{"v9.0.0", "1.2.3"},
		commitMessages: []string{"fix: bug"}, unreachable: map[string]bool{"sha:v9.0.0": true},
	}
	wf := newTestTagWorkflow(manager, nil, nil)
	wf.options.NoTimestamp = true
	plan, err := wf.Plan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", plan.StableTag)
	assert.Equal(t, "1.2.4-dev.1", plan.NextTag)
}

func TestTagWorkflowAutoRequiresConventionalCommits(t *testing.T) {
	manager := &fakeTagManager{branch: "main", head: "abc123", remoteTags: []string{"v1.2.3"}, commitMessages: []string{"made changes"}}
	wf := newTestTagWorkflow(manager, nil, nil)
	_, err := wf.Plan(context.Background())
	require.ErrorContains(t, err, "specify --bump")
	wf.options.Bump = "minor"
	plan, err := wf.Plan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v1.3.0-dev.1+20261105T153000UTC8", plan.NextTag)
}

func TestTagWorkflowRejectsNoChangesSinceStable(t *testing.T) {
	wf := newTestTagWorkflow(&fakeTagManager{branch: "main", head: "abc123", remoteTags: []string{"v1.2.3"}}, nil, nil)
	_, err := wf.Plan(context.Background())
	require.ErrorContains(t, err, "no new commit since latest stable")
}

func TestTagWorkflowExplicitTagIsExact(t *testing.T) {
	wf := newTestTagWorkflow(&fakeTagManager{branch: "main", head: "abc123", remoteTags: []string{"v1.2.3"}, commitMessages: []string{"anything"}}, nil, nil)
	wf.options.ExplicitTag = "1.2.4-rc.2+custom"
	plan, err := wf.Plan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, wf.options.ExplicitTag, plan.NextTag)
	wf.options.ExplicitTag = "v1.2.3+different"
	_, err = wf.Plan(context.Background())
	require.ErrorContains(t, err, "must be greater")
}

func TestTagWorkflowDryRunAndTimestampStayFixed(t *testing.T) {
	manager := &fakeTagManager{branch: "main", head: "abc123", remoteTags: []string{"v1.2.3"}, commitMessages: []string{"fix: bug"}}
	var out bytes.Buffer
	wf := newTestTagWorkflow(manager, &out, nil)
	wf.options.Timezone = "Asia/Tokyo"
	wf.options.DryRun = true
	require.NoError(t, wf.Run(context.Background()))
	assert.Contains(t, out.String(), "v1.2.4-dev.1+20261105T163000UTC9")
	assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag:")
	plan, err := wf.Plan(context.Background())
	require.NoError(t, err)
	wf.now = func() time.Time { panic("execution must use the confirmed timestamp") }
	require.NoError(t, wf.Execute(context.Background(), plan))
	assert.Contains(t, manager.Calls(), "create-tag:"+plan.NextTag)
}

func TestTagWorkflowRejectsConcurrentSemanticRelease(t *testing.T) {
	for _, remoteTag := range []string{
		"1.2.4-dev.1+other-time", "v1.2.4-dev.2", "v1.2.4",
	} {
		t.Run(remoteTag, func(t *testing.T) {
			manager := &fakeTagManager{branch: "main", head: "abc123", remoteTags: []string{"v1.2.3"}, commitMessages: []string{"fix: bug"}}
			wf := newTestTagWorkflow(manager, nil, nil)
			plan, err := wf.Plan(context.Background())
			require.NoError(t, err)
			manager.remoteTags = append(manager.remoteTags, remoteTag)
			err = wf.Execute(context.Background(), plan)
			require.ErrorContains(t, err, "conflicts with planned tag")
			assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag:")
		})
	}
}

func TestTagPrintFallsBackForLegacyCommits(t *testing.T) {
	for _, tt := range []struct {
		name     string
		messages []string
		want     string
		bump     string
	}{
		{"legacy only", []string{"update files", "Merge branch 'feature'"}, "v1.2.4-dev.1\n", "patch"},
		{"legacy and feature", []string{"update files", "feat: new feature"}, "v1.3.0-dev.1\n", "minor"},
		{"legacy and breaking", []string{"Merge branch 'feature'", "fix!: drop old config"}, "v2.0.0-dev.1\n", "major"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeTagManager{head: "abc123", remoteTags: []string{"v1.2.3"}, commitMessages: tt.messages}
			var stdout bytes.Buffer
			wf := newTestTagWorkflow(manager, &stdout, nil)
			wf.options.Print, wf.options.NoTimestamp = true, true
			require.NoError(t, wf.Run(context.Background()))
			assert.Equal(t, tt.want, stdout.String())
			assert.NotContains(t, strings.Join(manager.Calls(), ","), "create-tag:")
			assert.NotContains(t, strings.Join(manager.Calls(), ","), "push-tag:")
			assert.NotContains(t, manager.Calls(), "current-branch")
		})
	}
}
