package ui

import tea "github.com/charmbracelet/bubbletea"

// ---------------- Message Types ------------------
// GenerateStartMsg 表示开始生成过程
type GenerateStartMsg struct{}

// GenerateSuccessMsg 表示生成成功，携带 commit message
type GenerateSuccessMsg struct {
	Message string
}

// GenerateErrorMsg 表示生成失败
type GenerateErrorMsg struct {
	Err error
}

// ---------------- Model --------------------------
// Model 代表 Bubble Tea 状态模型。
// 仅保留最小字段以通过单元测试；后续可扩展为完整 TUI。
type Model struct {
	isLoading bool
	isDone    bool
	message   string
	err       error
}

// NewModel 返回初始模型，处于 Loading 状态。
func NewModel() Model {
	return Model{isLoading: true}
}

// Init 实现 tea.Model 接口，返回 nil 即可。
func (m Model) Init() tea.Cmd { return nil }

// Update 根据不同 Msg 更新模型。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case GenerateStartMsg:
		m.isLoading = true
		m.isDone = false
		m.err = nil
	case GenerateSuccessMsg:
		m.isLoading = false
		m.isDone = true
		m.message = msg.Message
	case GenerateErrorMsg:
		m.isLoading = false
		m.isDone = true
		m.err = msg.Err
	default:
		// 其他消息保持原状态
	}
	return m, nil
}

// View 返回当前视图字符串。简化实现，后续完善 TUI。
func (m Model) View() string {
	if m.isLoading {
		return "Loading..."
	}
	if m.err != nil {
		return "Error: " + m.err.Error()
	}
	return m.message
}
