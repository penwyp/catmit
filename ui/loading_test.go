package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// ---------------- mock implementations ----------------

type mockCollector struct {
	diff    string
	commits []string
	err     error
}

func (m mockCollector) Diff(ctx context.Context) (string, error) { return m.diff, m.err }
func (m mockCollector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.commits, nil
}

func (m mockCollector) BranchName(ctx context.Context) (string, error) { return "test", nil }
func (m mockCollector) ChangedFiles(ctx context.Context) ([]string, error) {
	return []string{"a.go"}, nil
}

type mockPrompt struct{}

func (mockPrompt) Build(seed, diff string, commits []string, branch string, files []string) string {
	return "prompt"
}

func (mockPrompt) BuildSystemPrompt() string {
	return "system prompt"
}

func (mockPrompt) BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string {
	return "user prompt"
}

type mockClient struct {
	msg string
	err error
}

func (m mockClient) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return m.msg, m.err
}

func (m mockClient) GetCommitMessageLegacy(ctx context.Context, prompt string) (string, error) {
	return m.msg, m.err
}

// -------------------------------------------------------

func runModel(m tea.Model) (tea.Model, error) {
	// For testing, use a test program that doesn't require a TTY
	return tea.NewProgram(m, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(nil)).Run()
}

func TestLoadingModel_Success(t *testing.T) {
	col := mockCollector{diff: "diff", commits: []string{"feat: a"}}
	lm := NewLoadingModel(context.Background(), col, mockPrompt{}, mockClient{msg: "feat: ok"}, "", "en")

	finalModel, err := runModel(lm)
	require.NoError(t, err)

	if m, ok := finalModel.(LoadingModel); ok {
		msg, err := m.IsDone()
		require.NoError(t, err)
		require.Equal(t, "feat: ok", msg)
	} else {
		t.Fatalf("unexpected model type")
	}
}

func TestLoadingModel_Error(t *testing.T) {
	col := mockCollector{err: context.Canceled}
	lm := NewLoadingModel(context.Background(), col, mockPrompt{}, mockClient{}, "", "en")

	finalModel, err := runModel(lm)
	require.NoError(t, err)
	if m, ok := finalModel.(LoadingModel); ok {
		_, e := m.IsDone()
		require.ErrorIs(t, e, context.Canceled)
	} else {
		t.Fatalf("unexpected model type")
	}
}
