package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/pkg/gitinfo"
)

// Stage represents the progress stage
type Stage int

const (
	StagePRCheck    Stage = iota // New: check if PR already exists
	StageCollect
	StagePreprocess       // New: intelligent data preprocessing stage
	StagePrompt
	StageQuery
	StageDone
)

// Interfaces duplicated to decouple from cmd package
type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
	FileStatusSummary(ctx context.Context) (*gitinfo.FileStatusSummary, error)
	ComprehensiveDiff(ctx context.Context) (string, error)
	AnalyzeChanges(ctx context.Context) (*gitinfo.ChangesSummary, error)
}

type promptInterface interface {
	Build(seed, diff string, commits []string, branch string, files []string) string
	BuildSystemPrompt() string
	BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string
	BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error)
	BuildPRSystemPrompt() string
	BuildPRUserPrompt(commits []string) string
}

type clientInterface interface {
	GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	GetCommitMessageStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error)
}







// ---------------- tea.Msg definitions ----------------

type diffCollectedMsg struct {
	diff    string
	commits []string
	branch  string
	files   []string
}

// New: message for preprocessing done
type preprocessDoneMsg struct {
	summary *gitinfo.FileStatusSummary
}


type queryDoneMsg struct {
	message        string
	rawLLMResponse string // Original response before any processing
}

// New: message for smart prompt built
type smartPromptBuiltMsg struct {
	systemPrompt string
	userPrompt   string
}

type errorMsg struct{ err error }

// ---------------- Cmd implementations --------------------

func collectCmd(col collectorInterface, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Use ComprehensiveDiff to include untracked files
		diff, err := col.ComprehensiveDiff(ctx)
		if err != nil {
			return errorMsg{err}
		}
		commits, err := col.RecentCommits(ctx, 10)
		if err != nil {
			return errorMsg{err}
		}
		branch, _ := col.BranchName(ctx)
		files, _ := col.ChangedFiles(ctx)
		return diffCollectedMsg{diff: diff, commits: commits, branch: branch, files: files}
	}
}

// Preprocessing command, get file status summary
func preprocessCmd(col collectorInterface, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Try to use the new FileStatusSummary method
		summary, err := col.FileStatusSummary(ctx)
		if err != nil {
			// If the new method fails, maybe collector does not implement the new interface, return error
			return errorMsg{err}
		}
		return preprocessDoneMsg{summary: summary}
	}
}

// Smart prompt building command, using token budget control
func buildSmartPromptCmd(pb promptInterface, col collectorInterface, ctx context.Context, seed string) tea.Cmd {
	return func() tea.Msg {
		// Try to use the new BuildUserPromptWithBudget method
		systemPrompt := pb.BuildSystemPrompt()
		userPrompt, err := pb.BuildUserPromptWithBudget(ctx, col, seed)
		if err != nil {
			// If the new method fails, fallback to traditional method
			return errorMsg{err}
		}
		return smartPromptBuiltMsg{systemPrompt: systemPrompt, userPrompt: userPrompt}
	}
}

func queryCmd(cli clientInterface, ctx context.Context, systemPrompt, userPrompt string, apiTimeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		// Create timeout context only for API call
		apiCtx, cancel := context.WithTimeout(ctx, apiTimeout)
		defer cancel()
		msg, err := cli.GetCommitMessage(apiCtx, systemPrompt, userPrompt)
		if err != nil {
			return errorMsg{err}
		}
		// For now, both message and rawLLMResponse are the same
		// In the future, message might be processed/cleaned version
		return queryDoneMsg{message: msg, rawLLMResponse: msg}
	}
}


