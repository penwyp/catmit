package workflow

import (
	"bytes"
	"context"
	"strings"
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

func (m *fakeTagManager) CurrentBranch(context.Context) (string, error) {
	m.calls = append(m.calls, "current-branch")
	return m.branch, nil
}

func (m *fakeTagManager) HeadSHA(context.Context, bool) (string, error) {
	m.calls = append(m.calls, "head")
	return m.head, nil
}

func (m *fakeTagManager) WorktreeStatus(context.Context) (git.WorktreeStatus, error) {
	m.calls = append(m.calls, "status")
	return m.worktreeStatus, nil
}

func (m *fakeTagManager) StageAll(context.Context) error {
	m.calls = append(m.calls, "stage")
	return nil
}

func (m *fakeTagManager) Commit(_ context.Context, message string) error {
	m.calls = append(m.calls, "commit:"+message)
	return nil
}

func (m *fakeTagManager) BranchStatus(context.Context, string, string) (git.BranchStatus, error) {
	m.calls = append(m.calls, "branch-status")
	return m.branchStatus, nil
}

func (m *fakeTagManager) PushBranch(_ context.Context, remote, branch string) error {
	m.calls = append(m.calls, "push-branch:"+remote+"/"+branch)
	return nil
}

func (m *fakeTagManager) ListRemoteTags(context.Context, string) ([]string, error) {
	m.calls = append(m.calls, "list-tags")
	return m.remoteTags, nil
}

func (m *fakeTagManager) LocalTagExists(context.Context, string) (bool, error) {
	m.calls = append(m.calls, "local-tag-exists")
	return m.localTagExists, nil
}

func (m *fakeTagManager) RemoteTagExists(context.Context, string, string) (bool, error) {
	m.calls = append(m.calls, "remote-tag-exists")
	if len(m.remoteTagChecks) > 0 {
		result := m.remoteTagChecks[0]
		m.remoteTagChecks = m.remoteTagChecks[1:]
		return result, nil
	}
	return m.remoteTagExists, nil
}

func (m *fakeTagManager) CreateAnnotatedTag(_ context.Context, tagName, _, _ string) error {
	m.calls = append(m.calls, "create-tag:"+tagName)
	return nil
}

func (m *fakeTagManager) PushTag(_ context.Context, remote, tagName string) error {
	m.calls = append(m.calls, "push-tag:"+remote+"/"+tagName)
	return nil
}

func (m *fakeTagManager) CommitMessagesSince(context.Context, string, string) ([]string, error) {
	m.calls = append(m.calls, "commit-messages")
	return m.commitMessages, nil
}

func (m *fakeTagManager) ResolveRevision(context.Context, string) (string, error) {
	m.calls = append(m.calls, "resolve")
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
	assert.Equal(t, []string{
		"current-branch",
		"head",
		"status",
		"remote-tag-exists",
		"local-tag-exists",
		"stage",
		"commit:feat: release command",
		"push-branch:origin/main",
		"remote-tag-exists",
		"local-tag-exists",
		"create-tag:v1.3.0",
		"push-tag:origin/v1.3.0",
	}, manager.calls)
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
	assert.NotContains(t, strings.Join(manager.calls, ","), "create-tag")
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
	assert.NotContains(t, strings.Join(manager.calls, ","), "create-tag")
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
	assert.NotContains(t, strings.Join(manager.calls, ","), "create-tag")
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
	assert.NotContains(t, strings.Join(manager.calls, ","), "create-tag")
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
	assert.NotContains(t, strings.Join(manager.calls, ","), "create-tag")
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
