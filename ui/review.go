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
	// 响应式终端尺寸支持
	terminalWidth  int // 终端宽度
	terminalHeight int // 终端高度
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
		terminalWidth:  80,           // 默认宽度，会通过 WindowSizeMsg 更新
		terminalHeight: 24,           // 默认高度，会通过 WindowSizeMsg 更新
	}
}

// Init 实现 tea.Model 接口
func (m *ReviewModel) Init() tea.Cmd { return nil }

// Update 处理按键事件
func (m *ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// 更新终端尺寸
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		return m, nil
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
		case "c", "C", "q", "Q", "esc":
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

// calculateContentWidth 计算基于终端宽度的动态内容宽度
func (m *ReviewModel) calculateContentWidth() int {
	const (
		minWidth = 60  // 最小宽度
		maxWidth = 120 // 最大宽度
		margin   = 4   // 左右边距
	)
	
	// 计算可用宽度（去除边距）
	availableWidth := m.terminalWidth - margin
	
	// 应用最小和最大宽度约束
	if availableWidth < minWidth {
		return minWidth
	}
	if availableWidth > maxWidth {
		return maxWidth
	}
	
	return availableWidth
}


// View 渲染
func (m *ReviewModel) View() string {
	// --- 调色板 ---
	const (
		cGray   = lipgloss.Color("245")
		cBlue   = lipgloss.Color("39")
		cGreen  = lipgloss.Color("42")
		cYellow = lipgloss.Color("220")
		cRed    = lipgloss.Color("196")
		cWhite  = lipgloss.Color("255")
		cBlack  = lipgloss.Color("0")
		padding = 1
	)
	
	// 动态计算内容宽度
	contentWidth := m.calculateContentWidth()

	// --- 编辑模式 ---
	if m.editing {
		promptStyle := lipgloss.NewStyle().Foreground(cYellow).Bold(true)
		prompt := promptStyle.Render("Editing commit message (enter to save, esc to cancel):")
		return fmt.Sprintf("\n%s\n%s\n", prompt, m.textInput.View())
	}

	// --- 样式定义 ---
	borderStyle := lipgloss.NewStyle().Foreground(cBlue)
	titleStyle := lipgloss.NewStyle().Foreground(cWhite).Bold(true)
	langStyle := lipgloss.NewStyle().Foreground(cGray)
	commitTypeStyle := lipgloss.NewStyle().Foreground(cYellow)
	commitDescStyle := lipgloss.NewStyle().Foreground(cWhite)
	commitBodyStyle := lipgloss.NewStyle().Foreground(cGray)

	// --- 辅助函数：行渲染器 ---
	renderLine := func(content string) string {
		contentDisplayWidth := lipgloss.Width(content)
		// 处理溢出情况：如果内容太长，进行截断
		if contentDisplayWidth > contentWidth {
			// 使用智能截断，保留重要信息
			truncated := truncateContent(content, contentWidth-3) + "..."
			content = truncated
			contentDisplayWidth = lipgloss.Width(content)
		}
		
		linePadding := contentWidth - contentDisplayWidth
		if linePadding < 0 {
			linePadding = 0
		}
		return borderStyle.Render("│") + content + strings.Repeat(" ", linePadding) + borderStyle.Render("│")
	}

	// --- 辅助函数：按钮渲染器 ---
	renderButton := func(hint, text string, isSelected bool, hintStyle, textStyle, selectedBg lipgloss.Color) string {
		hStyle := lipgloss.NewStyle().Foreground(hintStyle)
		tStyle := lipgloss.NewStyle().Foreground(textStyle)

		if isSelected {
			// 当按钮被选中时，设置高对比度的前景色以确保可读性
			fgColor := cBlack
			// 红色背景上白色文字更清晰
			if selectedBg == cRed {
				fgColor = cWhite
			}
			hStyle = hStyle.Copy().Background(selectedBg).Foreground(fgColor)
			tStyle = tStyle.Copy().Background(selectedBg).Foreground(fgColor)
		}

		return lipgloss.JoinHorizontal(lipgloss.Top,
			hStyle.Padding(0, 1).Render(hint),
			tStyle.Padding(0, 1).Render(text),
		)
	}

	// --- 构建标题 ---
	titleText := titleStyle.Render("Commit Preview") + langStyle.Render(fmt.Sprintf(" (%s)", m.lang))
	titlePadding := contentWidth - lipgloss.Width(titleText)
	if titlePadding < 0 {
		titlePadding = 0
	}
	header := borderStyle.Render("┌") + strings.Repeat(borderStyle.Render("─"), titlePadding/2) +
		titleText + strings.Repeat(borderStyle.Render("─"), titlePadding-titlePadding/2) +
		borderStyle.Render("┐")

	// --- 构建消息 Body ---
	var bodyBuilder strings.Builder
	lines := strings.Split(m.message, "\n")

	// 渲染第一行 (Subject)
	if len(lines) > 0 {
		parts := strings.SplitN(lines[0], ":", 2)
		var subject string
		if len(parts) == 2 {
			subject = commitTypeStyle.Render(parts[0]+":") + commitDescStyle.Render(parts[1])
		} else {
			subject = commitDescStyle.Render(lines[0])
		}
		bodyBuilder.WriteString(renderLine(" "+subject) + "\n")
	}

	// 渲染后续行 (Body)
	if len(lines) > 1 {
		bodyBuilder.WriteString(renderLine("") + "\n") // 空行
		bodyText := strings.Join(lines[1:], "\n")
		// 对 Body 进行自动换行，-2 是为了左右的内边距
		wrappedBody := wordWrap(bodyText, contentWidth-2)
		for _, l := range strings.Split(wrappedBody, "\n") {
			lineContent := " " + commitBodyStyle.Render(l)
			bodyBuilder.WriteString(renderLine(lineContent) + "\n")
		}
	}

	// --- 构建可交互按钮 ---
	btnAccept := renderButton("[A]", "Accept", m.selectedButton == buttonAccept, cGray, cGreen, cGreen)
	btnEdit := renderButton("[E]", "Edit", m.selectedButton == buttonEdit, cGray, cYellow, cYellow)
	btnCancel := renderButton("[C]", "Cancel", m.selectedButton == buttonCancel, cGray, cRed, cRed)
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, btnAccept, "  ", btnEdit, "  ", btnCancel)
	
	// 检查按钮是否超出内容宽度，如果超出则调整布局
	buttonsWidth := lipgloss.Width(buttons)
	if buttonsWidth > contentWidth-2 { // -2 for padding
		// 使用紧凑布局
		buttons = lipgloss.JoinHorizontal(lipgloss.Top, btnAccept, " ", btnEdit, " ", btnCancel)
		buttonsWidth = lipgloss.Width(buttons)
		if buttonsWidth > contentWidth-2 {
			// 如果仍然太长，使用最紧凑的布局
			buttons = btnAccept + " " + btnEdit + " " + btnCancel
		}
	}

	// --- 组装 Footer ---
	blankLine := renderLine("")
	buttonRow := renderLine(" " + buttons)
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", contentWidth) + "┘")
	footer := strings.Join([]string{blankLine, buttonRow, bottomBorder}, "\n")

	// 移除 body 末尾可能存在的多余换行符，避免破坏布局
	finalBody := strings.TrimRight(bodyBuilder.String(), "\n")

	return strings.Join([]string{header, finalBody, footer}, "\n")
}

// IsDone 返回模型是否结束，以及决策和最终消息。
func (m *ReviewModel) IsDone() (bool, Decision, string) {
	return m.done, m.decision, m.message
}
