package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for PR workflow testing

// mockPRCollector implements collectorInterface
type mockPRCollector struct {
	mock.Mock
}

func (m *mockPRCollector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	args := m.Called(ctx, n)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockPRCollector) BranchName(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockPRCollector) ChangedFiles(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockPRCollector) FileStatusSummary(ctx context.Context) (*gitinfo.FileStatusSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gitinfo.FileStatusSummary), args.Error(1)
}

func (m *mockPRCollector) ComprehensiveDiff(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockPRCollector) AnalyzeChanges(ctx context.Context) (*gitinfo.ChangesSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gitinfo.ChangesSummary), args.Error(1)
}

// mockPRPromptBuilder implements promptInterface
type mockPRPromptBuilder struct {
	mock.Mock
}

func (m *mockPRPromptBuilder) Build(seed, diff string, commits []string, branch string, files []string) string {
	args := m.Called(seed, diff, commits, branch, files)
	return args.String(0)
}

func (m *mockPRPromptBuilder) BuildSystemPrompt() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockPRPromptBuilder) BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string {
	args := m.Called(seed, diff, commits, branch, files)
	return args.String(0)
}

func (m *mockPRPromptBuilder) BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error) {
	args := m.Called(ctx, collector, seed)
	return args.String(0), args.Error(1)
}

func (m *mockPRPromptBuilder) BuildPRSystemPrompt() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockPRPromptBuilder) BuildPRUserPrompt(commits []string) string {
	args := m.Called(commits)
	return args.String(0)
}

// mockPRClient implements clientInterface
type mockPRClient struct {
	mock.Mock
}

func (m *mockPRClient) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := m.Called(ctx, systemPrompt, userPrompt)
	return args.String(0), args.Error(1)
}

// mockPRCommitter implements commitInterface
type mockPRCommitter struct {
	mock.Mock
}

func (m *mockPRCommitter) Commit(ctx context.Context, message string) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func (m *mockPRCommitter) Push(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPRCommitter) StageAll(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPRCommitter) HasStagedChanges(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *mockPRCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockPRCommitter) NeedsPush(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

// Helper function to create a PRWorkflowModel with mocks
func createTestPRWorkflowModel() (*PRWorkflowModel, *mockPRCollector, *mockPRPromptBuilder, *mockPRClient, *mockPRCommitter) {
	ctx := context.Background()
	collector := &mockPRCollector{}
	promptBuilder := &mockPRPromptBuilder{}
	client := &mockPRClient{}
	committer := &mockPRCommitter{}
	
	prConfig := PRConfig{
		CreatePR:    true,
		Remote:      "origin",
		Base:        "main",
		Draft:       true,
		Provider:    "github",
		UseTemplate: false,
	}
	
	model := NewPRWorkflowModel(
		ctx,
		collector,
		promptBuilder,
		client,
		committer,
		"en",
		5*time.Second,
		prConfig,
	)
	
	return model, collector, promptBuilder, client, committer
}

// Test PRWorkflowModel creation
func TestNewPRWorkflowModel(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	assert.NotNil(t, model)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StageCollect, model.loadingStage)
	assert.Equal(t, "en", model.lang)
	assert.Equal(t, "origin", model.prRemote)
	assert.Equal(t, "main", model.prBase)
	assert.True(t, model.prDraft)
	assert.Equal(t, "github", model.prProvider)
	assert.False(t, model.useTemplate)
	assert.Equal(t, 80, model.width)
	assert.Equal(t, 24, model.height)
	
	// Check that textarea is configured for PR description
	assert.Equal(t, "Edit PR description...", model.textArea.Placeholder)
	assert.Equal(t, 2000, model.textArea.CharLimit)
}

// Test Init method
func TestPRWorkflowModel_Init(t *testing.T) {
	model, collector, _, _, _ := createTestPRWorkflowModel()
	
	// Setup mocks
	commits := []string{"feat: commit 1", "fix: commit 2"}
	collector.On("RecentCommits", mock.Anything, 20).Return(commits, nil)
	collector.On("BranchName", mock.Anything).Return("feature-branch", nil)
	
	// Test Init returns both spinner tick and collect command
	cmd := model.Init()
	assert.NotNil(t, cmd)
}

// Test the NewPRWorkflowModel factory function returns the correct type
func TestNewPRWorkflowModel_Type(t *testing.T) {
	ctx := context.Background()
	collector := &mockPRCollector{}
	promptBuilder := &mockPRPromptBuilder{}
	client := &mockPRClient{}
	committer := &mockPRCommitter{}
	
	prConfig := PRConfig{
		CreatePR:    true,
		Remote:      "origin",
		Base:        "main",
		Draft:       false,
		Provider:    "github",
		UseTemplate: false,
	}
	
	model := NewPRWorkflowModel(
		ctx,
		collector,
		promptBuilder,
		client,
		committer,
		"en",
		5*time.Second,
		prConfig,
	)
	
	// Verify it's the correct type and not nil
	assert.IsType(t, &PRWorkflowModel{}, model)
	assert.NotNil(t, model)
}

// Test window size handling
func TestPRWorkflowModel_Update_WindowSize(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	// Test window size update
	newModel, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updatedModel := newModel.(*PRWorkflowModel)

	assert.Equal(t, 120, updatedModel.width)
	assert.Equal(t, 30, updatedModel.height)
}

// Test Ctrl+C cancellation
func TestPRWorkflowModel_Update_CtrlC(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	// Test Ctrl+C cancellation
	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updatedModel := newModel.(*PRWorkflowModel)

	assert.True(t, updatedModel.done)
	assert.Equal(t, context.Canceled, updatedModel.err)
	assert.NotNil(t, cmd)
}

// Test PR data collection
func TestPRWorkflowModel_CollectPRData(t *testing.T) {
	model, collector, _, _, _ := createTestPRWorkflowModel()
	
	// Setup mocks
	commits := []string{
		"feat: add new feature",
		"fix: fix bug in component",
		"docs: update documentation",
	}
	collector.On("RecentCommits", mock.Anything, 20).Return(commits, nil)
	collector.On("BranchName", mock.Anything).Return("feature-branch", nil)

	// Execute collection command
	cmd := model.collectPRDataCmd()
	msg := cmd()

	// Verify the message
	diffMsg, ok := msg.(diffCollectedMsg)
	assert.True(t, ok)
	assert.Equal(t, "", diffMsg.diff) // PR-only workflow has no diff
	assert.Equal(t, commits, diffMsg.commits)
	assert.Equal(t, "feature-branch", diffMsg.branch)
	assert.Empty(t, diffMsg.files) // No changed files in PR-only workflow
}

// Test PR data collection error
func TestPRWorkflowModel_CollectPRData_Error(t *testing.T) {
	model, collector, _, _, _ := createTestPRWorkflowModel()
	
	// Setup mock with error
	collectErr := errors.New("failed to collect commits")
	collector.On("RecentCommits", mock.Anything, 20).Return(([]string)(nil), collectErr)

	// Execute collection command
	cmd := model.collectPRDataCmd()
	msg := cmd()

	// Verify error message
	errMsg, ok := msg.(errorMsg)
	assert.True(t, ok)
	assert.Equal(t, collectErr, errMsg.err)
}

// Test PR prompt building
func TestPRWorkflowModel_BuildPRPrompt(t *testing.T) {
	model, collector, promptBuilder, _, _ := createTestPRWorkflowModel()
	
	// Setup mocks
	commits := []string{"feat: feature 1", "fix: bug fix"}
	collector.On("RecentCommits", mock.Anything, 20).Return(commits, nil)
	promptBuilder.On("BuildPRSystemPrompt").Return("PR system prompt")
	promptBuilder.On("BuildPRUserPrompt", commits).Return("PR user prompt")

	// Execute prompt building
	cmd := model.buildPRPromptCmd()
	msg := cmd()

	// Verify the message
	promptMsg, ok := msg.(smartPromptBuiltMsg)
	assert.True(t, ok)
	assert.Equal(t, "PR system prompt", promptMsg.systemPrompt)
	assert.Equal(t, "PR user prompt", promptMsg.userPrompt)
}

// Test loading phase progression
func TestPRWorkflowModel_LoadingPhase_Progression(t *testing.T) {
	model, collector, promptBuilder, client, _ := createTestPRWorkflowModel()
	
	// Setup mocks
	commits := []string{"feat: new feature", "fix: bug fix"}
	collector.On("RecentCommits", mock.Anything, 20).Return(commits, nil)
	collector.On("BranchName", mock.Anything).Return("feature-branch", nil)
	promptBuilder.On("BuildPRSystemPrompt").Return("PR system prompt")
	promptBuilder.On("BuildPRUserPrompt", commits).Return("PR user prompt")
	client.On("GetCommitMessage", mock.Anything, "PR system prompt", "PR user prompt").
		Return("feat: comprehensive PR title\n\nDetailed PR description with multiple lines\nand explanations", nil)

	// Test Init
	cmd := model.Init()
	assert.NotNil(t, cmd)

	// Test diff collection -> directly to prompt building for PR workflow
	newModel, _ := model.Update(diffCollectedMsg{
		diff:    "",
		commits: commits,
		branch:  "feature-branch",
		files:   []string{},
	})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, StagePrompt, updatedModel.loadingStage)

	// Test prompt building
	newModel2, _ := updatedModel.Update(smartPromptBuiltMsg{
		systemPrompt: "PR system prompt",
		userPrompt:   "PR user prompt",
	})
	updatedModel2 := newModel2.(*PRWorkflowModel)
	assert.Equal(t, StageQuery, updatedModel2.loadingStage)

	// Test query done
	newModel3, _ := updatedModel2.Update(queryDoneMsg{
		message: "feat: comprehensive PR title\n\nDetailed PR description",
	})
	finalModel := newModel3.(*PRWorkflowModel)
	assert.Equal(t, WorkflowPhaseReview, finalModel.phase)
	assert.Equal(t, "feat: comprehensive PR title\n\nDetailed PR description", finalModel.message)
}

// Test review phase actions
func TestPRWorkflowModel_ReviewPhase_Accept(t *testing.T) {
	model, collector, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: PR title\n\nPR body description"

	// Setup mocks for PR preview
	collector.On("BranchName", mock.Anything).Return("feature-branch", nil)

	// Simulate accept action
	cmd := model.handleAccept()
	assert.NotNil(t, cmd)

	// Execute PR preview preparation
	msg := cmd()
	prMsg, ok := msg.(prPreviewReadyMsg)
	assert.True(t, ok)
	assert.Equal(t, "feat: PR title", prMsg.data.Title)
	assert.Equal(t, "PR body description", prMsg.data.Body)
	assert.Equal(t, "main", prMsg.data.Base)
	assert.Equal(t, "feature-branch", prMsg.data.Head)
	assert.Equal(t, "origin", prMsg.data.Remote)
	assert.Equal(t, "github", prMsg.data.Provider)
	assert.True(t, prMsg.data.IsDraft)
	assert.True(t, prMsg.data.HasChanges)
}

func TestPRWorkflowModel_ReviewPhase_Edit(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: original PR title\n\nOriginal body"

	// Simulate edit action
	cmd := model.handleEdit()
	assert.NotNil(t, cmd)
	assert.True(t, model.editing)
	assert.True(t, model.textArea.Focused())

	// Simulate saving edit
	model.textArea.SetValue("feat: edited PR title\n\nEdited body with more details")
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.False(t, updatedModel.editing)
	assert.Equal(t, "feat: edited PR title\n\nEdited body with more details", updatedModel.message)

	// Test cancel edit
	model.editing = true
	newModel2, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updatedModel2 := newModel2.(*PRWorkflowModel)
	assert.False(t, updatedModel2.editing)
}

func TestPRWorkflowModel_ReviewPhase_Regenerate(t *testing.T) {
	model, collector, promptBuilder, client, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: original PR"

	// Setup mocks for regeneration
	commits := []string{"feat: commit 1"}
	collector.On("RecentCommits", mock.Anything, 20).Return(commits, nil)
	promptBuilder.On("BuildPRSystemPrompt").Return("PR system prompt")
	promptBuilder.On("BuildPRUserPrompt", commits).Return("PR user prompt")
	client.On("GetCommitMessage", mock.Anything, "PR system prompt", "PR user prompt").
		Return("feat: regenerated PR title\n\nNew description", nil)

	// Simulate regenerate action
	cmd := model.handleRegenerate()
	assert.NotNil(t, cmd)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StagePrompt, model.loadingStage)
}

// Test PR preview phase
func TestPRWorkflowModel_PRPreviewPhase(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhasePRPreview
	model.prPreviewData = PRPreviewData{
		Title:      "feat: new feature",
		Body:       "Detailed description",
		Base:       "main",
		Head:       "feature-branch",
		Remote:     "origin",
		Provider:   "github",
		IsDraft:    true,
		HasChanges: true,
	}
	model.prPreview = NewPRPreviewModel(model.prPreviewData, DefaultStyles(), CalculateContentWidth(model.width))

	// Test toggle details
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	assert.Equal(t, model, newModel)

	// Test continue to commit phase
	newModel2, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, model, newModel2)
	assert.NotNil(t, cmd)

	// Test cancel
	newModel3, cmd3 := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updatedModel := newModel3.(*PRWorkflowModel)
	assert.Equal(t, DecisionCancel, updatedModel.reviewDecision)
	assert.True(t, updatedModel.done)
	assert.NotNil(t, cmd3)
}

// Test commit phase transition with push needed
func TestPRWorkflowModel_CommitPhase_NeedsPush(t *testing.T) {
	model, _, _, _, committer := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: PR title\n\nPR body"

	// Setup mocks
	committer.On("NeedsPush", mock.Anything).Return(true, nil)

	// Transition to commit phase
	newModel, _ := model.Update(startCommitPhaseMsg{})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, WorkflowPhaseCommit, updatedModel.phase)
	assert.Equal(t, CommitStagePushing, updatedModel.commitStage)
}

// Test commit phase transition without push needed
func TestPRWorkflowModel_CommitPhase_NoPushNeeded(t *testing.T) {
	model, _, _, _, committer := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: PR title\n\nPR body"

	// Setup mocks
	committer.On("NeedsPush", mock.Anything).Return(false, nil)

	// Transition to commit phase
	newModel, _ := model.Update(startCommitPhaseMsg{})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, WorkflowPhaseCommit, updatedModel.phase)
	assert.Equal(t, CommitStageCreatingPR, updatedModel.commitStage)
}

// Test push success and PR creation
func TestPRWorkflowModel_PushSuccess_CreatePR(t *testing.T) {
	model, _, _, _, committer := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePushing

	// Setup mock for PR creation
	committer.On("CreatePullRequest", mock.Anything).Return("https://github.com/user/repo/pull/42", nil)

	// Test successful push
	newModel, _ := model.Update(pushDoneMsg{err: nil})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, CommitStagePushed, updatedModel.commitStage)

	// Simulate delayed PR creation
	newModel2, _ := updatedModel.Update(delayedCreatePRMsg{})
	updatedModel2 := newModel2.(*PRWorkflowModel)
	assert.Equal(t, CommitStageCreatingPR, updatedModel2.commitStage)

	// Execute PR creation
	cmd := updatedModel2.startCreatePR()
	msg := cmd()
	prMsg, ok := msg.(createPRDoneMsg)
	assert.True(t, ok)
	assert.Nil(t, prMsg.err)
	assert.Equal(t, "https://github.com/user/repo/pull/42", prMsg.prURL)

	// Update with PR creation success
	newModel3, _ := updatedModel2.Update(prMsg)
	finalModel := newModel3.(*PRWorkflowModel)
	assert.Equal(t, CommitStagePRCreated, finalModel.commitStage)
	assert.Equal(t, "https://github.com/user/repo/pull/42", finalModel.prURL)
}

// Test push failure
func TestPRWorkflowModel_PushFailure(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePushing

	pushErr := errors.New("push failed: authentication required")

	// Test push failure
	newModel, _ := model.Update(pushDoneMsg{err: pushErr})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, CommitStagePushFailed, updatedModel.commitStage)
	assert.Equal(t, pushErr, updatedModel.err)
}

// Test PR creation with existing PR
func TestPRWorkflowModel_CreatePR_AlreadyExists(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR

	// Simulate PR already exists error
	prExistsErr := &pr.ErrPRAlreadyExists{
		URL: "https://github.com/user/repo/pull/41",
	}

	// Update model with PR exists error
	newModel, _ := model.Update(createPRDoneMsg{err: prExistsErr})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, CommitStagePRCreated, updatedModel.commitStage)
	assert.Equal(t, "https://github.com/user/repo/pull/41", updatedModel.prURL)
}

// Test PR creation failure
func TestPRWorkflowModel_CreatePR_Failure(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR

	prErr := errors.New("failed to create PR: insufficient permissions")

	// Update model with error
	newModel, _ := model.Update(createPRDoneMsg{err: prErr})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, CommitStagePRFailed, updatedModel.commitStage)
	assert.Equal(t, prErr, updatedModel.err)
}

// Test complete PR workflow - simplified version
func TestPRWorkflowModel_CompleteWorkflow(t *testing.T) {
	model, collector, _, _, committer := createTestPRWorkflowModel()
	
	// Setup minimal mocks needed for key operations
	collector.On("BranchName", mock.Anything).Return("feature/awesome", nil).Maybe()
	committer.On("NeedsPush", mock.Anything).Return(true, nil)
	committer.On("Push", mock.Anything).Return(nil)
	committer.On("CreatePullRequest", mock.Anything).Return("https://github.com/user/repo/pull/100", nil)

	// Test phase transitions manually
	
	// 1. Loading -> Review phase transition
	model.phase = WorkflowPhaseLoading
	model.message = "feat: awesome PR\n\nDetailed description"
	newModel, _ := model.Update(queryDoneMsg{message: model.message})
	assert.Equal(t, WorkflowPhaseReview, newModel.(*PRWorkflowModel).phase)
	
	// 2. Review -> PR Preview phase transition
	model.phase = WorkflowPhaseReview
	prData := PRPreviewData{
		Title:      "feat: awesome PR",
		Body:       "Detailed description",
		Base:       "main",
		Head:       "feature/awesome",
		Provider:   "github",
		HasChanges: true,
	}
	newModel2, _ := model.Update(prPreviewReadyMsg{data: prData})
	assert.Equal(t, WorkflowPhasePRPreview, newModel2.(*PRWorkflowModel).phase)
	
	// 3. PR Preview -> Commit phase transition
	model.phase = WorkflowPhasePRPreview
	newModel3, _ := model.Update(startCommitPhaseMsg{})
	assert.Equal(t, WorkflowPhaseCommit, newModel3.(*PRWorkflowModel).phase)
	assert.Equal(t, CommitStagePushing, newModel3.(*PRWorkflowModel).commitStage)
	
	// 4. Push success -> PR creation
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePushing
	newModel4, _ := model.Update(pushDoneMsg{err: nil})
	assert.Equal(t, CommitStagePushed, newModel4.(*PRWorkflowModel).commitStage)
	
	// 5. PR creation success
	model.commitStage = CommitStageCreatingPR
	newModel5, _ := model.Update(createPRDoneMsg{err: nil, prURL: "https://github.com/user/repo/pull/100"})
	finalModel := newModel5.(*PRWorkflowModel)
	assert.Equal(t, CommitStagePRCreated, finalModel.commitStage)
	assert.Equal(t, "https://github.com/user/repo/pull/100", finalModel.prURL)
}

// Test View rendering
func TestPRWorkflowModel_View(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.message = "feat: awesome PR\n\nDetailed description"
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR

	// Test view rendering
	view := model.View()

	// Check that the view contains expected elements
	assert.Contains(t, view, "Creating PR (en)")
	assert.Contains(t, view, "Title:")
	assert.Contains(t, view, "feat: awesome PR")
	assert.Contains(t, view, "Creating pull request...")
}

// Test phase title rendering
func TestPRWorkflowModel_PhaseTitle(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	testCases := []struct {
		phase    WorkflowPhase
		editing  bool
		expected string
	}{
		{WorkflowPhaseLoading, false, "Analyzing Commits"},
		{WorkflowPhaseReview, false, "PR Preview"},
		{WorkflowPhaseReview, true, "Edit PR Description"},
		{WorkflowPhasePRPreview, false, "Pull Request Preview"},
		{WorkflowPhaseCommit, false, "Creating PR"},
	}

	for _, tc := range testCases {
		model.phase = tc.phase
		model.editing = tc.editing
		title := model.getPhaseTitle()
		assert.Equal(t, tc.expected, title, "Phase %v, editing %v", tc.phase, tc.editing)
	}
}

// Test error handling
func TestPRWorkflowModel_ErrorHandling(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	// Test error during collection
	collectErr := errors.New("failed to collect commits")
	newModel, cmd := model.Update(errorMsg{err: collectErr})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, collectErr, updatedModel.err)
	assert.True(t, updatedModel.done)
	assert.NotNil(t, cmd)
}

// Test final timeout
func TestPRWorkflowModel_FinalTimeout(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePRCreated

	// Test final timeout
	newModel, cmd := model.Update(finalTimeoutMsg{})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.True(t, updatedModel.done)
	assert.NotNil(t, cmd)
}

// Test template support
func TestPRWorkflowModel_TemplateSupport(t *testing.T) {
	ctx := context.Background()
	collector := &mockPRCollector{}
	promptBuilder := &mockPRPromptBuilder{}
	client := &mockPRClient{}
	committer := &mockPRCommitter{}
	
	prConfig := PRConfig{
		CreatePR:    true,
		Remote:      "origin",
		Base:        "develop",
		Draft:       false,
		Provider:    "gitlab",
		UseTemplate: true,
	}
	
	model := NewPRWorkflowModel(
		ctx,
		collector,
		promptBuilder,
		client,
		committer,
		"zh",
		10*time.Second,
		prConfig,
	)
	
	assert.True(t, model.useTemplate)
	assert.Equal(t, "gitlab", model.prProvider)
	assert.Equal(t, "develop", model.prBase)
	assert.False(t, model.prDraft)
}

// Test different provider configurations
func TestPRWorkflowModel_DifferentProviders(t *testing.T) {
	providers := []struct {
		provider string
		remote   string
		base     string
	}{
		{"github", "origin", "main"},
		{"gitlab", "upstream", "develop"},
		{"gitea", "fork", "master"},
	}

	for _, p := range providers {
		ctx := context.Background()
		prConfig := PRConfig{
			CreatePR:    true,
			Remote:      p.remote,
			Base:        p.base,
			Draft:       false,
			Provider:    p.provider,
			UseTemplate: false,
		}
		
		model := NewPRWorkflowModel(
			ctx,
			&mockPRCollector{},
			&mockPRPromptBuilder{},
			&mockPRClient{},
			&mockPRCommitter{},
			"en",
			5*time.Second,
			prConfig,
		)
		
		assert.Equal(t, p.provider, model.prProvider)
		assert.Equal(t, p.remote, model.prRemote)
		assert.Equal(t, p.base, model.prBase)
	}
}

// Test IsDone and GetError methods
func TestPRWorkflowModel_IsDone(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	// Test initial state
	done, decision, message, err := model.IsDone()
	assert.False(t, done)
	assert.Equal(t, DecisionNone, decision)
	assert.Equal(t, "", message)
	assert.Nil(t, err)

	// Test after cancellation
	model.done = true
	model.reviewDecision = DecisionCancel
	model.message = "feat: test PR"
	model.err = errors.New("test error")

	done, decision, message, err = model.IsDone()
	assert.True(t, done)
	assert.Equal(t, DecisionCancel, decision)
	assert.Equal(t, "feat: test PR", message)
	assert.Equal(t, "test error", err.Error())
}

func TestPRWorkflowModel_GetError(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()

	// Test no error
	assert.Nil(t, model.GetError())

	// Test with context.Canceled
	model.err = context.Canceled
	assert.Nil(t, model.GetError())

	// Test with push failed
	model.err = errors.New("push failed")
	model.commitStage = CommitStagePushFailed
	assert.Nil(t, model.GetError())

	// Test with other error
	model.err = errors.New("other error")
	model.commitStage = CommitStagePRFailed
	assert.Equal(t, "other error", model.GetError().Error())
}

// Test renderContent method
func TestPRWorkflowModel_RenderContent(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	
	// Test loading phase
	model.phase = WorkflowPhaseLoading
	model.loadingStage = StageQuery
	content := model.renderContent()
	assert.Contains(t, content, "Generating message")
	
	// Test review phase
	model.phase = WorkflowPhaseReview
	model.message = "feat: test PR\n\nDescription"
	content = model.renderContent()
	assert.Contains(t, content, "feat:")
	assert.Contains(t, content, "test PR")
	
	// Test editing mode
	model.editing = true
	content = model.renderContent()
	assert.Contains(t, content, "Edit Message:")
	assert.Contains(t, content, "[Ctrl+S] Save")
	
	// Test PR preview phase
	model.phase = WorkflowPhasePRPreview
	model.prPreview = nil
	content = model.renderContent()
	assert.Contains(t, content, "Preparing PR preview...")
	
	// Test commit phase
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePRCreated
	model.prURL = "https://github.com/user/repo/pull/123"
	content = model.renderContent()
	assert.Contains(t, content, "Pull request created successfully")
	assert.Contains(t, content, model.prURL)
}

// Test updateActionsForPhase
func TestPRWorkflowModel_UpdateActionsForPhase(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	
	// Test loading phase
	model.phase = WorkflowPhaseLoading
	model.updateActionsForPhase()
	assert.Nil(t, model.actions)
	
	// Test review phase
	model.phase = WorkflowPhaseReview
	model.editing = false
	model.updateActionsForPhase()
	assert.Equal(t, 4, len(model.actions))
	assert.Equal(t, "A", model.actions[0].Key)
	assert.Equal(t, "E", model.actions[1].Key)
	assert.Equal(t, "R", model.actions[2].Key)
	assert.Equal(t, "C", model.actions[3].Key)
	
	// Test editing mode
	model.editing = true
	model.updateActionsForPhase()
	assert.Nil(t, model.actions)
	
	// Test PR preview phase
	model.phase = WorkflowPhasePRPreview
	model.updateActionsForPhase()
	assert.Equal(t, 2, len(model.actions))
	assert.Equal(t, "A", model.actions[0].Key)
	assert.Equal(t, "ccept", model.actions[0].Label)
	assert.Equal(t, "C", model.actions[1].Key)
	assert.Equal(t, "ancel", model.actions[1].Label)
	
	// Test commit phase
	model.phase = WorkflowPhaseCommit
	model.updateActionsForPhase()
	assert.Nil(t, model.actions)
}

// Test startPush edge cases
func TestPRWorkflowModel_StartPush(t *testing.T) {
	model, _, _, _, committer := createTestPRWorkflowModel()
	
	// Test push error
	pushErr := errors.New("network error")
	committer.On("Push", mock.Anything).Return(pushErr)
	
	cmd := model.startPush()
	msg := cmd()
	
	pushMsg, ok := msg.(pushDoneMsg)
	assert.True(t, ok)
	assert.Equal(t, pushErr, pushMsg.err)
}

// Test renderPRPreviewContent with PR preview model
func TestPRWorkflowModel_RenderPRPreviewContent_WithModel(t *testing.T) {
	model, _, _, _, _ := createTestPRWorkflowModel()
	
	// Create PR preview model
	prData := PRPreviewData{
		Title:    "feat: test",
		Body:     "Test body",
		Provider: "github",
	}
	model.prPreview = NewPRPreviewModel(prData, DefaultStyles(), CalculateContentWidth(model.width))
	
	content := model.renderPRPreviewContent()
	// The PR preview model renders its own content
	assert.NotContains(t, content, "Preparing PR preview...")
}

// Test NeedsPush error handling
func TestPRWorkflowModel_CommitPhase_NeedsPushError(t *testing.T) {
	model, _, _, _, committer := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: PR title\n\nPR body"

	// Setup mock with error - should fall back to no push needed
	committer.On("NeedsPush", mock.Anything).Return(false, errors.New("git error"))

	// Transition to commit phase
	newModel, _ := model.Update(startCommitPhaseMsg{})
	updatedModel := newModel.(*PRWorkflowModel)
	assert.Equal(t, WorkflowPhaseCommit, updatedModel.phase)
	// Should go directly to PR creation on error
	assert.Equal(t, CommitStageCreatingPR, updatedModel.commitStage)
}