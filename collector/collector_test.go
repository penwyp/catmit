package collector

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockRunner 用于单元测试，按调用顺序返回预设结果。
type mockRunner struct {
	outputs [][]byte
	errs    []error
	idx     int
}

func (m *mockRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	if m.idx >= len(m.outputs) {
		return nil, errors.New("unexpected call")
	}
	out := m.outputs[m.idx]
	err := m.errs[m.idx]
	m.idx++
	return out, err
}

func TestCollector_RecentCommits(t *testing.T) {
	t.Parallel()

	mr := &mockRunner{
		outputs: [][]byte{[]byte("feat: add feature\nfix: bug fix\nchore: update deps")},
		errs:    []error{nil},
	}

	c := New(mr)

	commits, err := c.RecentCommits(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, commits, 3)
	require.Equal(t, "feat: add feature", commits[0])
}

func TestCollector_Diff(t *testing.T) {
	t.Parallel()

	t.Run("has_diff", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("diff --git a/file.txt b/file.txt"), []byte("")},
			errs:    []error{nil, nil},
		}
		c := New(mr)
		diff, err := c.Diff(context.Background())
		require.NoError(t, err)
		require.Contains(t, diff, "diff --git")
	})

	t.Run("no_diff", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte(""), []byte(""), []byte("")},
			errs:    []error{nil, nil, nil},
		}
		c := New(mr)
		_, err := c.Diff(context.Background())
		require.ErrorIs(t, err, ErrNoDiff)
	})

	t.Run("no_diff_but_git_status_has_changes", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte(""), []byte(""), []byte("M  file.txt\nA  newfile.txt")},
			errs:    []error{nil, nil, nil},
		}
		c := New(mr)
		diff, err := c.Diff(context.Background())
		require.NoError(t, err)
		require.Equal(t, "M  file.txt\nA  newfile.txt", diff)
	})

	t.Run("git_status_command_fails", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte(""), []byte(""), []byte("")},
			errs:    []error{nil, nil, errors.New("git status failed")},
		}
		c := New(mr)
		_, err := c.Diff(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "git status --porcelain failed")
	})
}

func TestCollector_ChangedFiles(t *testing.T) {
	t.Parallel()

	t.Run("changed_files_with_untracked", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("a.go\n"), []byte("b.go\n")},
			errs:    []error{nil, nil},
		}
		c := New(mr)
		files, err := c.ChangedFiles(context.Background())
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a.go", "b.go"}, files)
	})
}

func TestCollector_BranchName(t *testing.T) {
	t.Parallel()

	t.Run("valid_branch_name", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("main\n")},
			errs:    []error{nil},
		}
		c := New(mr)
		branch, err := c.BranchName(context.Background())
		require.NoError(t, err)
		require.Equal(t, "main", branch)
	})

	t.Run("feature_branch_name", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("feature/awesome-feature\n")},
			errs:    []error{nil},
		}
		c := New(mr)
		branch, err := c.BranchName(context.Background())
		require.NoError(t, err)
		require.Equal(t, "feature/awesome-feature", branch)
	})

	t.Run("invalid_branch_name", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("main;rm -rf /\n")},
			errs:    []error{nil},
		}
		c := New(mr)
		_, err := c.BranchName(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid branch name format")
	})

	t.Run("git_command_error", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("")},
			errs:    []error{errors.New("git error")},
		}
		c := New(mr)
		_, err := c.BranchName(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "git rev-parse failed")
	})
}

func TestCollector_RecentCommits_Security(t *testing.T) {
	t.Parallel()

	t.Run("negative_n", func(t *testing.T) {
		c := New(&mockRunner{})
		_, err := c.RecentCommits(context.Background(), -1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "n must be positive")
	})

	t.Run("zero_n", func(t *testing.T) {
		c := New(&mockRunner{})
		_, err := c.RecentCommits(context.Background(), 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "n must be positive")
	})

	t.Run("too_large_n", func(t *testing.T) {
		c := New(&mockRunner{})
		_, err := c.RecentCommits(context.Background(), 1001)
		require.Error(t, err)
		require.Contains(t, err.Error(), "n too large")
	})
}

func TestSanitizeOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean_string",
			input:    "normal-file.txt",
			expected: "normal-file.txt",
		},
		{
			name:     "with_semicolon",
			input:    "file;rm -rf /",
			expected: "filerm -rf /",
		},
		{
			name:     "with_control_chars",
			input:    "file\x00\x1f\x7f",
			expected: "file",
		},
		{
			name:     "with_pipe",
			input:    "file|dangerous",
			expected: "filedangerous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeOutput(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
