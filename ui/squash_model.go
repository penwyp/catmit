package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/squash"
	"github.com/charmbracelet/bubbles/spinner"
)

// SquashPhase 表示 squash 流程的阶段
type SquashPhase int

const (
	SquashPhaseGenerating SquashPhase = iota
	SquashPhaseReviewing
	SquashPhaseDone
)

// SquashModel 是 squash 命令的 TUI 模型
type SquashModel struct {
	squash        *squash.Squash
	messages      []string
	result        string
	phase         SquashPhase
	spinner       spinner.Model
	err           error
	width         int
	height        int
	selectedIndex int
	copySuccess   bool
}

// squashMsg 用于传递生成的结果
type squashMsg struct {
	result string
	err    error
}

// NewSquashModel 创建一个新的 SquashModel
func NewSquashModel(s *squash.Squash, messages []string) *SquashModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	
	return &SquashModel{
		squash:   s,
		messages: messages,
		phase:    SquashPhaseGenerating,
		spinner:  sp,
	}
}

// Run 运行 TUI
func (m *SquashModel) Run() error {
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// Init 初始化模型
func (m *SquashModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.generateCommitMessage(),
	)
}

// generateCommitMessage 生成合并后的 commit message
func (m *SquashModel) generateCommitMessage() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := m.squash.Generate(ctx, m.messages)
		return squashMsg{result: result, err: err}
	}
}

// Update 处理消息
func (m *SquashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.phase {
		case SquashPhaseReviewing:
			switch msg.String() {
			case "a", "A":
				// Accept
				m.phase = SquashPhaseDone
				// 输出结果
				fmt.Println(m.result)
				// 尝试复制到剪贴板
				if err := clipboard.WriteAll(m.result); err == nil {
					fmt.Fprintln(os.Stderr, "✓ Copied to clipboard")
				}
				return m, tea.Quit

			case "r", "R":
				// Regenerate
				m.phase = SquashPhaseGenerating
				m.copySuccess = false
				return m, tea.Batch(
					m.spinner.Tick,
					m.generateCommitMessage(),
				)

			case "e", "E":
				// Edit - 这里可以实现编辑功能，暂时先返回提示
				fmt.Fprintln(os.Stderr, "Edit feature not yet implemented")
				return m, nil

			case "q", "Q", "ctrl+c":
				// Quit
				return m, tea.Quit
			}
		}

	case squashMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = SquashPhaseReviewing
			return m, nil
		}
		m.result = msg.result
		m.phase = SquashPhaseReviewing
		// 尝试复制到剪贴板
		if err := clipboard.WriteAll(m.result); err == nil {
			m.copySuccess = true
		}
		return m, nil

	case spinner.TickMsg:
		if m.phase == SquashPhaseGenerating {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View 渲染视图
func (m *SquashModel) View() string {
	switch m.phase {
	case SquashPhaseGenerating:
		return m.viewGenerating()
	case SquashPhaseReviewing:
		return m.viewReviewing()
	case SquashPhaseDone:
		return ""
	default:
		return ""
	}
}

// viewGenerating 渲染生成中的视图
func (m *SquashModel) viewGenerating() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12"))

	content := []string{
		titleStyle.Render("Squashing Commit Messages"),
		"",
		m.spinner.View() + " Generating consolidated commit message...",
		"",
		fmt.Sprintf("Processing %d commit messages", len(m.messages)),
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(content, "\n"))
}

// viewReviewing 渲染审核视图
func (m *SquashModel) viewReviewing() string {
	if m.err != nil {
		return m.viewError()
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12"))

	resultStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(min(80, m.width-4))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10"))

	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)

	content := []string{
		titleStyle.Render("Generated commit message:"),
		"",
		resultStyle.Render(m.result),
		"",
	}

	if m.copySuccess {
		content = append(content, successStyle.Render("✅ Copied to clipboard!"), "")
	}

	actions := []string{
		actionStyle.Render("[A]") + "ccept  ",
		actionStyle.Render("[R]") + "egenerate  ",
		actionStyle.Render("[E]") + "dit  ",
		actionStyle.Render("[Q]") + "uit",
	}
	content = append(content, strings.Join(actions, ""))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(content, "\n"))
}

// viewError 渲染错误视图
func (m *SquashModel) viewError() string {
	errorStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9"))

	content := []string{
		errorStyle.Render("Error:"),
		"",
		m.err.Error(),
		"",
		"Press [R] to retry or [Q] to quit",
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(content, "\n"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}