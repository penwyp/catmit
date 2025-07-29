package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/pkg/gitinfo"
)

// Action represents a user action with keyboard shortcuts
type Action struct {
	Key     string         // e.g., "A", "Enter", "Esc"
	Label   string         // e.g., "Accept", "Cancel"
	Handler func() tea.Cmd // Action handler
}

// BaseModel provides a unified TUI model that all other models can embed
type BaseModel struct {
	// Display properties
	title  string
	width  int
	height int

	// State management
	done bool
	err  error

	// Navigation
	actions  []Action
	selected int

	// Styles
	styles UIStyles

	// Content renderer - to be provided by embedding model
	contentRenderer func() string
}

// NewBaseModel creates a new base model with default settings
func NewBaseModel(title string, actions []Action) BaseModel {
	return BaseModel{
		title:      title,
		actions:    actions,
		selected:   0,
		styles:     DefaultStyles(),
		width:      80,
		height:     24,
	}
}

// HandleKeyboard processes keyboard input with unified navigation
func (m *BaseModel) HandleKeyboard(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c", "q", "Q":
		m.done = true
		return tea.Quit

	case "left", "h", "up", "k":
		if m.selected > 0 {
			m.selected--
		} else {
			m.selected = len(m.actions) - 1 // Wrap around
		}

	case "right", "l", "down", "j":
		if m.selected < len(m.actions)-1 {
			m.selected++
		} else {
			m.selected = 0 // Wrap around
		}

	case "enter", " ":
		if m.selected >= 0 && m.selected < len(m.actions) {
			return m.actions[m.selected].Handler()
		}

	default:
		// Check letter key shortcuts
		key := strings.ToUpper(msg.String())
		for i, action := range m.actions {
			if strings.HasPrefix(strings.ToUpper(action.Key), key) {
				m.selected = i
				return action.Handler()
			}
		}
	}
	return nil
}

// HandleWindowSize updates the model's dimensions
func (m *BaseModel) HandleWindowSize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
}

// RenderActions renders the action buttons in squash style
func (m *BaseModel) RenderActions() string {
	if len(m.actions) == 0 {
		return ""
	}

	colors := DefaultColors()
	actionStyle := lipgloss.NewStyle().
		Foreground(colors.BrightCyan).
		Bold(true)

	selectedStyle := lipgloss.NewStyle().
		Background(colors.BrightCyan).
		Foreground(colors.Black).
		Bold(true).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Padding(0, 1)

	var rendered []string
	for i, action := range m.actions {
		label := fmt.Sprintf("[%s]%s", action.Key, action.Label)

		if i == m.selected {
			rendered = append(rendered, selectedStyle.Render(label))
		} else {
			keyPart := actionStyle.Render(fmt.Sprintf("[%s]", action.Key))
			labelPart := action.Label
			rendered = append(rendered, normalStyle.Render(keyPart+labelPart))
		}
	}

	return strings.Join(rendered, "  ")
}

// View renders the model
func (m *BaseModel) View() string {
	return m.standardView()
}

// standardView renders the full UI, clearing screen each time
func (m *BaseModel) standardView() string {
	colors := DefaultColors()
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colors.BrightBlue)

	contentStyle := lipgloss.NewStyle().
		Padding(1, 2)

	var content []string

	// Title
	if m.title != "" {
		content = append(content, titleStyle.Render(m.title), "")
	}

	// Main content (provided by embedding model)
	if m.contentRenderer != nil {
		mainContent := m.contentRenderer()
		if mainContent != "" {
			content = append(content, mainContent, "")
		}
	}

	// Error display
	if m.err != nil {
		// Skip displaying ErrNoDiff here as it will be handled by the error handler
		if !errors.Is(m.err, gitinfo.ErrNoDiff) {
			errorStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(colors.BrightRed)
			content = append(content, errorStyle.Render("Error: "+m.err.Error()), "")
		}
	}

	// Actions
	if len(m.actions) > 0 {
		content = append(content, m.RenderActions())
	}

	return contentStyle.Render(strings.Join(content, "\n"))
}

// SetContentRenderer sets the function that renders the main content
func (m *BaseModel) SetContentRenderer(renderer func() string) {
	m.contentRenderer = renderer
}

// SetTitle updates the model title
func (m *BaseModel) SetTitle(title string) {
	m.title = title
}

// SetActions updates the available actions
func (m *BaseModel) SetActions(actions []Action) {
	m.actions = actions
	if m.selected >= len(actions) {
		m.selected = len(actions) - 1
	}
	if m.selected < 0 && len(actions) > 0 {
		m.selected = 0
	}
}

// IsDone returns whether the model has finished
func (m *BaseModel) IsDone() bool {
	return m.done
}

// GetError returns any error that occurred
func (m *BaseModel) GetError() error {
	return m.err
}

// SetError sets an error on the model
func (m *BaseModel) SetError(err error) {
	m.err = err
}

// GetSelected returns the currently selected action index
func (m *BaseModel) GetSelected() int {
	return m.selected
}

// GetActions returns the current actions
func (m *BaseModel) GetActions() []Action {
	return m.actions
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
