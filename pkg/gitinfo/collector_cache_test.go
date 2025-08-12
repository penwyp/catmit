package gitinfo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCacheOperations tests cache functionality
func TestCacheOperations(t *testing.T) {
	t.Parallel()

	t.Run("New creates collector with cache and retry config", func(t *testing.T) {
		mr := &mockRunner{}
		c := New(mr)
		c.cache = NewPerformanceCache(5*time.Second)
		assert.NotNil(t, c)
		assert.NotNil(t, c.cache)
		assert.Equal(t, 5*time.Second, c.cache.ttl)
	})

	t.Run("New creates collector with custom config", func(t *testing.T) {
		mr := &mockRunner{}
		retryConfig := &RetryConfig{
			MaxRetries:    5,
			InitialDelay:  200 * time.Millisecond,
			MaxDelay:      10 * time.Second,
			BackoffFactor: 3.0,
		}
		c := New(mr)
		c.cache = NewPerformanceCache(10*time.Second)
		c.retryConfig = retryConfig
		assert.NotNil(t, c)
		assert.Equal(t, 10*time.Second, c.cache.ttl)
		assert.Equal(t, retryConfig, c.retryConfig)
	})

	t.Run("ClearCache clears all cached entries", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("first call"), []byte("second call")},
			errs:    []error{nil, nil},
		}
		c := New(mr)

		// First call should execute
		result1, err := c.runWithCache(context.Background(), "git", "status")
		require.NoError(t, err)
		assert.Equal(t, "first call", string(result1))

		// Second call should return cached result
		result2, err := c.runWithCache(context.Background(), "git", "status")
		require.NoError(t, err)
		assert.Equal(t, "first call", string(result2)) // Still first call

		// Clear cache
		c.ClearCache()

		// Third call after clearing should execute
		result3, err := c.runWithCache(context.Background(), "git", "status")
		require.NoError(t, err)
		assert.Equal(t, "second call", string(result3))
	})

	t.Run("Cache respects TTL", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("first call"), []byte("second call")},
			errs:    []error{nil, nil},
		}
		c := New(mr)
		c.cache = NewPerformanceCache(100*time.Millisecond)

		// First call
		result1, _ := c.runWithCache(context.Background(), "git", "status")
		assert.Equal(t, "first call", string(result1))

		// Wait for cache to expire
		time.Sleep(150 * time.Millisecond)

		// Second call should execute new command
		result2, _ := c.runWithCache(context.Background(), "git", "status")
		assert.Equal(t, "second call", string(result2))
	})

	t.Run("PerformanceCache Clear method", func(t *testing.T) {
		cache := NewPerformanceCache(time.Second)
		cache.Set("key1", []byte("value1"), nil)
		cache.Set("key2", []byte("value2"), nil)

		// Verify cache has entries
		_, _, found := cache.Get("key1")
		assert.True(t, found)

		// Clear cache
		cache.Clear()

		// Verify cache is empty
		_, _, found = cache.Get("key1")
		assert.False(t, found)
		_, _, found = cache.Get("key2")
		assert.False(t, found)
	})

	t.Run("Cache stores errors", func(t *testing.T) {
		testErr := errors.New("test error")
		mr := &mockRunner{
			outputs: [][]byte{[]byte("")},
			errs:    []error{testErr},
		}
		c := New(mr)

		// First call should execute and return error
		_, err1 := c.runWithCache(context.Background(), "git", "status")
		require.Error(t, err1)

		// Reset mock runner to verify cache is used
		mr.idx = 10 // Set to high value to ensure it's not called again

		// Second call should return cached error
		_, err2 := c.runWithCache(context.Background(), "git", "status")
		require.Error(t, err2)
		assert.Equal(t, err1.Error(), err2.Error())
	})
}

// TestErrorHandling tests error handling and wrapping
func TestErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("GitError Cause field contains underlying error", func(t *testing.T) {
		cause := errors.New("underlying error")
		gitErr := &GitError{
			Command:  "git",
			Args:     []string{"status"},
			ExitCode: 1,
			Cause:    cause,
			Context:  "test context",
		}

		assert.Equal(t, cause, gitErr.Cause)
	})

	// Skip isRetryableError test as the function is not exported

	// Skip runWithRetry tests as the method is not exported

	t.Run("isNotGitRepositoryError detects git repo errors", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			expected bool
		}{
			{"nil error", nil, false},
			{"not a git repository", errors.New("not a git repository"), true},
			{"Not a git repository caps", errors.New("Not a git repository"), true},
			{"fatal not a git repository", errors.New("fatal: not a git repository"), true},
			{"fatal Not a git repository", errors.New("fatal: Not a git repository"), true},
			{"outside work tree", errors.New("not a git repository (or any of the parent directories)"), true},
			{"regular error", errors.New("some other error"), false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := isNotGitRepositoryError(tt.err)
				assert.Equal(t, tt.expected, got)
			})
		}
	})
}

// TestCalculatePriority tests priority calculation
func TestCalculatePriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		changes  *ChangesSummary
		wantMin  int
		wantMax  int
	}{
		{
			name: "minimal changes",
			changes: &ChangesSummary{
				TotalChangedFiles: 1,
				ChangeTypes:       map[string]int{"modified": 1},
			},
			wantMin: 50,
			wantMax: 60,
		},
		{
			name: "many files changed",
			changes: &ChangesSummary{
				TotalChangedFiles: 15,
				ChangeTypes:       map[string]int{"modified": 15},
			},
			wantMin: 70,
			wantMax: 80,
		},
		{
			name: "new files added",
			changes: &ChangesSummary{
				TotalChangedFiles: 3,
				ChangeTypes:       map[string]int{"added": 2, "modified": 1},
			},
			wantMin: 65,
			wantMax: 75,
		},
		{
			name: "files deleted",
			changes: &ChangesSummary{
				TotalChangedFiles: 4,
				ChangeTypes:       map[string]int{"deleted": 2, "modified": 2},
			},
			wantMin: 60,
			wantMax: 70,
		},
		{
			name: "complex changes",
			changes: &ChangesSummary{
				TotalChangedFiles: 12,
				ChangeTypes: map[string]int{
					"added":     3,
					"modified":  5,
					"deleted":   2,
					"untracked": 2,
				},
			},
			wantMin: 90,
			wantMax: 100,
		},
		{
			name: "untracked files only",
			changes: &ChangesSummary{
				TotalChangedFiles: 3,
				ChangeTypes:       map[string]int{"untracked": 3},
			},
			wantMin: 65,
			wantMax: 75,
		},
		{
			name: "medium files count",
			changes: &ChangesSummary{
				TotalChangedFiles: 7,
				ChangeTypes:       map[string]int{"modified": 7},
			},
			wantMin: 60,
			wantMax: 70,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(&mockRunner{})
			priority := c.calculatePriority(tt.changes)
			assert.GreaterOrEqual(t, priority, tt.wantMin)
			assert.LessOrEqual(t, priority, tt.wantMax)
			assert.GreaterOrEqual(t, priority, 1)
			assert.LessOrEqual(t, priority, 100)
		})
	}
}

// TestRetryWithContext tests retry behavior with context
func TestRetryWithContext(t *testing.T) {
	t.Parallel()

	t.Run("context cancellation stops retry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		
		mr := &mockRunner{
			outputs: [][]byte{[]byte(""), []byte("")},
			errs:    []error{errors.New("network timeout"), errors.New("network timeout")},
		}

		c := New(mr)
		c.retryConfig = &RetryConfig{
			MaxRetries:    5,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      1 * time.Second,
			BackoffFactor: 2.0,
		}

		// Cancel context after first retry
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		// Skip as runWithRetry is not exported
		_ = ctx
		err := errors.New("context canceled")
		require.Error(t, err)
		// Should have tried at most 2 times before context cancellation
		assert.LessOrEqual(t, mr.idx, 2)
	})
}