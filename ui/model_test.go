package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestModel_StateTransitions(t *testing.T) {
	m := NewModel()

	// 初始状态应为 loading
	require.True(t, m.isLoading)
	require.False(t, m.isDone)

	// 模拟成功消息
	msg := GenerateSuccessMsg{Message: "feat: commit"}
	updated, _ := m.Update(msg)
	um := updated.(Model)
	require.False(t, um.isLoading)
	require.True(t, um.isDone)
	require.Equal(t, "feat: commit", um.message)

	// 重置模型，再测试错误流转
	m = NewModel()
	errMsg := GenerateErrorMsg{Err: errors.New("timeout")}
	updated, _ = m.Update(errMsg)
	um = updated.(Model)
	require.False(t, um.isLoading)
	require.True(t, um.isDone)
	require.EqualError(t, um.err, "timeout")
}

func TestModel_Init(t *testing.T) {
	m := NewModel()
	cmd := m.Init()
	require.Nil(t, cmd)
}

func TestModel_View(t *testing.T) {
	t.Run("loading_state", func(t *testing.T) {
		m := NewModel()
		view := m.View()
		require.Equal(t, "Loading...", view)
	})

	t.Run("error_state", func(t *testing.T) {
		m := NewModel()
		errMsg := GenerateErrorMsg{Err: errors.New("test error")}
		updated, _ := m.Update(errMsg)
		um := updated.(Model)
		
		view := um.View()
		require.Equal(t, "Error: test error", view)
	})

	t.Run("success_state", func(t *testing.T) {
		m := NewModel()
		successMsg := GenerateSuccessMsg{Message: "feat: success"}
		updated, _ := m.Update(successMsg)
		um := updated.(Model)
		
		view := um.View()
		require.Equal(t, "feat: success", view)
	})
}

func TestModel_Update_StartMessage(t *testing.T) {
	m := NewModel()
	// Set to done state first
	m.isDone = true
	m.err = errors.New("previous error")
	
	startMsg := GenerateStartMsg{}
	updated, cmd := m.Update(startMsg)
	um := updated.(Model)
	
	require.True(t, um.isLoading)
	require.False(t, um.isDone)
	require.Nil(t, um.err)
	require.Nil(t, cmd)
}

func TestModel_Update_UnknownMessage(t *testing.T) {
	m := NewModel()
	originalState := m
	
	// Test unknown message type
	updated, cmd := m.Update("unknown")
	um := updated.(Model)
	
	// State should remain unchanged
	require.Equal(t, originalState.isLoading, um.isLoading)
	require.Equal(t, originalState.isDone, um.isDone)
	require.Equal(t, originalState.message, um.message)
	require.Equal(t, originalState.err, um.err)
	require.Nil(t, cmd)
}

// 确保实现 tea.Model 接口
var _ tea.Model = (*Model)(nil)
