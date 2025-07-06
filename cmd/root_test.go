package cmd

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/penwyp/catmit/collector"
	"github.com/stretchr/testify/require"
)

// ---------------- Mock 实现 ----------------

type mockCollector struct {
	diff    string
	commits []string
	err     error
}

func (m mockCollector) RecentCommits(_ context.Context, n int) ([]string, error) {
	return m.commits, m.err
}
func (m mockCollector) Diff(_ context.Context) (string, error) {
	return m.diff, m.err
}
func (m mockCollector) BranchName(_ context.Context) (string, error) { return "test", nil }
func (m mockCollector) ChangedFiles(_ context.Context) ([]string, error) {
	return []string{"file.txt"}, nil
}

type mockPrompt struct{}

func (mockPrompt) Build(seed, diff string, commits []string, branch string, files []string) string {
	return "prompt"
}

type mockClient struct {
	message string
	err     error
}

func (m mockClient) GetCommitMessage(ctx context.Context, prompt string) (string, error) {
	return m.message, m.err
}

type recordCommitter struct {
	called bool
	msg    string
}

func (r *recordCommitter) Commit(message string) error { r.called = true; r.msg = message; return nil }

// ------------------------------------------------

func TestRoot_DryRun(t *testing.T) {
	// 注入 mock 依赖
	flagDryRun = false
	collectorProvider = func() collectorInterface { return mockCollector{diff: "diff", commits: []string{"feat: a"}} }
	promptProvider = func(lang string) promptInterface { return mockPrompt{} }
	clientProvider = func(timeout time.Duration) clientInterface { return mockClient{message: "feat: auto"} }
	comm := &recordCommitter{}
	committer = comm

	rootCmd.SetArgs([]string{"--dry-run"})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	err := rootCmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "feat: auto")
	require.False(t, comm.called)
}

func TestRoot_YesFlag_Commits(t *testing.T) {
	flagDryRun = false
	collectorProvider = func() collectorInterface { return mockCollector{diff: "diff", commits: nil} }
	promptProvider = func(lang string) promptInterface { return mockPrompt{} }
	clientProvider = func(timeout time.Duration) clientInterface { return mockClient{message: "feat: yes"} }
	comm := &recordCommitter{}
	committer = comm

	rootCmd.SetArgs([]string{"-y"})
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	err := rootCmd.Execute()
	require.NoError(t, err)
	require.True(t, comm.called)
	require.Equal(t, "feat: yes", comm.msg)
}

func TestRoot_NoDiff_NoCommit(t *testing.T) {
	flagDryRun = false
	collectorProvider = func() collectorInterface { return mockCollector{diff: "", err: collector.ErrNoDiff} }
	promptProvider = func(lang string) promptInterface { return mockPrompt{} }

	rootCmd.SetArgs([]string{})
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	err := rootCmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Nothing to commit")
}

func TestExecute(t *testing.T) {
	// Save original values
	originalCollectorProvider := collectorProvider
	originalPromptProvider := promptProvider
	originalClientProvider := clientProvider
	originalCommitter := committer
	originalFlagDryRun := flagDryRun
	
	// Restore after test
	defer func() {
		collectorProvider = originalCollectorProvider
		promptProvider = originalPromptProvider
		clientProvider = originalClientProvider
		committer = originalCommitter
		flagDryRun = originalFlagDryRun
	}()

	// Set up test scenario
	flagDryRun = false
	collectorProvider = func() collectorInterface { return mockCollector{diff: "diff", commits: []string{"feat: a"}} }
	promptProvider = func(lang string) promptInterface { return mockPrompt{} }
	clientProvider = func(timeout time.Duration) clientInterface { return mockClient{message: "feat: execute"} }
	committer = &recordCommitter{}

	// Test with dry-run flag
	rootCmd.SetArgs([]string{"--dry-run"})
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	err := Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "feat: execute")
}

func TestDefaultProviders(t *testing.T) {
	// Test defaultCollectorProvider
	t.Run("defaultCollectorProvider", func(t *testing.T) {
		col := defaultCollectorProvider()
		require.NotNil(t, col)
		// We can't easily test the actual functionality without git, 
		// but we can verify it returns a collector interface
		require.Implements(t, (*collectorInterface)(nil), col)
	})

	// Test defaultPromptProvider
	t.Run("defaultPromptProvider", func(t *testing.T) {
		prompt := defaultPromptProvider("en")
		require.NotNil(t, prompt)
		require.Implements(t, (*promptInterface)(nil), prompt)
		
		// Test with different language
		promptZh := defaultPromptProvider("zh")
		require.NotNil(t, promptZh)
		require.Implements(t, (*promptInterface)(nil), promptZh)
	})

	// Test defaultClientProvider
	t.Run("defaultClientProvider", func(t *testing.T) {
		client := defaultClientProvider(10 * time.Second)
		require.NotNil(t, client)
		require.Implements(t, (*clientInterface)(nil), client)
	})
}

func TestRealRunner(t *testing.T) {
	runner := realRunner{}
	
	// Test successful command
	output, err := runner.Run(context.Background(), "echo", "hello")
	require.NoError(t, err)
	require.Contains(t, string(output), "hello")
	
	// Test command with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	_, err = runner.Run(ctx, "sleep", "1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
}

func TestDefaultCommitter(t *testing.T) {
	// We can't easily test the actual git commit without affecting the repository
	// So we'll test the structure and ensure it implements the interface
	committer := defaultCommitter{}
	require.Implements(t, (*commitInterface)(nil), committer)
}

func TestRun_ErrorPaths(t *testing.T) {
	// Save original values
	originalCollectorProvider := collectorProvider
	originalPromptProvider := promptProvider
	originalClientProvider := clientProvider
	originalCommitter := committer
	originalFlagDryRun := flagDryRun
	originalFlagYes := flagYes
	
	// Restore after test
	defer func() {
		collectorProvider = originalCollectorProvider
		promptProvider = originalPromptProvider
		clientProvider = originalClientProvider
		committer = originalCommitter
		flagDryRun = originalFlagDryRun
		flagYes = originalFlagYes
	}()

	t.Run("collector_error", func(t *testing.T) {
		flagDryRun = false
		flagYes = true
		collectorProvider = func() collectorInterface { 
			return mockCollector{diff: "", err: context.DeadlineExceeded} 
		}
		
		rootCmd.SetArgs([]string{"-y"})
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		err := rootCmd.Execute()
		require.Error(t, err)
		require.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("recent_commits_error", func(t *testing.T) {
		flagDryRun = false
		flagYes = true
		collectorProvider = func() collectorInterface { 
			return mockCollector{diff: "diff", commits: nil, err: context.DeadlineExceeded} 
		}
		
		rootCmd.SetArgs([]string{"-y"})
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		err := rootCmd.Execute()
		require.Error(t, err)
		require.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("client_error", func(t *testing.T) {
		flagDryRun = false
		flagYes = true
		collectorProvider = func() collectorInterface { 
			return mockCollector{diff: "diff", commits: []string{"feat: a"}} 
		}
		promptProvider = func(lang string) promptInterface { return mockPrompt{} }
		clientProvider = func(timeout time.Duration) clientInterface { 
			return mockClient{message: "", err: context.DeadlineExceeded} 
		}
		
		rootCmd.SetArgs([]string{"-y"})
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		err := rootCmd.Execute()
		require.Error(t, err)
		require.Equal(t, context.DeadlineExceeded, err)
	})
}

func TestRun_WithSeedText(t *testing.T) {
	// Save original values
	originalCollectorProvider := collectorProvider
	originalPromptProvider := promptProvider
	originalClientProvider := clientProvider
	originalCommitter := committer
	originalFlagDryRun := flagDryRun
	
	// Restore after test
	defer func() {
		collectorProvider = originalCollectorProvider
		promptProvider = originalPromptProvider
		clientProvider = originalClientProvider
		committer = originalCommitter
		flagDryRun = originalFlagDryRun
	}()

	flagDryRun = false
	collectorProvider = func() collectorInterface { return mockCollector{diff: "diff", commits: []string{"feat: a"}} }
	promptProvider = func(lang string) promptInterface { return mockPrompt{} }
	clientProvider = func(timeout time.Duration) clientInterface { return mockClient{message: "feat: with seed"} }
	committer = &recordCommitter{}

	rootCmd.SetArgs([]string{"--dry-run", "seed text"})
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	err := rootCmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "feat: with seed")
}
