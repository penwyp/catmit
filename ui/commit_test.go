package ui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockCommitter 实现 commitInterface 用于测试
type mockCommitter struct {
	mock.Mock
}

func (m *mockCommitter) Commit(ctx context.Context, message string) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func (m *mockCommitter) Push(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockCommitter) StageAll(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockCommitter) HasStagedChanges(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *mockCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockCommitter) NeedsPush(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func TestNewCommitModel(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	
	model := NewCommitModel(ctx, committer, "test: commit message", "en", true, true)
	
	assert.NotNil(t, model)
	assert.Equal(t, CommitStageInit, model.stage)
	assert.Equal(t, "test: commit message", model.message)
	assert.Equal(t, "en", model.lang)
	assert.True(t, model.enablePush)
	assert.True(t, model.stageAll)
	assert.Equal(t, 80, model.terminalWidth)
	assert.Equal(t, 24, model.terminalHeight)
}

func TestCommitModel_Update_WindowSize(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	model := NewCommitModel(ctx, committer, "test message", "en", false, false)
	
	// Test window size update
	newModel, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updatedModel := newModel.(*CommitModel)
	
	assert.Equal(t, 120, updatedModel.terminalWidth)
	assert.Equal(t, 30, updatedModel.terminalHeight)
}

func TestCommitModel_Update_CtrlC(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	model := NewCommitModel(ctx, committer, "test message", "en", false, false)
	
	// Test Ctrl+C cancellation
	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updatedModel := newModel.(*CommitModel)
	
	assert.True(t, updatedModel.done)
	assert.Equal(t, context.Canceled, updatedModel.err)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}

func TestCommitModel_CommitSuccess_NoPush(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	
	model := NewCommitModel(ctx, committer, "test message", "en", false, false)
	
	// Test successful commit without push
	newModel, _ := model.Update(commitDoneMsg{err: nil})
	updatedModel := newModel.(*CommitModel)
	
	assert.Equal(t, CommitStageDone, updatedModel.stage)
}

func TestCommitModel_CommitSuccess_WithPush(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	
	model := NewCommitModel(ctx, committer, "test message", "en", true, false)
	
	// Test successful commit, should proceed to push
	newModel, _ := model.Update(commitDoneMsg{err: nil})
	updatedModel := newModel.(*CommitModel)
	
	assert.Equal(t, CommitStagePushing, updatedModel.stage)
	
	// Test successful push
	newModel2, _ := updatedModel.Update(pushDoneMsg{err: nil})
	finalModel := newModel2.(*CommitModel)
	
	assert.Equal(t, CommitStageDone, finalModel.stage)
}

func TestCommitModel_CommitError(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	commitErr := errors.New("commit failed")
	
	model := NewCommitModel(ctx, committer, "test message", "en", false, false)
	
	// Test commit error
	newModel, cmd := model.Update(commitDoneMsg{err: commitErr})
	updatedModel := newModel.(*CommitModel)
	
	assert.True(t, updatedModel.done)
	assert.Equal(t, commitErr, updatedModel.err)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}

func TestCommitModel_PushError(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	pushErr := errors.New("push failed")
	
	model := NewCommitModel(ctx, committer, "test message", "en", true, false)
	model.stage = CommitStagePushing // 设置为pushing阶段
	
	// Test push error
	newModel, cmd := model.Update(pushDoneMsg{err: pushErr})
	updatedModel := newModel.(*CommitModel)
	
	assert.True(t, updatedModel.done)
	assert.Equal(t, pushErr, updatedModel.err)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}

func TestCommitModel_View(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	model := NewCommitModel(ctx, committer, "test: sample commit message", "en", true, false)
	
	// Test view rendering
	view := model.View()
	
	assert.Contains(t, view, "Commit Progress")
	assert.Contains(t, view, "Message:")
	assert.Contains(t, view, "test: sample commit message")
	assert.Contains(t, view, "┌") // Top border
	assert.Contains(t, view, "└") // Bottom border
	assert.Contains(t, view, "│") // Side borders
}

func TestCommitModel_calculateContentWidth(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	model := NewCommitModel(ctx, committer, "test message", "en", false, false)
	
	// Test minimum width
	model.terminalWidth = 50
	assert.Equal(t, 60, model.calculateContentWidth()) // Should use minimum
	
	// Test normal width
	model.terminalWidth = 100
	assert.Equal(t, 96, model.calculateContentWidth()) // 100 - 4 margin
	
	// Test maximum width
	model.terminalWidth = 150
	assert.Equal(t, 120, model.calculateContentWidth()) // Should use maximum
}

func TestCommitModel_FinalTimeout(t *testing.T) {
	ctx := context.Background()
	committer := &mockCommitter{}
	model := NewCommitModel(ctx, committer, "test message", "en", false, false)
	model.stage = CommitStageDone
	
	// Test final timeout
	newModel, cmd := model.Update(finalTimeoutMsg{})
	updatedModel := newModel.(*CommitModel)
	
	assert.True(t, updatedModel.done)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}