package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Decision 表示用户在 Review 界面的选择
type Decision int

const (
	DecisionNone Decision = iota
	DecisionAccept
	DecisionCancel
)

// ReviewModel 用于展示 Commit message 供用户确认 / 编辑。
// 当 user 按下 a/e/c 时结束程序并返回决策与最终消息。
// 友好起见，支持上下键切换按钮（简化实现）。

type ReviewModel struct {
	message   string // 当前 commit message
	editing   bool   // 是否处于编辑模式
	textInput textinput.Model
	decision  Decision
	done      bool
}

// NewReviewModel 创建初始模型。
func NewReviewModel(msg string) ReviewModel {
	ti := textinput.New()
	ti.Placeholder = "Edit commit message"
	ti.SetValue(msg)
	ti.CharLimit = 256
	ti.Focus()
	return ReviewModel{
		message:   msg,
		editing:   false,
		textInput: ti,
	}
}

// Init 实现 tea.Model 接口
func (m ReviewModel) Init() tea.Cmd { return nil }

// Update 处理按键事件
func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editing {
			// 编辑模式下交给 textinput 处理
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			switch msg.String() {
			case "enter":
				m.message = strings.TrimSpace(m.textInput.Value())
				m.editing = false
			case "esc":
				m.editing = false
			}
			return m, cmd
		}

		switch msg.String() {
		case "a", "A":
			m.decision = DecisionAccept
			m.done = true
			return m, tea.Quit
		case "e", "E":
			m.editing = true
			return m, nil
		case "c", "C", "q", "Q":
			m.decision = DecisionCancel
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View 渲染
func (m ReviewModel) View() string {
	if m.editing {
		return fmt.Sprintf("\nEditing commit message (enter to save, esc to cancel):\n%s\n", m.textInput.View())
	}

	header := "┌ Commit Preview ─────────────────────────────────────────┐\n"
	footer := "│ [A] Accept  [E] Edit  [C] Cancel                       │\n" +
		"└──────────────────────────────────────────────────────────┘"

	// 对 message 按行分割并填充
	var bodyLines []string
	for _, l := range strings.Split(m.message, "\n") {
		bodyLines = append(bodyLines, fmt.Sprintf("│ %-50s │", l))
	}
	body := strings.Join(bodyLines, "\n") + "\n"
	return header + body + footer
}

// IsDone 返回模型是否结束，以及决策和最终消息。
func (m ReviewModel) IsDone() (bool, Decision, string) {
	return m.done, m.decision, m.message
}
