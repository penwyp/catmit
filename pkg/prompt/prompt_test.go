package prompt

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/stretchr/testify/require"
)

// Test building prompt in English
func TestBuilder_Build_English(t *testing.T) {
	b := NewBuilder("en", 0)
	diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
	commits := []string{"feat: add feature", "fix: bug"}

	out := b.Build("", diff, commits, "test", []string{"main.go"})

	require.Contains(t, out, "MUST be in English")
	require.Contains(t, out, diff)
	require.Contains(t, out, "feat: add feature")
}

// Test building prompt in Chinese
func TestBuilder_Build_Chinese(t *testing.T) {
	b := NewBuilder("zh", 0)
	diff := "diff --git a/file.txt b/file.txt\n+增加一行"

	out := b.Build("seed text", diff, nil, "test", []string{"file.txt"})
	require.Contains(t, out, "MUST be in Chinese")
	require.Contains(t, out, "seed text")
}

// Test diff truncation logic
func TestBuilder_Truncation(t *testing.T) {
	longDiff := strings.Repeat("a", 500)
	b := NewBuilder("en", 100) // limit to 100 bytes

	out := b.Build("", longDiff, nil, "dev", []string{})

	require.Contains(t, out, "(diff truncated)")
	// Ensure the final diff part is truncated
	require.LessOrEqual(t, len(out), 2000) // Result should not be too long (system prompt is longer)
}

// Test system prompt and user prompt separation
func TestBuilder_BuildSystemPrompt_English(t *testing.T) {
	b := NewBuilder("en", 0)
	systemPrompt := b.BuildSystemPrompt()

	// Validate system prompt contains key elements
	require.Contains(t, systemPrompt, "expert software engineer")
	require.Contains(t, systemPrompt, "Conventional Commits")
	require.Contains(t, systemPrompt, "MUST be in English")
	require.Contains(t, systemPrompt, "feat, fix, refactor")
	require.Contains(t, systemPrompt, "EXAMPLE")
	require.Contains(t, systemPrompt, "commit message text")
}

func TestBuilder_BuildSystemPrompt_Chinese(t *testing.T) {
	b := NewBuilder("zh", 0)
	systemPrompt := b.BuildSystemPrompt()

	// Validate Chinese language instruction
	require.Contains(t, systemPrompt, "MUST be in Chinese")
	// Other elements should still exist
	require.Contains(t, systemPrompt, "expert software engineer")
	require.Contains(t, systemPrompt, "Conventional Commits")
}

// Test user prompt content
func TestBuilder_BuildUserPrompt(t *testing.T) {
	b := NewBuilder("en", 0)
	diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
	commits := []string{"feat: add feature", "fix: bug"}

	userPrompt := b.BuildUserPrompt("seed text", diff, commits, "test", []string{"main.go"})

	// Validate user prompt contains data elements
	require.Contains(t, userPrompt, "Seed: seed text")
	require.Contains(t, userPrompt, "Branch: test")
	require.Contains(t, userPrompt, "Changed files: main.go")
	require.Contains(t, userPrompt, "Recent commits:")
	require.Contains(t, userPrompt, "feat: add feature")
	require.Contains(t, userPrompt, "Git diff:")
	require.Contains(t, userPrompt, diff)
}

// Test user prompt with diff truncation
func TestBuilder_BuildUserPrompt_WithTruncation(t *testing.T) {
	b := NewBuilder("en", 100) // limit to 100 bytes
	longDiff := strings.Repeat("a", 500)

	userPrompt := b.BuildUserPrompt("", longDiff, nil, "", []string{})

	// Validate diff truncation
	require.Contains(t, userPrompt, "(diff truncated)")
	// Ensure the full original diff is not included
	require.NotContains(t, userPrompt, strings.Repeat("a", 300))
}

// Test user prompt with empty context
func TestBuilder_BuildUserPrompt_EmptyContext(t *testing.T) {
	b := NewBuilder("en", 0)

	userPrompt := b.BuildUserPrompt("", "", nil, "", []string{})

	// Validate handling of empty context
	require.Equal(t, "No changes detected.", userPrompt)
}

// Test backward compatibility
func TestBuilder_Build_BackwardCompatibility(t *testing.T) {
	b := NewBuilder("en", 0)
	diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
	commits := []string{"feat: add feature"}

	// Old method should still work
	oldResult := b.Build("seed", diff, commits, "test", []string{"main.go"})

	// New method's combined result should be similar to the old method
	systemPrompt := b.BuildSystemPrompt()
	userPrompt := b.BuildUserPrompt("seed", diff, commits, "test", []string{"main.go"})
	newResult := systemPrompt + "\n\n" + userPrompt

	// Validate results are similar (content may not be exactly the same, but both contain key info)
	require.Contains(t, oldResult, "MUST be in English")
	require.Contains(t, oldResult, "seed")
	require.Contains(t, oldResult, diff)
	require.Contains(t, newResult, "expert software engineer")
	require.Contains(t, newResult, "seed")
	require.Contains(t, newResult, diff)
}

// Test token budget feature
func TestNewBuilderWithTokenBudget(t *testing.T) {
	t.Parallel()

	b := NewBuilderWithTokenBudget("en", 0, 4000)

	require.Equal(t, 4000, b.tokenBudget.MaxTokens)
	require.Equal(t, 1000, b.tokenBudget.ReservedTokens) // 25% reserved
	require.Equal(t, 3000, b.tokenBudget.AvailableTokens)
}

func TestEstimateTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{name: "empty", text: "", expected: 0},
		{name: "short", text: "hello", expected: 2},
		{name: "medium", text: "hello world test", expected: 6},
		{name: "long", text: strings.Repeat("a", 300), expected: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokens(tt.text)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestGetFilePriority removed - functionality moved to collector package

// TestSortFilesByPriority removed - functionality moved to collector package

// Test smart diff truncation logic
func TestSmartTruncateDiff(t *testing.T) {
	t.Parallel()

	b := NewBuilder("en", 0)

	t.Run("small_diff_no_truncation", func(t *testing.T) {
		diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
		result := b.smartTruncateDiff(diff, 100)
		require.Equal(t, diff, result)
	})

	t.Run("large_diff_truncation", func(t *testing.T) {
		lines := make([]string, 100)
		for i := 0; i < 100; i++ {
			lines[i] = fmt.Sprintf("line %d", i)
		}
		largeDiff := strings.Join(lines, "\n")

		result := b.smartTruncateDiff(largeDiff, 20) // very small token limit

		require.Contains(t, result, "line 0")         // should contain head
		require.Contains(t, result, "line 99")        // should contain tail
		require.Contains(t, result, "Diff truncated") // should contain truncation marker
		require.Less(t, len(result), len(largeDiff))  // result should be shorter
	})

	t.Run("small_line_count", func(t *testing.T) {
		diff := "line1\nline2\nline3"
		result := b.smartTruncateDiff(diff, 1) // very small token limit
		require.Equal(t, diff, result)         // less than 20 lines, no truncation
	})
}

// Mock collector for testing
type mockCollector struct {
	summary     *gitinfo.FileStatusSummary
	diff        string
	commits     []string
	shouldError bool
}

func (m *mockCollector) FileStatusSummary(ctx context.Context) (*gitinfo.FileStatusSummary, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock error")
	}
	return m.summary, nil
}

func (m *mockCollector) ComprehensiveDiff(ctx context.Context) (string, error) {
	if m.shouldError {
		return "", fmt.Errorf("mock error")
	}
	return m.diff, nil
}

func (m *mockCollector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock error")
	}
	return m.commits, nil
}

func (m *mockCollector) BranchName(ctx context.Context) (string, error) {
	if m.shouldError {
		return "", fmt.Errorf("mock error")
	}
	return "main", nil
}

func (m *mockCollector) ChangedFiles(ctx context.Context) ([]string, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock error")
	}
	return []string{}, nil
}

// Test BuildUserPromptWithBudget with various scenarios
func TestBuildUserPromptWithBudget(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		b := NewBuilderWithTokenBudget("en", 0, 8000)

		collector := &mockCollector{
			summary: &gitinfo.FileStatusSummary{
				BranchName: "feature/test",
				Files: []gitinfo.FileStatus{
					{Path: "main.go", IndexStatus: 'A'},
					{Path: "test.js", IndexStatus: 'M'},
				},
			},
			diff:    "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")",
			commits: []string{"feat: add feature", "fix: bug"},
		}

		userPrompt, err := b.BuildUserPromptWithBudget(context.Background(), collector, "seed text")
		require.NoError(t, err)

		require.Contains(t, userPrompt, "Seed: seed text")
		require.Contains(t, userPrompt, "Branch: feature/test")
		require.Contains(t, userPrompt, "Summary of Staged Files:")
		require.Contains(t, userPrompt, "A: main.go")
		require.Contains(t, userPrompt, "M: test.js")
		require.Contains(t, userPrompt, "Recent commits:")
		require.Contains(t, userPrompt, "feat: add feature")
		require.Contains(t, userPrompt, "Git diff")
		require.Contains(t, userPrompt, "fmt.Println")
	})

	t.Run("collector_error", func(t *testing.T) {
		b := NewBuilder("en", 0)
		collector := &mockCollector{shouldError: true}

		_, err := b.BuildUserPromptWithBudget(context.Background(), collector, "seed")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get file status summary")
	})

	t.Run("no_changes", func(t *testing.T) {
		b := NewBuilder("en", 0)
		collector := &mockCollector{
			summary: &gitinfo.FileStatusSummary{BranchName: "", Files: []gitinfo.FileStatus{}},
			diff:    "",
			commits: []string{},
		}

		userPrompt, err := b.BuildUserPromptWithBudget(context.Background(), collector, "")
		require.NoError(t, err)
		require.Equal(t, "No changes detected.", userPrompt)
	})
}
