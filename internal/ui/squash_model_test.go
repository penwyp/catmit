package ui

import (
	"context"
	"testing"

	"github.com/penwyp/catmit/internal/squash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSquashClientUI is a mock implementation for testing UI
type MockSquashClientUI struct {
	mock.Mock
}

func (m *MockSquashClientUI) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
}

func TestNewSquashModel(t *testing.T) {
	// Create a real squash instance with mock client
	mockClient := new(MockSquashClientUI)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature", "fix: fix bug"}

	model := NewSquashModel(squashInstance, messages)

	assert.NotNil(t, model)
	assert.Equal(t, messages, model.messages)
	assert.Equal(t, SquashPhaseGenerating, model.phase)
	assert.False(t, model.accepted)
	assert.False(t, model.copySuccess)
	assert.Empty(t, model.result)
}

func TestSquashModel_Init(t *testing.T) {
	mockClient := new(MockSquashClientUI)
	squashInstance := squash.New(mockClient, "en")
	messages := []string{"feat: add feature", "fix: fix bug"}
	model := NewSquashModel(squashInstance, messages)

	cmd := model.Init()
	assert.NotNil(t, cmd)
	// The Init command should start the generation process
}

func TestSquashModel_IsAccepted(t *testing.T) {
	mockClient := new(MockSquashClientUI)
	squashInstance := squash.New(mockClient, "en")
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})

	// Initially not accepted
	assert.False(t, model.IsAccepted())

	// Set accepted
	model.accepted = true
	assert.True(t, model.IsAccepted())
}

func TestSquashModel_GetResult(t *testing.T) {
	mockClient := new(MockSquashClientUI)
	squashInstance := squash.New(mockClient, "en")
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})

	// Initially empty
	assert.Empty(t, model.GetResult())

	// Set result
	expectedResult := "feat: consolidated commit message"
	model.result = expectedResult
	assert.Equal(t, expectedResult, model.GetResult())
}

func TestSquashModel_IsCopySuccess(t *testing.T) {
	mockClient := new(MockSquashClientUI)
	squashInstance := squash.New(mockClient, "en")
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})

	// Initially false
	assert.False(t, model.IsCopySuccess())

	// Set copy success
	model.copySuccess = true
	assert.True(t, model.IsCopySuccess())
}