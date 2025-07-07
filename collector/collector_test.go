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

// 测试新增的文件过滤功能
func TestShouldIgnoreFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		// 正常源码文件
		{name: "go_file", filePath: "main.go", expected: false},
		{name: "js_file", filePath: "src/index.js", expected: false},
		{name: "py_file", filePath: "app.py", expected: false},
		
		// 锁文件
		{name: "package_lock", filePath: "package-lock.json", expected: true},
		{name: "yarn_lock", filePath: "yarn.lock", expected: true},
		{name: "go_sum", filePath: "go.sum", expected: true},
		{name: "go_mod", filePath: "go.mod", expected: true},
		
		// 构建产物目录
		{name: "dist_file", filePath: "dist/bundle.js", expected: true},
		{name: "build_file", filePath: "build/app.exe", expected: true},
		{name: "node_modules", filePath: "node_modules/express/index.js", expected: true},
		{name: "vendor_file", filePath: "vendor/lib.go", expected: true},
		
		// 二进制文件
		{name: "exe_file", filePath: "app.exe", expected: true},
		{name: "dll_file", filePath: "lib.dll", expected: true},
		{name: "so_file", filePath: "lib.so", expected: true},
		{name: "jpg_file", filePath: "image.jpg", expected: true},
		{name: "png_file", filePath: "logo.png", expected: true},
		{name: "pdf_file", filePath: "doc.pdf", expected: true},
		
		// 临时文件
		{name: "log_file", filePath: "app.log", expected: true},
		{name: "tmp_file", filePath: "temp.tmp", expected: true},
		{name: "bak_file", filePath: "config.bak", expected: true},
		{name: "swp_file", filePath: ".file.swp", expected: true},
		
		// 嵌套路径测试
		{name: "nested_build", filePath: "frontend/build/static/js/main.js", expected: true},
		{name: "nested_source", filePath: "src/components/Button.tsx", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldIgnoreFile(tt.filePath)
			require.Equal(t, tt.expected, result, "File: %s", tt.filePath)
		})
	}
}

// 测试git status解析功能
func TestParseGitStatusPorcelain(t *testing.T) {
	t.Parallel()

	t.Run("empty_output", func(t *testing.T) {
		summary, err := parseGitStatusPorcelain("")
		require.NoError(t, err)
		require.Empty(t, summary.Files)
		require.Empty(t, summary.BranchName)
	})

	t.Run("branch_only", func(t *testing.T) {
		output := "## main"
		summary, err := parseGitStatusPorcelain(output)
		require.NoError(t, err)
		require.Equal(t, "main", summary.BranchName)
		require.Empty(t, summary.Files)
	})

	t.Run("branch_with_tracking", func(t *testing.T) {
		output := "## feature/test...origin/feature/test"
		summary, err := parseGitStatusPorcelain(output)
		require.NoError(t, err)
		require.Equal(t, "feature/test", summary.BranchName)
		require.Empty(t, summary.Files)
	})

	t.Run("modified_files", func(t *testing.T) {
		output := `## main
M  file1.go
A  file2.js
D  file3.txt`
		summary, err := parseGitStatusPorcelain(output)
		require.NoError(t, err)
		require.Equal(t, "main", summary.BranchName)
		require.Len(t, summary.Files, 3)
		
		require.Equal(t, 'M', summary.Files[0].IndexStatus)
		require.Equal(t, "file1.go", summary.Files[0].Path)
		
		require.Equal(t, 'A', summary.Files[1].IndexStatus)
		require.Equal(t, "file2.js", summary.Files[1].Path)
		
		require.Equal(t, 'D', summary.Files[2].IndexStatus)
		require.Equal(t, "file3.txt", summary.Files[2].Path)
	})

	t.Run("renamed_file", func(t *testing.T) {
		output := `## main
R  old.txt -> new.txt`
		summary, err := parseGitStatusPorcelain(output)
		require.NoError(t, err)
		require.Len(t, summary.Files, 1)
		
		file := summary.Files[0]
		require.Equal(t, 'R', file.IndexStatus)
		require.True(t, file.IsRenamed)
		require.Equal(t, "old.txt", file.OldPath)
		require.Equal(t, "new.txt", file.Path)
	})

	t.Run("filtered_files", func(t *testing.T) {
		output := `## main
M  main.go
A  package-lock.json
D  dist/bundle.js`
		summary, err := parseGitStatusPorcelain(output)
		require.NoError(t, err)
		require.Equal(t, "main", summary.BranchName)
		// 只有main.go应该被保留，其他两个被过滤
		require.Len(t, summary.Files, 1)
		require.Equal(t, "main.go", summary.Files[0].Path)
	})
}

// 测试文件优先级排序
func TestSortFilesByPriority(t *testing.T) {
	t.Parallel()

	files := []FileStatus{
		{Path: "README.md", IndexStatus: 'M'},      // 修改的文档文件
		{Path: "main.go", IndexStatus: 'A'},        // 新增的Go文件
		{Path: "style.css", IndexStatus: 'M'},      // 修改的CSS文件
		{Path: "config.json", IndexStatus: 'A'},    // 新增的配置文件
		{Path: "old.txt", IndexStatus: 'D'},        // 删除的文件
		{Path: "test/unit_test.go", IndexStatus: 'M'}, // 修改的测试文件
	}

	sorted := sortFilesByPriority(files)
	
	// 验证排序结果：新增的Go文件应该在最前面
	require.Equal(t, "main.go", sorted[0].Path)
	require.Equal(t, 'A', sorted[0].IndexStatus)
	
	// 新增的配置文件应该在第二
	require.Equal(t, "config.json", sorted[1].Path)
	require.Equal(t, 'A', sorted[1].IndexStatus)
	
	// 验证最后几个是低优先级的文件（删除文件和测试文件）
	lastTwoFiles := []string{sorted[len(sorted)-2].Path, sorted[len(sorted)-1].Path}
	require.Contains(t, lastTwoFiles, "old.txt")      // 删除文件应该在最后几个
	require.Contains(t, lastTwoFiles, "test/unit_test.go") // 测试文件也应该在最后几个
}

// 测试FileStatusSummary方法
func TestCollector_FileStatusSummary(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		output := `## main...origin/main
M  main.go
A  new.txt
D  old.txt`
		
		mr := &mockRunner{
			outputs: [][]byte{[]byte(output)},
			errs:    []error{nil},
		}
		
		c := New(mr)
		summary, err := c.FileStatusSummary(context.Background())
		require.NoError(t, err)
		require.Equal(t, "main", summary.BranchName)
		require.Len(t, summary.Files, 3)
	})

	t.Run("git_command_fails", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("")},
			errs:    []error{errors.New("git status failed")},
		}
		
		c := New(mr)
		_, err := c.FileStatusSummary(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "git status --porcelain -b failed")
	})
}
