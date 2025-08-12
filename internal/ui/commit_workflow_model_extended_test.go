package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	internalErrors "github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Extended tests for CommitWorkflowModel to improve coverage

// Test Init method in detail
func TestCommitWorkflowModel_Init_Extended(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	
	cmd := model.Init()
	assert.NotNil(t, cmd)
	
	// The Init should return a batch command containing spinner tick and collect command
	// We can't easily introspect the batch command, but we can verify it's not nil
}

// Test spinner update
func TestCommitWorkflowModel_Update_Spinner(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	
	// Create a spinner tick message directly
	// We can't execute tea.Cmd outside of the runtime
	tickMsg := spinner.TickMsg{Time: time.Now()}
	
	// Update with spinner message
	newModel, cmd := model.Update(tickMsg)
	assert.Equal(t, model, newModel) // Should return same model
	assert.NotNil(t, cmd)            // Should return spinner update command
}

// Test loading phase progression with all stages
func TestCommitWorkflowModel_LoadingPhase_AllTransitions(t *testing.T) {
	model, collector, promptBuilder, client, _ := createTestCommitWorkflowModel()
	
	// Setup comprehensive mocks
	diff := "diff --git a/file.go b/file.go\n+added line"
	commits := []string{"feat: commit 1", "fix: commit 2", "docs: commit 3"}
	branch := "feature/awesome-feature"
	files := []string{"file1.go", "file2.go", "README.md"}
	summary := &gitinfo.FileStatusSummary{
		BranchName: branch,
		Files: []gitinfo.FileStatus{
			{Path: "file1.go", IndexStatus: 'A', WorkStatus: ' '},
			{Path: "file2.go", IndexStatus: 'M', WorkStatus: ' '},
		},
	}
	changesSummary := &gitinfo.ChangesSummary{
		TotalFiles:        3,
		HasStagedChanges:  true,
		HasUnstagedChanges: false,
		ChangeTypes:       map[string]int{"modified": 2, "added": 1},
		PrimaryChangeType: "modified",
		AffectedAreas:     []string{"internal", "pkg"},
	}
	
	// Mock all collector methods
	collector.On("ComprehensiveDiff", mock.Anything).Return(diff, nil)
	collector.On("RecentCommits", mock.Anything, 10).Return(commits, nil)
	collector.On("BranchName", mock.Anything).Return(branch, nil)
	collector.On("ChangedFiles", mock.Anything).Return(files, nil)
	collector.On("FileStatusSummary", mock.Anything).Return(summary, nil)
	collector.On("AnalyzeChanges", mock.Anything).Return(changesSummary, nil)
	
	// Mock prompt builder
	promptBuilder.On("BuildSystemPrompt").Return("You are a commit message generator")
	promptBuilder.On("BuildUserPromptWithBudget", mock.Anything, mock.Anything, "feat: initial seed").
		Return("Generate a commit message for these changes", nil)
	
	// Mock LLM client
	generatedMessage := "feat: add awesome feature\n\nThis commit introduces a new feature that:\n- Improves performance\n- Adds new functionality\n- Fixes edge cases"
	client.On("GetCommitMessage", mock.Anything, mock.Anything, mock.Anything).
		Return(generatedMessage, nil)
	
	// Test each transition
	// 1. Start -> Collect
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StageCollect, model.loadingStage)
	
	// 2. Collect -> Preprocess
	newModel, cmd := model.Update(diffCollectedMsg{
		diff:    diff,
		commits: commits,
		branch:  branch,
		files:   files,
	})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, StagePreprocess, updatedModel.loadingStage)
	assert.NotNil(t, cmd)
	
	// 3. Preprocess -> Prompt
	newModel2, cmd2 := updatedModel.Update(preprocessDoneMsg{summary: summary})
	updatedModel2 := newModel2.(*CommitWorkflowModel)
	assert.Equal(t, StagePrompt, updatedModel2.loadingStage)
	assert.NotNil(t, cmd2)
	
	// 4. Prompt -> Query
	newModel3, cmd3 := updatedModel2.Update(smartPromptBuiltMsg{
		systemPrompt: "You are a commit message generator",
		userPrompt:   "Generate a commit message for these changes",
	})
	updatedModel3 := newModel3.(*CommitWorkflowModel)
	assert.Equal(t, StageQuery, updatedModel3.loadingStage)
	assert.NotNil(t, cmd3)
	
	// 5. Query -> Review
	newModel4, _ := updatedModel3.Update(queryDoneMsg{
		message: generatedMessage + "\r\n\r\nExtra carriage returns\r",
	})
	finalModel := newModel4.(*CommitWorkflowModel)
	assert.Equal(t, WorkflowPhaseReview, finalModel.phase)
	// Message should have carriage returns cleaned
	assert.Equal(t, generatedMessage + "\n\nExtra carriage returns", finalModel.message)
	assert.Equal(t, finalModel.message, finalModel.textArea.Value())
}

// Test error handling during loading phase
func TestCommitWorkflowModel_LoadingPhase_Errors(t *testing.T) {
	model, collector, promptBuilder, client, _ := createTestCommitWorkflowModel()
	
	testCases := []struct {
		name        string
		setupMocks  func()
		updateMsg   tea.Msg
		expectError bool
	}{
		{
			name: "Collector error",
			setupMocks: func() {
				collector.On("ComprehensiveDiff", mock.Anything).Return("", errors.New("git error"))
			},
			updateMsg:   errorMsg{err: errors.New("git error")},
			expectError: true,
		},
		{
			name: "Prompt builder error",
			setupMocks: func() {
				collector.On("FileStatusSummary", mock.Anything).Return(nil, errors.New("analysis error"))
			},
			updateMsg:   errorMsg{err: errors.New("analysis error")},
			expectError: true,
		},
		{
			name: "LLM client error",
			setupMocks: func() {
				promptBuilder.On("BuildSystemPrompt").Return("system")
				promptBuilder.On("BuildUserPromptWithBudget", mock.Anything, mock.Anything, mock.Anything).
					Return("", errors.New("prompt error"))
			},
			updateMsg:   errorMsg{err: errors.New("prompt error")},
			expectError: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mocks
			collector.ExpectedCalls = nil
			collector.Calls = nil
			promptBuilder.ExpectedCalls = nil
			promptBuilder.Calls = nil
			client.ExpectedCalls = nil
			client.Calls = nil
			
			tc.setupMocks()
			
			newModel, cmd := model.Update(tc.updateMsg)
			updatedModel := newModel.(*CommitWorkflowModel)
			
			if tc.expectError {
				assert.NotNil(t, updatedModel.err)
				assert.True(t, updatedModel.done)
				assert.NotNil(t, cmd) // Should return quit command
			}
		})
	}
}

// Test review phase with all keyboard inputs
func TestCommitWorkflowModel_ReviewPhase_KeyboardInput(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: test commit"
	model.editing = false
	
	// Test non-editing mode - should delegate to base updateReview
	keyTests := []struct {
		name string
		key  tea.KeyMsg
		desc string
	}{
		{"Up arrow", tea.KeyMsg{Type: tea.KeyUp}, "Navigate up"},
		{"Down arrow", tea.KeyMsg{Type: tea.KeyDown}, "Navigate down"},
		{"Left arrow", tea.KeyMsg{Type: tea.KeyLeft}, "Navigate left"},
		{"Right arrow", tea.KeyMsg{Type: tea.KeyRight}, "Navigate right"},
		{"Tab", tea.KeyMsg{Type: tea.KeyTab}, "Tab navigation"},
		{"Shift+Tab", tea.KeyMsg{Type: tea.KeyShiftTab}, "Reverse tab"},
	}
	
	for _, kt := range keyTests {
		t.Run(kt.name, func(t *testing.T) {
			// updateReview will be called, which delegates to HandleKeyboard
			newModel, _ := model.Update(kt.key)
			// Model should handle navigation
			assert.NotNil(t, newModel)
		})
	}
}

// Test PR preview preparation with edge cases
func TestCommitWorkflowModel_PreparePRPreview_EdgeCases(t *testing.T) {
	model, collector, _, _, _ := createTestCommitWorkflowModel()
	
	testCases := []struct {
		name           string
		message        string
		branchName     string
		changedFiles   []string
		expectedTitle  string
		expectedBody   string
		setupOverrides func()
	}{
		{
			name:          "Single line message",
			message:       "fix: critical bug",
			branchName:    "hotfix/critical",
			changedFiles:  []string{"bug.go"},
			expectedTitle: "fix: critical bug",
			expectedBody:  "",
		},
		{
			name:          "Multi-line with empty lines",
			message:       "feat: new feature\n\n\n\nDescription after empty lines",
			branchName:    "feature/new",
			changedFiles:  []string{"feature.go"},
			expectedTitle: "feat: new feature",
			expectedBody:  "Description after empty lines",
		},
		{
			name:          "No changed files",
			message:       "docs: update readme",
			branchName:    "docs/update",
			changedFiles:  []string{},
			expectedTitle: "docs: update readme",
			expectedBody:  "",
		},
		{
			name:          "Branch with special characters",
			message:       "feat: test",
			branchName:    "feature/test-#123-@special",
			changedFiles:  []string{"test.go"},
			expectedTitle: "feat: test",
			expectedBody:  "",
		},
		{
			name:          "Empty provider and base",
			message:       "feat: test",
			branchName:    "feature",
			changedFiles:  []string{"file.go"},
			expectedTitle: "feat: test",
			expectedBody:  "",
			setupOverrides: func() {
				model.prProvider = ""
				model.prBase = ""
			},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset model state
			model.message = tc.message
			model.prProvider = "github"
			model.prBase = "main"
			
			if tc.setupOverrides != nil {
				tc.setupOverrides()
			}
			
			// Setup mocks
			collector.On("BranchName", mock.Anything).Return(tc.branchName, nil).Once()
			collector.On("ChangedFiles", mock.Anything).Return(tc.changedFiles, nil).Once()
			
			// Execute PR preview preparation
			cmd := model.preparePRPreview()
			msg := cmd()
			
			prMsg, ok := msg.(prPreviewReadyMsg)
			assert.True(t, ok)
			assert.Equal(t, tc.expectedTitle, prMsg.data.Title)
			assert.Equal(t, tc.expectedBody, prMsg.data.Body)
			assert.Equal(t, tc.branchName, prMsg.data.Head)
			assert.Equal(t, len(tc.changedFiles) > 0, prMsg.data.HasChanges)
			assert.Len(t, prMsg.data.FileChanges, len(tc.changedFiles))
			
			// Check defaults when empty
			if model.prProvider == "" {
				assert.Equal(t, "github", prMsg.data.Provider)
			}
			if model.prBase == "" {
				assert.Equal(t, "main", prMsg.data.Base)
			}
		})
	}
}

// Test commit operation with all scenarios
func TestCommitWorkflowModel_StartCommit_Comprehensive(t *testing.T) {
	testCases := []struct {
		name          string
		stageAll      bool
		message       string
		stageError    error
		commitError   error
		expectedError string
	}{
		{
			name:     "Success with staging",
			stageAll: true,
			message:  "feat: comprehensive test",
		},
		{
			name:     "Success without staging",
			stageAll: false,
			message:  "fix: bug fix",
		},
		{
			name:          "Staging failure with wrapped error",
			stageAll:      true,
			stageError:    errors.New("permission denied"),
			expectedError: "staging failed",
		},
		{
			name:          "Commit failure",
			stageAll:      true,
			commitError:   errors.New("no changes to commit"),
			expectedError: "no changes to commit",
		},
		{
			name:     "Very long commit message",
			stageAll: true,
			message:  "feat: " + strings.Repeat("a", 1000),
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model, _, _, _, committer := createTestCommitWorkflowModel()
			model.stageAll = tc.stageAll
			model.message = tc.message
			
			// Setup mocks
			if tc.stageAll {
				committer.On("StageAll", mock.Anything).Return(tc.stageError).Once()
			}
			if tc.stageError == nil {
				committer.On("Commit", mock.Anything, tc.message).Return(tc.commitError).Once()
			}
			
			// Execute
			cmd := model.startCommit()
			msg := cmd()
			
			commitMsg, ok := msg.(commitDoneMsg)
			assert.True(t, ok)
			
			if tc.expectedError != "" {
				assert.NotNil(t, commitMsg.err)
				assert.Contains(t, commitMsg.err.Error(), tc.expectedError)
			} else {
				assert.Nil(t, commitMsg.err)
			}
		})
	}
}

// Test complete workflow with PR creation
func TestCommitWorkflowModel_CompleteWorkflow_WithPR_Extended(t *testing.T) {
	model, collector, promptBuilder, client, committer := createTestCommitWorkflowModel()
	model.createPR = true
	model.prDraft = true
	model.prProvider = "gitlab"
	model.prBase = "develop"
	
	// Setup all mocks for complete workflow
	collector.On("ComprehensiveDiff", mock.Anything).Return("diff", nil)
	collector.On("RecentCommits", mock.Anything, 10).Return([]string{"commit"}, nil)
	collector.On("BranchName", mock.Anything).Return("feature/test", nil)
	collector.On("ChangedFiles", mock.Anything).Return([]string{"file.go"}, nil)
	collector.On("FileStatusSummary", mock.Anything).Return(&gitinfo.FileStatusSummary{}, nil)
	collector.On("AnalyzeChanges", mock.Anything).Return(&gitinfo.ChangesSummary{}, nil)
	
	promptBuilder.On("BuildSystemPrompt").Return("system")
	promptBuilder.On("BuildUserPromptWithBudget", mock.Anything, mock.Anything, mock.Anything).Return("user", nil)
	
	client.On("GetCommitMessage", mock.Anything, "system", "user").
		Return("feat: test PR\n\nDetailed description for PR", nil)
	
	committer.On("StageAll", mock.Anything).Return(nil)
	committer.On("Commit", mock.Anything, mock.Anything).Return(nil)
	committer.On("Push", mock.Anything).Return(nil)
	committer.On("CreatePullRequest", mock.Anything).Return("https://gitlab.com/user/repo/-/merge_requests/1", nil)
	
	// Execute workflow stages
	
	// 1. Loading -> Review
	model.message = "feat: test PR\n\nDetailed description for PR"
	newModel, _ := model.Update(queryDoneMsg{message: model.message})
	assert.Equal(t, WorkflowPhaseReview, newModel.(*CommitWorkflowModel).phase)
	
	// 2. Accept -> PR Preview
	model.phase = WorkflowPhaseReview
	cmd := model.handleAccept()
	msg := cmd()
	prMsg, ok := msg.(prPreviewReadyMsg)
	assert.True(t, ok)
	assert.Equal(t, "feat: test PR", prMsg.data.Title)
	assert.Equal(t, "Detailed description for PR", prMsg.data.Body)
	assert.Equal(t, "gitlab", prMsg.data.Provider)
	assert.Equal(t, "develop", prMsg.data.Base)
	assert.True(t, prMsg.data.IsDraft)
	
	// 3. PR Preview -> Commit
	model.phase = WorkflowPhasePRPreview
	model.prPreviewData = prMsg.data
	newModel2, _ := model.Update(prPreviewReadyMsg{data: prMsg.data})
	assert.Equal(t, WorkflowPhasePRPreview, newModel2.(*CommitWorkflowModel).phase)
	
	// 4. Continue to commit phase
	newModel3, _ := model.Update(startCommitPhaseMsg{})
	assert.Equal(t, WorkflowPhaseCommit, newModel3.(*CommitWorkflowModel).phase)
	
	// 5. Execute commit
	cmd = model.startCommit()
	commitResult := cmd()
	assert.IsType(t, commitDoneMsg{}, commitResult)
	
	// 6. Commit success -> Push
	newModel4, _ := model.Update(commitDoneMsg{err: nil})
	assert.Equal(t, CommitStageCommitted, newModel4.(*CommitWorkflowModel).commitStage)
	
	// 7. Execute push
	model.commitStage = CommitStagePushing
	cmd = model.startPush()
	pushResult := cmd()
	assert.IsType(t, pushDoneMsg{}, pushResult)
	
	// 8. Push success -> Create PR
	newModel5, _ := model.Update(pushDoneMsg{err: nil})
	assert.Equal(t, CommitStagePushed, newModel5.(*CommitWorkflowModel).commitStage)
	
	// 9. Execute PR creation
	model.commitStage = CommitStageCreatingPR
	cmd = model.startCreatePR()
	prResult := cmd()
	prDoneMsg, ok := prResult.(createPRDoneMsg)
	assert.True(t, ok)
	assert.Nil(t, prDoneMsg.err)
	assert.Equal(t, "https://gitlab.com/user/repo/-/merge_requests/1", prDoneMsg.prURL)
	
	// 10. PR created successfully
	newModel6, _ := model.Update(prDoneMsg)
	finalModel := newModel6.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePRCreated, finalModel.commitStage)
	assert.Equal(t, "https://gitlab.com/user/repo/-/merge_requests/1", finalModel.prURL)
}

// Test renderCommitContent with error message truncation
func TestCommitWorkflowModel_RenderCommitContent_ErrorTruncation(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.message = "feat: test"
	
	// Test very long error messages
	longError := strings.Repeat("error ", 50) // 300 characters
	
	testCases := []struct {
		stage      CommitStage
		err        error
		enablePush bool
		createPR   bool
		expected   string
	}{
		{
			stage:      CommitStagePushFailed,
			err:        errors.New(longError),
			enablePush: true,
			expected:   "...",
		},
		{
			stage:    CommitStagePRFailed,
			err:      errors.New(longError),
			createPR: true,
			expected: "...",
		},
		{
			stage:      CommitStagePushFailed,
			err:        &internalErrors.CatmitError{Type: internalErrors.ErrTypeGit, Message: longError},
			enablePush: true,
			expected:   "...",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.stage.String(), func(t *testing.T) {
			model.commitStage = tc.stage
			model.err = tc.err
			model.enablePush = tc.enablePush
			model.createPR = tc.createPR
			
			content := model.renderCommitContent()
			assert.Contains(t, content, tc.expected)
			
			// Error text should be truncated to ~120 characters
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				if strings.Contains(line, "✗") {
					// Remove ANSI codes and check length
					assert.LessOrEqual(t, len(line), 150) // Allow some buffer for ANSI codes
				}
			}
		})
	}
}

// Test GetError with NoDiff error
func TestCommitWorkflowModel_GetError_NoDiff(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	
	// NoDiff error should be returned as-is
	model.err = gitinfo.ErrNoDiff
	assert.Equal(t, gitinfo.ErrNoDiff, model.GetError())
	
	// Even with push failed stage
	model.commitStage = CommitStagePushFailed
	assert.Equal(t, gitinfo.ErrNoDiff, model.GetError())
}

// Test delayed message handling
func TestCommitWorkflowModel_DelayedMessages(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	
	// Test delayed push message
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCommitted
	
	newModel, cmd := model.Update(delayedPushMsg{})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePushing, updatedModel.commitStage)
	assert.NotNil(t, cmd)
	
	// Test delayed PR creation message
	model.commitStage = CommitStagePushed
	model.createPR = true
	
	newModel2, cmd2 := model.Update(delayedCreatePRMsg{})
	updatedModel2 := newModel2.(*CommitWorkflowModel)
	assert.Equal(t, CommitStageCreatingPR, updatedModel2.commitStage)
	assert.NotNil(t, cmd2)
}

// Test PR already exists error handling
func TestCommitWorkflowModel_PRAlreadyExists_Detailed(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR
	model.createPR = true
	
	// Create different types of PR exists errors
	testCases := []struct {
		name        string
		err         error
		expectedURL string
		expectError bool
	}{
		{
			name:        "PR exists with URL",
			err:         &pr.ErrPRAlreadyExists{URL: "https://github.com/user/repo/pull/42"},
			expectedURL: "https://github.com/user/repo/pull/42",
			expectError: false,
		},
		{
			name:        "PR exists with empty URL",
			err:         &pr.ErrPRAlreadyExists{URL: ""},
			expectedURL: "",
			expectError: false,
		},
		{
			name:        "Other PR error",
			err:         errors.New("API rate limit exceeded"),
			expectedURL: "",
			expectError: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			newModel, _ := model.Update(createPRDoneMsg{err: tc.err})
			updatedModel := newModel.(*CommitWorkflowModel)
			
			if tc.expectError {
				assert.Equal(t, CommitStagePRFailed, updatedModel.commitStage)
				assert.NotNil(t, updatedModel.err)
			} else {
				assert.Equal(t, CommitStagePRCreated, updatedModel.commitStage)
				assert.Equal(t, tc.expectedURL, updatedModel.prURL)
			}
		})
	}
}

// Test updateActionsForPhase override
func TestCommitWorkflowModel_UpdateActionsForPhase_Override(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	
	// This tests the commit-specific override of updateActionsForPhase
	testCases := []struct {
		phase         WorkflowPhase
		editing       bool
		expectedCount int
		expectedKeys  []string
	}{
		{WorkflowPhaseLoading, false, 0, nil},
		{WorkflowPhaseReview, false, 4, []string{"A", "E", "R", "C"}},
		{WorkflowPhaseReview, true, 0, nil},
		{WorkflowPhasePRPreview, false, 2, []string{"Enter", "C"}},
		{WorkflowPhaseCommit, false, 0, nil},
		{WorkflowPhaseDone, false, 0, nil},
	}
	
	for _, tc := range testCases {
		t.Run(tc.phase.String()+"-"+boolToString(tc.editing), func(t *testing.T) {
			model.phase = tc.phase
			model.editing = tc.editing
			
			model.updateActionsForPhase()
			
			if tc.expectedCount == 0 {
				assert.Nil(t, model.actions)
			} else {
				assert.Len(t, model.actions, tc.expectedCount)
				for i, key := range tc.expectedKeys {
					assert.Equal(t, key, model.actions[i].Key)
				}
			}
		})
	}
}

// Test getPhaseTitle override
func TestCommitWorkflowModel_GetPhaseTitle_Override(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	
	// This tests the commit-specific override of getPhaseTitle
	testCases := []struct {
		phase    WorkflowPhase
		editing  bool
		expected string
	}{
		{WorkflowPhaseLoading, false, "Generating Message"},
		{WorkflowPhaseReview, false, "Commit Preview"},
		{WorkflowPhaseReview, true, "Edit Message"},
		{WorkflowPhasePRPreview, false, "Pull Request Preview"},
		{WorkflowPhaseCommit, false, "Commit Progress"},
		{WorkflowPhaseDone, false, "Catmit"},
		{WorkflowPhase(99), false, "Catmit"}, // Unknown phase
	}
	
	for _, tc := range testCases {
		t.Run(tc.phase.String(), func(t *testing.T) {
			model.phase = tc.phase
			model.editing = tc.editing
			
			title := model.getPhaseTitle()
			assert.Equal(t, tc.expected, title)
		})
	}
}

// Test renderPRPreviewContent
func TestCommitWorkflowModel_RenderPRPreviewContent_Extended(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhasePRPreview
	
	// Test without PR preview model
	content := model.renderPRPreviewContent()
	assert.Contains(t, content, "Preparing PR preview...")
	assert.Contains(t, content, model.spinner.View())
	
	// Test with PR preview model
	prData := PRPreviewData{
		Title:    "feat: test PR",
		Body:     "Test body",
		Provider: "github",
	}
	model.prPreview = NewEnhancedPRPreviewModel(prData, DefaultStyles(), 80, 24)
	
	content = model.renderPRPreviewContent()
	// Should return the PR preview model's view
	assert.NotContains(t, content, "Preparing PR preview...")
	assert.Equal(t, model.prPreview.View(), content)
}

// Test workflow without push and PR
func TestCommitWorkflowModel_NoPushNoPR(t *testing.T) {
	model, _, _, _, committer := createTestCommitWorkflowModel()
	model.enablePush = false
	model.createPR = false
	model.phase = WorkflowPhaseCommit
	model.message = "fix: simple fix"
	
	committer.On("StageAll", mock.Anything).Return(nil)
	committer.On("Commit", mock.Anything, "fix: simple fix").Return(nil)
	
	// Commit success should go directly to done when no push and no PR
	newModel, cmd := model.Update(commitDoneMsg{err: nil})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, CommitStageDone, updatedModel.commitStage)
	assert.NotNil(t, cmd) // Should have tick command for final timeout
	assert.NotZero(t, updatedModel.finalStartTime)
}

// Test workflow with PR but without push
func TestCommitWorkflowModel_PRWithoutPush(t *testing.T) {
	model, _, _, _, committer := createTestCommitWorkflowModel()
	model.enablePush = false
	model.createPR = true
	model.phase = WorkflowPhaseCommit
	
	committer.On("CreatePullRequest", mock.Anything).Return("https://github.com/user/repo/pull/1", nil)
	
	// After commit, should go to PR creation
	newModel, cmd := model.Update(commitDoneMsg{err: nil})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, CommitStageCommitted, updatedModel.commitStage)
	assert.NotNil(t, cmd)
	
	// Handle delayed PR creation
	newModel2, cmd2 := updatedModel.Update(delayedCreatePRMsg{})
	updatedModel2 := newModel2.(*CommitWorkflowModel)
	assert.Equal(t, CommitStageCreatingPR, updatedModel2.commitStage)
	assert.NotNil(t, cmd2)
}