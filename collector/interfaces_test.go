package collector

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Enhanced mockRunner with call counting for performance tests
// We need to extend the existing mockRunner from collector_test.go
type enhancedMockRunner struct {
	outputs   [][]byte
	errs      []error
	idx       int
	callCount int
}

func (m *enhancedMockRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	m.callCount++
	if m.idx >= len(m.outputs) {
		return nil, errors.New("unexpected call")
	}
	out := m.outputs[m.idx]
	err := m.errs[m.idx]
	m.idx++
	return out, err
}

// TestNewInterfaceImplementations verifies that the new focused interfaces work correctly
func TestNewInterfaceImplementations(t *testing.T) {
	t.Parallel()

	// Test GitReader interface
	t.Run("GitReader", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte("diff --git a/file.txt b/file.txt"),  // StagedDiff
				[]byte("unstaged changes"),                   // UnstagedDiff
				[]byte("M  file.txt"),                        // GitStatus
				[]byte("feat: add feature\nfix: bug fix"),    // RecentCommits
				[]byte("main"),                               // BranchName
			},
			errs: []error{nil, nil, nil, nil, nil},
		}

		collector := New(mr)
		var gitReader GitReader = collector

		// Test StagedDiff
		staged, err := gitReader.StagedDiff(context.Background())
		require.NoError(t, err)
		assert.Contains(t, staged, "diff --git")

		// Test UnstagedDiff
		unstaged, err := gitReader.UnstagedDiff(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "unstaged changes", unstaged)

		// Test GitStatus
		status, err := gitReader.GitStatus(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "M  file.txt", status)

		// Test RecentCommits
		commits, err := gitReader.RecentCommits(context.Background(), 2)
		require.NoError(t, err)
		assert.Len(t, commits, 2)
		assert.Equal(t, "feat: add feature", commits[0])

		// Test BranchName
		branch, err := gitReader.BranchName(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "main", branch)
	})

	// Test ChangeAnalyzer interface
	t.Run("ChangeAnalyzer", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte("## main\nM  file.txt\nA  new_file.go\n"), // FileStatusSummary (AnalyzeChanges)
				[]byte(""),                                       // UntrackedFiles (AnalyzeChanges)
				[]byte("## main\nM  file.txt\nA  new_file.go\n"), // FileStatusSummary (GetPriorityFiles)
			},
			errs: []error{nil, nil, nil},
		}

		collector := New(mr)
		var analyzer ChangeAnalyzer = collector

		// Test AnalyzeChanges
		changes, err := analyzer.AnalyzeChanges(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, changes)
		assert.True(t, changes.HasStagedChanges)
		assert.Equal(t, 2, changes.TotalFiles)
		assert.Contains(t, changes.ChangeTypes, "modified")
		assert.Contains(t, changes.ChangeTypes, "added")
		assert.NotEmpty(t, changes.PrimaryChangeType)

		// Test GetPriorityFiles
		files, err := analyzer.GetPriorityFiles(context.Background())
		require.NoError(t, err)
		assert.Len(t, files, 2)
		// Files should be sorted by priority
		assert.Equal(t, 'A', files[0].IndexStatus) // Added files have higher priority
	})

	// Test FileContentProvider interface
	t.Run("FileContentProvider", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte("untracked_file.txt\nnew_script.go\n"),  // UntrackedFiles
				[]byte("package main\n\nfunc main() {\n\t// Hello world\n}"), // UntrackedFileContent
			},
			errs: []error{nil, nil},
		}

		collector := New(mr)
		var provider FileContentProvider = collector

		// Test UntrackedFiles
		files, err := provider.UntrackedFiles(context.Background())
		require.NoError(t, err)
		assert.Len(t, files, 2)
		assert.Contains(t, files, "untracked_file.txt")
		assert.Contains(t, files, "new_script.go")

		// Test UntrackedFileContent
		content, err := provider.UntrackedFileContent(context.Background(), "new_script.go")
		require.NoError(t, err)
		assert.Contains(t, content, "package main")
		assert.Contains(t, content, "func main()")
	})

	// Test LegacyCollectorInterface for backward compatibility
	t.Run("LegacyCollectorInterface", func(t *testing.T) {
		// Test legacy interface methods individually with fresh collectors for each test
		// to avoid complex mock ordering issues
		
		// Test RecentCommits
		mr1 := &mockRunner{outputs: [][]byte{[]byte("feat: add feature")}, errs: []error{nil}}
		c1 := New(mr1)
		commits, err := c1.RecentCommits(context.Background(), 1)
		require.NoError(t, err)
		assert.Len(t, commits, 1)

		// Test Diff (uses ComprehensiveDiff)
		mr2 := &mockRunner{
			outputs: [][]byte{[]byte("diff content"), []byte(""), []byte("")},
			errs: []error{nil, nil, nil},
		}
		c2 := New(mr2)
		diff, err := c2.Diff(context.Background())
		require.NoError(t, err)
		assert.Contains(t, diff, "diff content")

		// Test BranchName
		mr3 := &mockRunner{outputs: [][]byte{[]byte("main")}, errs: []error{nil}}
		c3 := New(mr3)
		branch, err := c3.BranchName(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "main", branch)

		// Test ChangedFiles
		mr4 := &mockRunner{
			outputs: [][]byte{[]byte("file.txt"), []byte("")},
			errs: []error{nil, nil},
		}
		c4 := New(mr4)
		files, err := c4.ChangedFiles(context.Background())
		require.NoError(t, err)
		assert.Len(t, files, 1)
		assert.Equal(t, "file.txt", files[0])

		// Test FileStatusSummary
		mr5 := &mockRunner{outputs: [][]byte{[]byte("## main\nM  file.txt")}, errs: []error{nil}}
		c5 := New(mr5)
		summary, err := c5.FileStatusSummary(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "main", summary.BranchName)
		assert.Len(t, summary.Files, 1)
	})
}

// TestInterfaceSegregation verifies that interfaces are properly segregated
func TestInterfaceSegregation(t *testing.T) {
	t.Parallel()

	collector := New(&mockRunner{})

	// Test that we can use interfaces independently
	var gitReader GitReader = collector
	var analyzer ChangeAnalyzer = collector
	var provider FileContentProvider = collector
	var legacy LegacyCollectorInterface = collector

	// These should compile without error, demonstrating interface segregation
	_ = gitReader
	_ = analyzer
	_ = provider
	_ = legacy
}

// TestHelperFunctions tests the new helper functions
func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	t.Run("determinePrimaryChangeType", func(t *testing.T) {
		tests := []struct {
			name      string
			changes   map[string]int
			expected  string
		}{
			{
				name:     "feat for added files",
				changes:  map[string]int{"added": 2, "modified": 1},
				expected: "feat",
			},
			{
				name:     "chore for deleted files",
				changes:  map[string]int{"deleted": 1, "modified": 1},
				expected: "chore",
			},
			{
				name:     "refactor for renamed files",
				changes:  map[string]int{"renamed": 1, "modified": 1},
				expected: "refactor",
			},
			{
				name:     "fix for modified files",
				changes:  map[string]int{"modified": 2},
				expected: "fix",
			},
			{
				name:     "default fallback",
				changes:  map[string]int{},
				expected: "chore",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := determinePrimaryChangeType(tt.changes)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("analyzeAffectedAreas", func(t *testing.T) {
		files := []FileStatus{
			{Path: "src/main.go", IndexStatus: 'M'},
			{Path: "src/utils/helper.go", IndexStatus: 'A'},
			{Path: "test/main_test.go", IndexStatus: 'M'},
			{Path: "README.md", IndexStatus: 'M'},
		}

		areas := analyzeAffectedAreas(files)
		assert.Len(t, areas, 3)
		assert.Contains(t, areas, "src")
		assert.Contains(t, areas, "test")
		assert.Contains(t, areas, "root")
	})
}

// Phase 3 Performance Benchmarks and Tests
func TestPerformanceOptimizations(t *testing.T) {
	t.Parallel()

	t.Run("CachePerformance", func(t *testing.T) {
		mr := &enhancedMockRunner{
			outputs: [][]byte{
				[]byte("branch-output"),
				[]byte("branch-output"), // Same output should be cached
			},
			errs: []error{nil, nil},
		}

		collector := New(mr)
		
		// First call should hit the runner
		result1, err1 := collector.BranchName(context.Background())
		require.NoError(t, err1)
		assert.Equal(t, "branch-output", result1)
		
		// Second call should use cache (runner should not be called again)
		result2, err2 := collector.BranchName(context.Background())
		require.NoError(t, err2)
		assert.Equal(t, "branch-output", result2)
		
		// Verify cache is working by checking mock runner call count
		assert.Equal(t, 1, mr.callCount) // Only one call should have been made
	})
	
	t.Run("BatchOperations", func(t *testing.T) {
		mr := &enhancedMockRunner{
			outputs: [][]byte{
				[]byte("staged-file.txt"),
				[]byte("untracked-file.txt"),
			},
			errs: []error{nil, nil},
		}

		collector := New(mr)
		
		// Test batched operations in ChangedFiles
		files, err := collector.ChangedFiles(context.Background())
		require.NoError(t, err)
		assert.Contains(t, files, "staged-file.txt")
		assert.Contains(t, files, "untracked-file.txt")
		
		// Verify both commands were called
		assert.Equal(t, 2, mr.callCount)
	})
	
	t.Run("MemoryOptimization", func(t *testing.T) {
		// Test optimizeStringSlice function
		input := []string{"", "file1.txt", "file1.txt", "  file2.txt  ", "", "file3.txt"}
		result := optimizeStringSlice(input)
		
		expected := []string{"file1.txt", "file2.txt", "file3.txt"}
		assert.Equal(t, expected, result)
		assert.Len(t, result, 3) // Duplicates and empty strings removed
	})
	
	t.Run("ErrorHandling", func(t *testing.T) {
		mr := &enhancedMockRunner{
			outputs: [][]byte{nil},
			errs:    []error{fmt.Errorf("network timeout")},
		}

		collector := New(mr)
		
		// Test retry logic with retryable error
		_, err := collector.runWithRetry(context.Background(), &RetryConfig{
			MaxRetries: 2,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay: 10 * time.Millisecond,
			BackoffFactor: 2.0,
		}, "git", "status")
		
		require.Error(t, err)
		var gitErr *GitError
		assert.True(t, errors.As(err, &gitErr))
		assert.Equal(t, "git", gitErr.Command)
		assert.Contains(t, gitErr.Context, "failed after")
	})
}

// Benchmarks for performance testing
func BenchmarkCollectorOperations(b *testing.B) {
	mr := &mockRunner{
		outputs: [][]byte{
			[]byte("main"),
			[]byte("feat: test commit"),
			[]byte("diff --git a/file.txt b/file.txt"),
			[]byte("M  file.txt"),
		},
		errs: []error{nil, nil, nil, nil},
	}

	collector := New(mr)
	ctx := context.Background()

	b.Run("BranchName", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := collector.BranchName(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RecentCommits", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := collector.RecentCommits(ctx, 10)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ComprehensiveDiff", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := collector.ComprehensiveDiff(ctx)
			if err != nil && !errors.Is(err, ErrNoDiff) {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("AnalyzeChanges", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := collector.AnalyzeChanges(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark cache performance
func BenchmarkCachePerformance(b *testing.B) {
	cache := NewPerformanceCache(1 * time.Minute)
	
	b.Run("CacheSet", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Set(fmt.Sprintf("key-%d", i), []byte("value"), nil)
		}
	})
	
	b.Run("CacheGet", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < 1000; i++ {
			cache.Set(fmt.Sprintf("key-%d", i), []byte("value"), nil)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _ = cache.Get(fmt.Sprintf("key-%d", i%1000))
		}
	})
}

// Benchmark memory optimization
func BenchmarkMemoryOptimization(b *testing.B) {
	// Create a large slice with duplicates and empty strings
	input := make([]string, 10000)
	for i := 0; i < len(input); i++ {
		switch i % 5 {
		case 0:
			input[i] = ""
		case 1:
			input[i] = "file1.txt"
		case 2:
			input[i] = "file2.txt"
		case 3:
			input[i] = "  file3.txt  "
		case 4:
			input[i] = "file1.txt" // Duplicate
		}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = optimizeStringSlice(input)
	}
}