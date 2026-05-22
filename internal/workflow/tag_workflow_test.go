package workflow

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func (m *fakeTagManager) CreateAnnotatedTag(_ context.Context, tagName, _, _ string) error {
	m.appendCall("create-tag:" + tagName)
	return nil
}

func (m *fakeTagManager) PushTag(_ context.Context, remote, tagName string) error {
	m.appendCall("push-tag:" + remote + "/" + tagName)
	return nil
}

func (m *fakeTagManager) CommitMessagesSince(context.Context, string, string) ([]string, error) {
	m.appendCall("commit-messages")
	return m.commitMessages, nil
}

func (m *fakeTagManager) ResolveRevision(context.Context, string) (string, error) {
	m.appendCall("resolve")
	return m.head, nil
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
	assert.False(t, plan.CommitNeeded)
	assert.False(t, plan.PushBranch)
	assert.Equal(t, "v1.2.3", plan.LatestTag)
	assert.Equal(t, "v1.3.0", plan.NextTag)
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

func TestTagWorkflowPlanRejectsStageAllFalseWithoutStagedChanges(t *testing.T) {
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
	workflow.config.StageAll = false

	_, err := workflow.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "none are staged")
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

func TestTagWorkflowPlanRejectsStageAllFalseWithMixedChanges(t *testing.T) {
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
	workflow.config.StageAll = false

	_, err := workflow.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unstaged or untracked changes")
}

func TestTagWorkflowExecuteRunsCommitPushTagPushTag(t *testing.T) {
	manager := &fakeTagManager{
		branch: "main",
		head:   "abc123",
		worktreeStatus: git.WorktreeStatus{
			HasChanges:       true,
			HasStagedChanges: true,
		},
	}
	workflow := newTestTagWorkflow(manager, nil, nil)
	workflow.config.StageAll = true
	plan := &ReleasePlan{
		Remote:        "origin",
		Branch:        "main",
		Head:          "abc123",
		CommitNeeded:  true,
		CommitMessage: "feat: release command",
		PushBranch:    true,
		NextTag:       "v1.3.0",
	}

	err := workflow.Execute(context.Background(), plan)

	require.NoError(t, err)
	calls := manager.Calls()
	assert.Subset(t, calls, []string{
		"current-branch",
		"head",
		"status",
		"stage",
		"commit:feat: release command",
		"push-branch:origin/main",
		"create-tag:v1.3.0",
		"push-tag:origin/v1.3.0",
	})
	assert.Equal(t, 2, countCalls(calls, "remote-tag-exists"))
	assert.Equal(t, 2, countCalls(calls, "local-tag-exists"))
	assertCallOrder(t, calls,
		"stage",
		"commit:feat: release command",
		"push-branch:origin/main",
		"create-tag:v1.3.0",
		"push-tag:origin/v1.3.0",
	)
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
	assert.Contains(t, err.Error(), "worktree changed after release plan")
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

	deps := &app.Dependencies{
		Logger: zap.NewNop(),
		GitRunnerFunc: func() git.Runner {
			return tagWorkflowRunner{}
		},
	}

	return &TagWorkflow{
		deps: deps,
		config: &app.TagConfig{
			Language:       "en",
			Timeout:        30,
			Remote:         "origin",
			Bump:           "auto",
			InitialVersion: "v0.1.0",
			StageAll:       true,
		},
		output:  output,
		input:   input,
		manager: manager,
	}
}
