package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for squash workflow testing

// mockSquashClient implements squash.ClientInterface
type mockSquashClient struct {
	mock.Mock
}

func (m *mockSquashClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
}

// Test creation of SquashWorkflowModel
func TestNewSquashWorkflowModel(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature", "fix: fix bug"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	assert.NotNil(t, model)
	assert.NotNil(t, model.BaseWorkflowModel)
	assert.Equal(t, "Squashing Commit Messages", model.title)
	assert.Equal(t, messages, model.messages)
	assert.Equal(t, squashInstance, model.squash)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.False(t, model.copySuccess)
	assert.False(t, model.accepted)
}

// Test Init command
func TestSquashWorkflowModel_Init(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)
	
	// Setup mock to return immediately
	mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
		Return("feat: consolidated feature", nil)

	cmd := model.Init()
	assert.NotNil(t, cmd)
}

// Test phase titles
func TestSquashWorkflowModel_GetPhaseTitle(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	tests := []struct {
		name     string
		phase    WorkflowPhase
		editing  bool
		hasError bool
		expected string
	}{
		{
			name:     "loading phase",
			phase:    WorkflowPhaseLoading,
			expected: "Squashing Commit Messages",
		},
		{
			name:     "review phase",
			phase:    WorkflowPhaseReview,
			expected: "Generated commit message:",
		},
		{
			name:     "review phase with error",
			phase:    WorkflowPhaseReview,
			hasError: true,
			expected: "Error",
		},
		{
			name:     "review phase editing",
			phase:    WorkflowPhaseReview,
			editing:  true,
			expected: "Edit Message",
		},
		{
			name:     "unknown phase",
			phase:    WorkflowPhaseDone,
			expected: "Squash Draft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model.phase = tt.phase
			model.editing = tt.editing
			if tt.hasError {
				model.err = errors.New("test error")
			} else {
				model.err = nil
			}

			title := model.getPhaseTitle()
			assert.Equal(t, tt.expected, title)
		})
	}
}

// Test successful message generation
func TestSquashWorkflowModel_Update_SuccessfulGeneration(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature", "fix: fix bug"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)
	
	// Test successful generation message
	generatedMsg := squashGeneratedMsg{
		result: "feat: consolidated changes",
		err:    nil,
	}

	updatedModel, cmd := model.Update(generatedMsg)
	squashModel := updatedModel.(*SquashWorkflowModel)

	assert.Nil(t, cmd)
	assert.Equal(t, WorkflowPhaseReview, squashModel.phase)
	assert.Equal(t, "feat: consolidated changes", squashModel.message)
	assert.Equal(t, "feat: consolidated changes", squashModel.textArea.Value())
	// Note: copySuccess might be true or false depending on clipboard availability
	// We just verify it doesn't panic
	assert.Nil(t, squashModel.err)
}

// Test failed message generation
func TestSquashWorkflowModel_Update_FailedGeneration(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	// Test failed generation message
	testErr := errors.New("API error")
	generatedMsg := squashGeneratedMsg{
		result: "",
		err:    testErr,
	}

	updatedModel, cmd := model.Update(generatedMsg)
	squashModel := updatedModel.(*SquashWorkflowModel)

	assert.Nil(t, cmd)
	assert.Equal(t, WorkflowPhaseReview, squashModel.phase)
	assert.Equal(t, testErr, squashModel.err)
	assert.False(t, squashModel.copySuccess)
}

// Test keyboard handling in review phase
func TestSquashWorkflowModel_Update_KeyboardHandling(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	// Mock the GenerateCommitMessage for regeneration test
	mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
		Return("feat: regenerated message", nil)

	t.Run("accept key", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		model.phase = WorkflowPhaseReview
		model.message = "feat: test message"
		model.updateActionsForPhase()

		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
		updatedModel, cmd := model.Update(key)
		squashModel := updatedModel.(*SquashWorkflowModel)

		assert.True(t, squashModel.accepted)
		assert.Equal(t, DecisionAccept, squashModel.reviewDecision)
		assert.True(t, squashModel.done)
		assert.NotNil(t, cmd)
	})

	t.Run("edit key", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		model.phase = WorkflowPhaseReview
		model.message = "feat: test message"
		model.updateActionsForPhase()

		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}
		updatedModel, cmd := model.Update(key)
		
		// The base workflow model handles editing
		assert.NotNil(t, cmd)
		assert.NotNil(t, updatedModel)
	})

	t.Run("regenerate key", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		model.phase = WorkflowPhaseReview
		model.message = "feat: test message"
		model.updateActionsForPhase()

		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
		updatedModel, cmd := model.Update(key)
		squashModel := updatedModel.(*SquashWorkflowModel)

		assert.Equal(t, WorkflowPhaseLoading, squashModel.phase)
		assert.NotNil(t, cmd)
	})

	t.Run("ctrl+c key", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		model.phase = WorkflowPhaseReview
		model.message = "feat: test message"
		model.updateActionsForPhase()

		key := tea.KeyMsg{Type: tea.KeyCtrlC}
		updatedModel, cmd := model.Update(key)
		squashModel := updatedModel.(*SquashWorkflowModel)

		assert.True(t, squashModel.done)
		assert.NotNil(t, cmd)
		assert.Equal(t, context.Canceled, squashModel.err)
	})
}

// Test spinner update
func TestSquashWorkflowModel_Update_SpinnerTick(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	spinnerMsg := spinner.TickMsg{ID: model.spinner.ID(), Time: time.Now()}
	updatedModel, cmd := model.Update(spinnerMsg)
	
	assert.NotNil(t, updatedModel)
	assert.NotNil(t, cmd)
}

// Test window resize
func TestSquashWorkflowModel_Update_WindowResize(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 50}
	updatedModel, cmd := model.Update(resizeMsg)
	
	assert.NotNil(t, updatedModel)
	assert.Nil(t, cmd)
	squashModel := updatedModel.(*SquashWorkflowModel)
	assert.Equal(t, 100, squashModel.width)
	assert.Equal(t, 50, squashModel.height)
}

// Test action handlers
func TestSquashWorkflowModel_ActionHandlers(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	t.Run("handleAccept", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		cmd := model.handleAccept()
		
		assert.NotNil(t, cmd)
		assert.True(t, model.accepted)
		assert.Equal(t, DecisionAccept, model.reviewDecision)
		assert.True(t, model.done)
	})

	t.Run("handleRegenerate", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		model.copySuccess = true
		model.err = errors.New("test error")
		
		mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
			Return("feat: new message", nil)
		
		cmd := model.handleRegenerate()
		
		assert.NotNil(t, cmd)
		assert.Equal(t, WorkflowPhaseLoading, model.phase)
		assert.Equal(t, StageQuery, model.loadingStage)
		assert.False(t, model.copySuccess)
		assert.Nil(t, model.err)
	})

	t.Run("handleQuit", func(t *testing.T) {
		model := NewSquashWorkflowModel(ctx, squashInstance, messages)
		cmd := model.handleQuit()
		
		assert.NotNil(t, cmd)
		assert.Equal(t, DecisionCancel, model.reviewDecision)
		assert.True(t, model.done)
	})
}

// Test View method
func TestSquashWorkflowModel_View(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)
	model.phase = WorkflowPhaseReview
	model.message = "feat: test message"

	view := model.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Generated commit message:")
}

// Test public getters
func TestSquashWorkflowModel_PublicGetters(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)
	
	// Set some values
	model.accepted = true
	model.message = "feat: test message"
	model.copySuccess = true

	assert.True(t, model.IsAccepted())
	assert.Equal(t, "feat: test message", model.GetResult())
	assert.True(t, model.IsCopySuccess())
}

// Test error message handling
func TestSquashWorkflowModel_Update_ErrorMsg(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	testErr := errors.New("critical error")
	errMsg := errorMsg{err: testErr}

	updatedModel, cmd := model.Update(errMsg)
	squashModel := updatedModel.(*SquashWorkflowModel)

	assert.NotNil(t, cmd)
	assert.Equal(t, testErr, squashModel.err)
	assert.True(t, squashModel.done)
}

// Test content rendering in different phases
func TestSquashWorkflowModel_RenderContent(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature", "fix: fix bug"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	t.Run("loading phase", func(t *testing.T) {
		model.phase = WorkflowPhaseLoading
		content := model.renderContent()
		
		assert.Contains(t, content, "Generating consolidated commit message...")
		assert.Contains(t, content, "Processing 2 commit messages")
	})

	t.Run("review phase", func(t *testing.T) {
		model.phase = WorkflowPhaseReview
		model.message = "feat: test message"
		model.editing = false
		content := model.renderContent()
		
		assert.NotEmpty(t, content)
	})

	t.Run("review phase with clipboard success", func(t *testing.T) {
		model.phase = WorkflowPhaseReview
		model.message = "feat: test message"
		model.copySuccess = true
		model.editing = false
		content := model.renderContent()
		
		assert.Contains(t, content, "✅ Copied to clipboard!")
	})

	t.Run("editing mode", func(t *testing.T) {
		model.phase = WorkflowPhaseReview
		model.editing = true
		content := model.renderContent()
		
		assert.NotEmpty(t, content)
	})
}

// Test updateActionsForPhase
func TestSquashWorkflowModel_UpdateActionsForPhase(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSquashClient)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature"}

	model := NewSquashWorkflowModel(ctx, squashInstance, messages)

	tests := []struct {
		name            string
		phase           WorkflowPhase
		editing         bool
		hasError        bool
		expectedActions int
		expectedKeys    []string
	}{
		{
			name:            "loading phase",
			phase:           WorkflowPhaseLoading,
			expectedActions: 0,
		},
		{
			name:            "review phase normal",
			phase:           WorkflowPhaseReview,
			expectedActions: 4,
			expectedKeys:    []string{"A", "E", "R", "Q"},
		},
		{
			name:            "review phase with error",
			phase:           WorkflowPhaseReview,
			hasError:        true,
			expectedActions: 2,
			expectedKeys:    []string{"R", "Q"},
		},
		{
			name:            "review phase editing",
			phase:           WorkflowPhaseReview,
			editing:         true,
			expectedActions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model.phase = tt.phase
			model.editing = tt.editing
			if tt.hasError {
				model.err = errors.New("test error")
			} else {
				model.err = nil
			}

			model.updateActionsForPhase()
			
			assert.Len(t, model.actions, tt.expectedActions)
			
			if tt.expectedKeys != nil {
				for i, key := range tt.expectedKeys {
					assert.Equal(t, key, model.actions[i].Key)
				}
			}
		})
	}
}