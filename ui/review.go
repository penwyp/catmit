package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Decision 表示用户在 Review 界面的选择
type Decision int

// buttonState 定义按钮的索引
type buttonState int

const (
	DecisionNone Decision = iota
	DecisionAccept
	DecisionCancel
)

const (
	buttonAccept buttonState = iota
	buttonEdit
	buttonCancel
)

// ReviewModel 用于展示 Commit message 供用户确认 / 编辑。
// 当 user 按下 a/e/c 时结束程序并返回决策与最终消息。
// 友好起见，支持上下键切换按钮（简化实现）。

type ReviewModel struct {
	message        string // 当前 commit message
	lang           string // 语言
	editing        bool   // 是否处于编辑模式
	textInput      textinput.Model
	decision       Decision
	done           bool
	selectedButton buttonState
}

// NewReviewModel 创建初始模型。
func NewReviewModel(msg, lang string) *ReviewModel {
	// 移除 \r 并裁剪首尾空白，避免回车符导致 TUI 渲染异常
	cleanMsg := strings.TrimSpace(strings.ReplaceAll(msg, "\r", ""))

	ti := textinput.New()
	ti.Placeholder = "Edit commit message"
	ti.SetValue(cleanMsg)
	ti.CharLimit = 256
	ti.Focus()
	return &ReviewModel{
		message:        cleanMsg,
		lang:           lang,
		editing:        false,
		textInput:      ti,
		selectedButton: buttonAccept, // 默认选中 Accept
	}
}

// Init 实现 tea.Model 接口
func (m *ReviewModel) Init() tea.Cmd { return nil }

// Update 处理按键事件
func (m *ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// 统一处理 Ctrl+C，无论在哪种模式下都直接取消并退出
		if msg.String() == "ctrl+c" {
			m.decision = DecisionCancel
			m.done = true
			return m, tea.Quit
		}

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

		// 导航和选择逻辑
		switch msg.String() {
		// 切换按钮
		case "left", "h", "up", "k":
			m.selectedButton--
			if m.selectedButton < buttonAccept {
				m.selectedButton = buttonCancel
			}
		case "right", "l", "down", "j":
			m.selectedButton++
			if m.selectedButton > buttonCancel {
				m.selectedButton = buttonAccept
			}
		// 快捷键
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
		// 确认选择
		case "enter":
			switch m.selectedButton {
			case buttonAccept:
				m.decision = DecisionAccept
				m.done = true
				return m, tea.Quit
			case buttonEdit:
				m.editing = true
				return m, nil
			case buttonCancel:
				m.decision = DecisionCancel
				m.done = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

// View 渲染
func (m *ReviewModel) View() string {
	if m.editing {
		return fmt.Sprintf("\nEditing commit message (enter to save, esc to cancel):\n%s\n", m.textInput.View())
	}

	// --- 样式定义 ---
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	normalStyle := lipgloss.NewStyle().Padding(0, 1)
	boxStyle := lipgloss.NewStyle().PaddingLeft(2) // 统一 Box 内的左边距

	// --- 动态构建标题 ---
	titleText := fmt.Sprintf("Commit Preview (%s)", m.lang)
	padding := strings.Repeat("─", 56-1-len(titleText))
	header := fmt.Sprintf("┌ %s %s┐\n", titleText, padding)

	// --- 构建消息 Body ---
	var bodyLines []string
	for _, l := range strings.Split(m.message, "\n") {
		l = strings.ReplaceAll(l, "\r", "")
		// 使用 boxStyle 来确保与按钮行的对齐
		bodyLines = append(bodyLines, boxStyle.Render(fmt.Sprintf("%-56s", l)))
	}
	body := "│" + strings.Join(bodyLines, "│\n│") + "│\n"

	// --- 构建可交互按钮 ---
	btnAccept := "[A] Accept"
	btnEdit := "[E] Edit"
	btnCancel := "[C] Cancel"

	if m.selectedButton == buttonAccept {
		btnAccept = selectedStyle.Render(btnAccept)
	} else {
		btnAccept = normalStyle.Render(btnAccept)
	}

	if m.selectedButton == buttonEdit {
		btnEdit = selectedStyle.Render(btnEdit)
	} else {
		btnEdit = normalStyle.Render(btnEdit)
	}

	if m.selectedButton == buttonCancel {
		btnCancel = selectedStyle.Render(btnCancel)
	} else {
		btnCancel = normalStyle.Render(btnCancel)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, btnAccept, btnEdit, btnCancel)
	// 使用 boxStyle 来对齐
	buttonRow := "│" + boxStyle.Render(buttons)
	// 计算右侧填充
	rightPadding := 58 - lipgloss.Width(buttonRow)
	if rightPadding < 0 {
		rightPadding = 0
	}
	buttonRow = buttonRow + strings.Repeat(" ", rightPadding) + "│\n"

	// --- 组装 Footer ---
	blankLine := fmt.Sprintf("│ %-56s │\n", "")
	footer := blankLine + buttonRow + "└──────────────────────────────────────────────────────────┘"

	return header + body + footer
}

// IsDone 返回模型是否结束，以及决策和最终消息。
func (m *ReviewModel) IsDone() (bool, Decision, string) {
	return m.done, m.decision, m.message
}
