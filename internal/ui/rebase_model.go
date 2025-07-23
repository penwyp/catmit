package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/internal/rebase"
)

// RebasePhase represents the current phase of the rebase workflow
type RebasePhase int

const (
	RebasePhaseAnalyzing RebasePhase = iota
	RebasePhaseReviewing
	RebasePhaseGenerating
	RebasePhaseConfirming
	RebasePhaseExecuting
	RebasePhaseDone
	RebasePhaseError
)

// RebaseModel is the Bubble Tea model for the rebase workflow
type RebaseModel struct {
	workflow    *rebase.Workflow
	phase       RebasePhase
	analysis    *rebase.AnalysisResult
	message     string
	result      string
	error       error
	spinner     spinner.Model
	width       int
	height      int
	accepted    bool
	copySuccess bool
	backupBranch string

	// Styles
	titleStyle   lipgloss.Style
	infoStyle    lipgloss.Style
	errorStyle   lipgloss.Style
	successStyle lipgloss.Style
	normalStyle  lipgloss.Style
	dimStyle     lipgloss.Style
	helpStyle    lipgloss.Style
}

// NewRebaseModel creates a new RebaseModel
func NewRebaseModel(workflow *rebase.Workflow) *RebaseModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &RebaseModel{
		workflow:   workflow,
		phase:      RebasePhaseAnalyzing,
		spinner:    s,

		// Initialize styles
		titleStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		infoStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("86")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
		normalStyle:  lipgloss.NewStyle(),
		dimStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		helpStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
	}
}

// Run starts the TUI
func (m *RebaseModel) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// IsAccepted returns whether the user accepted the result
func (m *RebaseModel) IsAccepted() bool {
	return m.accepted
}

// GetResult returns the generated commit message
func (m *RebaseModel) GetResult() string {
	return m.result
}

// IsCopySuccess returns whether the result was copied to clipboard
func (m *RebaseModel) IsCopySuccess() bool {
	return m.copySuccess
}

// GetBackupBranch returns the name of the backup branch created
func (m *RebaseModel) GetBackupBranch() string {
	return m.backupBranch
}

// Init implements tea.Model
func (m *RebaseModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.startAnalysis(),
	)
}

// Update implements tea.Model
func (m *RebaseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.phase {
		case RebasePhaseReviewing:
			switch msg.String() {
			case "y", "Y":
				m.phase = RebasePhaseGenerating
				return m, m.generateMessage()
			case "n", "N", "q", "ctrl+c":
				return m, tea.Quit
			}

		case RebasePhaseConfirming:
			switch msg.String() {
			case "a", "A":
				m.accepted = true
				m.phase = RebasePhaseExecuting
				return m, m.executeRebase()
			case "e", "E":
				// TODO: Implement edit functionality
				m.message = "Edit functionality not yet implemented"
				return m, nil
			case "r", "R":
				m.phase = RebasePhaseGenerating
				return m, m.generateMessage()
			case "c", "C", "q", "ctrl+c":
				return m, tea.Quit
			}

		case RebasePhaseDone, RebasePhaseError:
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case analysisMsg:
		if msg.err != nil {
			m.error = msg.err
			m.phase = RebasePhaseError
			return m, nil
		}
		m.analysis = msg.result
		if m.analysis.CanRebase {
			m.phase = RebasePhaseReviewing
		} else {
			m.message = m.analysis.Message
			m.phase = RebasePhaseDone
		}
		return m, nil

	case generatedMsg:
		if msg.err != nil {
			m.error = msg.err
			m.phase = RebasePhaseError
			return m, nil
		}
		m.result = msg.message
		m.phase = RebasePhaseConfirming
		// Try to copy to clipboard
		if err := clipboard.WriteAll(m.result); err == nil {
			m.copySuccess = true
		}
		return m, nil

	case executedMsg:
		if msg.err != nil {
			m.error = msg.err
			m.phase = RebasePhaseError
			return m, nil
		}
		m.backupBranch = msg.backupBranch
		m.phase = RebasePhaseDone
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View implements tea.Model
func (m *RebaseModel) View() string {
	var s strings.Builder

	switch m.phase {
	case RebasePhaseAnalyzing:
		s.WriteString(m.titleStyle.Render("🔍 Analyzing repository state...") + "\n\n")
		s.WriteString(m.spinner.View() + " Checking for unpushed commits\n")

	case RebasePhaseReviewing:
		s.WriteString(m.titleStyle.Render("📋 Commits to Squash") + "\n\n")
		s.WriteString(m.infoStyle.Render(fmt.Sprintf("Branch: %s → %s", m.analysis.CurrentBranch, m.analysis.BaseBranch)) + "\n")
		s.WriteString(m.infoStyle.Render(fmt.Sprintf("Commits to squash: %d", len(m.analysis.UnpushedCommits))) + "\n\n")
		
		s.WriteString(m.normalStyle.Render("The following commits will be squashed:") + "\n")
		s.WriteString(m.dimStyle.Render(rebase.FormatCommitList(m.analysis.UnpushedCommits)) + "\n\n")
		
		s.WriteString(m.helpStyle.Render("Continue? (y/n): "))

	case RebasePhaseGenerating:
		s.WriteString(m.titleStyle.Render("🤖 Generating commit message...") + "\n\n")
		s.WriteString(m.spinner.View() + " Analyzing commit history\n")

	case RebasePhaseConfirming:
		s.WriteString(m.titleStyle.Render("📝 Generated Commit Message") + "\n\n")
		s.WriteString(m.normalStyle.Render(m.result) + "\n\n")
		
		if m.copySuccess {
			s.WriteString(m.successStyle.Render("✓ Copied to clipboard") + "\n\n")
		}
		
		s.WriteString(m.helpStyle.Render("[A]ccept  [E]dit  [R]egenerate  [C]ancel: "))

	case RebasePhaseExecuting:
		s.WriteString(m.titleStyle.Render("🔄 Executing rebase...") + "\n\n")
		s.WriteString(m.spinner.View() + " Creating backup branch\n")
		s.WriteString(m.spinner.View() + " Performing interactive rebase\n")

	case RebasePhaseDone:
		if m.accepted {
			s.WriteString(m.successStyle.Render("✅ Rebase completed successfully!") + "\n\n")
			s.WriteString(m.infoStyle.Render(fmt.Sprintf("Backup branch: %s", m.backupBranch)) + "\n\n")
			s.WriteString(m.normalStyle.Render(rebase.GetRecoveryInstructions(m.backupBranch)) + "\n")
		} else {
			s.WriteString(m.infoStyle.Render(m.message) + "\n")
		}
		s.WriteString("\n" + m.dimStyle.Render("Press any key to exit"))

	case RebasePhaseError:
		s.WriteString(m.errorStyle.Render("❌ Error: "+m.error.Error()) + "\n\n")
		if m.backupBranch != "" {
			s.WriteString(m.normalStyle.Render(rebase.GetRecoveryInstructions(m.backupBranch)) + "\n\n")
		}
		s.WriteString(m.dimStyle.Render("Press any key to exit"))
	}

	return s.String()
}

// Message types for async operations
type analysisMsg struct {
	result *rebase.AnalysisResult
	err    error
}

type generatedMsg struct {
	message string
	err     error
}

type executedMsg struct {
	backupBranch string
	err          error
}

// Commands for async operations
func (m *RebaseModel) startAnalysis() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := m.workflow.Analyze(ctx)
		return analysisMsg{result: result, err: err}
	}
}

func (m *RebaseModel) generateMessage() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		message, err := m.workflow.GenerateCommitMessage(ctx, m.analysis.UnpushedCommits)
		return generatedMsg{message: message, err: err}
	}
}

func (m *RebaseModel) executeRebase() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.workflow.ExecuteRebase(ctx, m.analysis, m.result)
		
		// Extract backup branch name from analysis or error message
		backupBranch := ""
		if m.analysis != nil {
			// The workflow should have created a backup
			backupBranch = fmt.Sprintf("%s_bak", m.analysis.CurrentBranch)
		}
		
		return executedMsg{backupBranch: backupBranch, err: err}
	}
}