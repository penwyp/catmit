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
			// ComprehensiveDiff calls: StagedDiff, UnstagedDiff, UntrackedFiles
			outputs: [][]byte{
				[]byte("diff --git a/file.txt b/file.txt"), // StagedDiff
				[]byte(""),                                 // UnstagedDiff  
				[]byte(""),                                 // UntrackedFiles
			},
			errs: []error{nil, nil, nil},
		}
		c := New(mr)
		diff, err := c.Diff(context.Background())
		require.NoError(t, err)
		require.Contains(t, diff, "diff --git")
	})

	t.Run("no_diff", func(t *testing.T) {
		mr := &mockRunner{
			// ComprehensiveDiff calls: StagedDiff, UnstagedDiff, UntrackedFiles, GitStatus (fallback)
			// Then falls back to CombinedDiff which calls: staged diff, unstaged diff
			outputs: [][]byte{
				[]byte(""), // StagedDiff (returns ErrNoDiff)
				[]byte(""), // UnstagedDiff 
				[]byte(""), // UntrackedFiles
				[]byte(""), // GitStatus (fallback)
				[]byte(""), // CombinedDiff -> staged diff (fallback)
				[]byte(""), // CombinedDiff -> unstaged diff (fallback)
				[]byte(""), // CombinedDiff -> git status (fallback)
			},
			errs: []error{ErrNoDiff, nil, nil, nil, nil, nil, nil}, // StagedDiff returns ErrNoDiff
		}
		c := New(mr)
		_, err := c.Diff(context.Background())
		require.ErrorIs(t, err, ErrNoDiff)
	})

	t.Run("no_diff_but_git_status_has_changes", func(t *testing.T) {
		mr := &mockRunner{
			// ComprehensiveDiff calls: StagedDiff, UnstagedDiff, UntrackedFiles, GitStatus (fallback)
			outputs: [][]byte{
				[]byte(""),                               // StagedDiff (returns ErrNoDiff)
				[]byte(""),                               // UnstagedDiff
				[]byte(""),                               // UntrackedFiles  
				[]byte("M  file.txt\nA  newfile.txt"),   // GitStatus (fallback)
			},
			errs: []error{ErrNoDiff, nil, nil, nil}, // StagedDiff returns ErrNoDiff
		}
		c := New(mr)
		diff, err := c.Diff(context.Background())
		require.NoError(t, err)
		require.Equal(t, "M  file.txt\nA  newfile.txt", diff)
	})

	t.Run("git_status_command_fails", func(t *testing.T) {
		mr := &mockRunner{
			// ComprehensiveDiff calls: StagedDiff, UnstagedDiff, UntrackedFiles, GitStatus (fails)
			// Then falls back to CombinedDiff which calls: staged diff, unstaged diff, git status (fails)
			outputs: [][]byte{
				[]byte(""), // StagedDiff (returns ErrNoDiff)
				[]byte(""), // UnstagedDiff 
				[]byte(""), // UntrackedFiles
				nil,        // GitStatus (fails)
				[]byte(""), // CombinedDiff -> staged diff (fallback)
				[]byte(""), // CombinedDiff -> unstaged diff (fallback)
				nil,        // CombinedDiff -> git status (fails)
			},
			errs: []error{ErrNoDiff, nil, nil, errors.New("git status --porcelain failed"), nil, nil, errors.New("git status --porcelain failed")},
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

// ============================================================================
// Phase 2: Enhanced Data Model Tests
// ============================================================================

// Test ComprehensiveDiff - the core fix for untracked files
func TestCollector_ComprehensiveDiff(t *testing.T) {
	t.Parallel()

	t.Run("with_untracked_files", func(t *testing.T) {
		// Mock responses for: staged diff, unstaged diff, ls-files, head
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte("diff --git a/main.go b/main.go\n+func main() {\n+  fmt.Println(\"hello\")\n+}"), // staged diff
				[]byte(""), // unstaged diff (empty)
				[]byte("newfile.txt"), // untracked files
				[]byte("This is a new file content"), // file content
			},
			errs: []error{nil, nil, nil, nil},
		}

		c := New(mr)
		diff, err := c.ComprehensiveDiff(context.Background())
		require.NoError(t, err)
		
		// Should contain staged diff
		require.Contains(t, diff, "diff --git a/main.go b/main.go")
		// Should contain untracked file as diff
		require.Contains(t, diff, "diff --git a/newfile.txt b/newfile.txt")
		require.Contains(t, diff, "new file mode 100644")
		require.Contains(t, diff, "+This is a new file content")
	})

	t.Run("no_changes", func(t *testing.T) {
		// Mock responses for: staged diff (empty), unstaged diff (empty), ls-files (empty), git status (empty)
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte(""), // staged diff
				[]byte(""), // unstaged diff
				[]byte(""), // untracked files
				[]byte(""), // git status
			},
			errs: []error{ErrNoDiff, nil, nil, nil},
		}

		c := New(mr)
		_, err := c.ComprehensiveDiff(context.Background())
		require.ErrorIs(t, err, ErrNoDiff)
	})

	t.Run("only_untracked_files", func(t *testing.T) {
		// Mock responses for: staged diff (empty), unstaged diff (empty), ls-files, head
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte(""), // staged diff
				[]byte(""), // unstaged diff
				[]byte("README.md\ndocs/api.md"), // untracked files
				[]byte("# New Project\nThis is a new project"), // README.md content
				[]byte("# API Documentation\nAPI endpoints"), // docs/api.md content
			},
			errs: []error{ErrNoDiff, nil, nil, nil, nil},
		}

		c := New(mr)
		diff, err := c.ComprehensiveDiff(context.Background())
		require.NoError(t, err)
		
		// Should contain both untracked files as diffs
		require.Contains(t, diff, "diff --git a/README.md b/README.md")
		require.Contains(t, diff, "diff --git a/docs/api.md b/docs/api.md")
		require.Contains(t, diff, "+# New Project")
		require.Contains(t, diff, "+# API Documentation")
	})
}

// Test UntrackedFileAsDiff method
func TestCollector_UntrackedFileAsDiff(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte("package main\n\nfunc main() {\n\tfmt.Println(\"Hello World\")\n}"),
			},
			errs: []error{nil},
		}

		c := New(mr)
		diff, err := c.UntrackedFileAsDiff(context.Background(), "main.go")
		require.NoError(t, err)

		// Check diff format
		require.Contains(t, diff, "diff --git a/main.go b/main.go")
		require.Contains(t, diff, "new file mode 100644")
		require.Contains(t, diff, "+package main")
		require.Contains(t, diff, "+func main() {")
	})

	t.Run("file_read_error", func(t *testing.T) {
		mr := &mockRunner{
			outputs: [][]byte{[]byte("")},
			errs:    []error{errors.New("file not found")},
		}

		c := New(mr)
		_, err := c.UntrackedFileAsDiff(context.Background(), "nonexistent.go")
		require.Error(t, err)
		require.Contains(t, err.Error(), "file not found")
	})
}

// Test enhanced AnalyzeChanges method
func TestCollector_AnalyzeChanges_Enhanced(t *testing.T) {
	t.Parallel()

	t.Run("with_untracked_files", func(t *testing.T) {
		// Mock git status output with some files
		gitStatusOutput := `## main
M  src/main.go
A  docs/README.md
D  old/deprecated.txt`
		
		// Mock untracked files
		untrackedOutput := "newfile.py\nconfig.json"
		
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte(gitStatusOutput), // FileStatusSummary
				[]byte(untrackedOutput), // UntrackedFiles
			},
			errs: []error{nil, nil},
		}

		c := New(mr)
		summary, err := c.AnalyzeChanges(context.Background())
		require.NoError(t, err)

		// Check basic fields
		require.True(t, summary.HasStagedChanges)
		require.True(t, summary.HasUntrackedFiles)
		require.Equal(t, 5, summary.TotalChangedFiles) // 3 tracked + 2 untracked
		
		// Check change types
		require.Equal(t, 1, summary.ChangeTypes["modified"])
		require.Equal(t, 1, summary.ChangeTypes["added"])
		require.Equal(t, 1, summary.ChangeTypes["deleted"])
		require.Equal(t, 2, summary.ChangeTypes["untracked"])
		
		// Check magnitude
		require.Equal(t, ChangeMagnitudeMedium, summary.Magnitude)
		
		// Check suggested prefix
		require.Equal(t, "feat", summary.SuggestedPrefix) // Should be feat due to untracked files
		
		// Check untracked files
		require.Len(t, summary.UntrackedFiles, 2)
		require.Equal(t, "newfile.py", summary.UntrackedFiles[0].Path)
		require.Equal(t, "config.json", summary.UntrackedFiles[1].Path)
		
		// Check priority sorting
		require.NotEmpty(t, summary.FilesByPriority)
	})

	t.Run("only_modifications", func(t *testing.T) {
		gitStatusOutput := `## main
M  src/bugfix.go
M  tests/bugfix_test.go`
		
		mr := &mockRunner{
			outputs: [][]byte{
				[]byte(gitStatusOutput), // FileStatusSummary
				[]byte(""), // UntrackedFiles (empty)
			},
			errs: []error{nil, nil},
		}

		c := New(mr)
		summary, err := c.AnalyzeChanges(context.Background())
		require.NoError(t, err)

		// Should suggest "fix" for modifications
		require.Equal(t, "fix", summary.SuggestedPrefix)
		require.Equal(t, ChangeMagnitudeSmall, summary.Magnitude)
		require.False(t, summary.HasUntrackedFiles)
	})
}

// Test enhanced file status functionality
func TestCollector_EnhanceFileStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		path            string
		expectedType    string
		expectedArea    string
	}{
		{"go_file", "main.go", "code", "root"},
		{"config_file", "config.json", "config", "root"},
		{"frontend_file", "src/components/Button.tsx", "frontend", "src"},
		{"test_file", "tests/unit_test.go", "test", "tests"},
		{"docs_file", "docs/README.md", "docs", "docs"},
		{"nested_code", "backend/services/user.py", "code", "backend"},
	}

	c := New(&mockRunner{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileStatus := FileStatus{
				Path:        tt.path,
				IndexStatus: 'A',
			}

			enhanced := c.enhanceFileStatus(fileStatus)
			require.Equal(t, tt.expectedType, enhanced.ContentType)
			require.Equal(t, tt.expectedArea, enhanced.AffectedArea)
			require.Greater(t, enhanced.Priority, 0)
		})
	}
}

// Test change magnitude calculation
func TestCollector_CalculateChangeMagnitude(t *testing.T) {
	t.Parallel()

	c := New(&mockRunner{})
	tests := []struct {
		name      string
		fileCount int
		expected  ChangeMagnitude
	}{
		{"small_change", 1, ChangeMagnitudeSmall},
		{"small_change_max", 3, ChangeMagnitudeSmall},
		{"medium_change", 5, ChangeMagnitudeMedium},
		{"medium_change_max", 10, ChangeMagnitudeMedium},
		{"large_change", 15, ChangeMagnitudeLarge},
		{"very_large_change", 50, ChangeMagnitudeLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.calculateChangeMagnitude(tt.fileCount)
			require.Equal(t, tt.expected, result)
		})
	}
}

// Test suggested prefix determination
func TestCollector_DetermineSuggestedPrefix(t *testing.T) {
	t.Parallel()

	c := New(&mockRunner{})
	tests := []struct {
		name        string
		changeTypes map[string]int
		expected    string
	}{
		{
			name:        "new_files",
			changeTypes: map[string]int{"added": 2, "modified": 1},
			expected:    "feat",
		},
		{
			name:        "untracked_files",
			changeTypes: map[string]int{"untracked": 3},
			expected:    "feat",
		},
		{
			name:        "deletions",
			changeTypes: map[string]int{"deleted": 2, "modified": 1},
			expected:    "chore",
		},
		{
			name:        "renames",
			changeTypes: map[string]int{"renamed": 1},
			expected:    "refactor",
		},
		{
			name:        "modifications_only",
			changeTypes: map[string]int{"modified": 5},
			expected:    "fix",
		},
		{
			name:        "empty_changes",
			changeTypes: map[string]int{},
			expected:    "chore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := &ChangesSummary{
				ChangeTypes: tt.changeTypes,
			}
			result := c.determineSuggestedPrefix(changes)
			require.Equal(t, tt.expected, result)
		})
	}
}

// Test file priority sorting with enhanced logic
func TestCollector_SortFilesByPriorityEnhanced(t *testing.T) {
	t.Parallel()

	c := New(&mockRunner{})
	files := []FileStatus{
		{Path: "README.md", IndexStatus: 'M'},     // docs file, modified
		{Path: "main.go", IndexStatus: 'A'},       // code file, added (highest priority)
		{Path: "config.json", IndexStatus: 'A'},   // config file, added
		{Path: "test/unit.go", IndexStatus: 'M'},  // test file, modified (lowest priority)
		{Path: "style.css", IndexStatus: 'M'},     // frontend file, modified
	}

	sorted := c.sortFilesByPriorityEnhanced(files)
	
	// The first file should be the added Go file (highest priority)
	require.Equal(t, "main.go", sorted[0].Path)
	require.Equal(t, "code", sorted[0].ContentType)
	
	// The last file should be the test file (lowest priority)
	lastFile := sorted[len(sorted)-1]
	require.Equal(t, "test/unit.go", lastFile.Path)
	require.Equal(t, "test", lastFile.ContentType)
	
	// All files should have enhanced metadata
	for _, file := range sorted {
		require.NotEmpty(t, file.ContentType)
		require.NotEmpty(t, file.AffectedArea)
		require.Greater(t, file.Priority, 0)
	}
}

// Test backward compatibility
func TestCollector_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("legacy_diff_method", func(t *testing.T) {
		// Test that the legacy Diff method still works
		mr := &mockRunner{
			// ComprehensiveDiff calls: StagedDiff, UnstagedDiff, UntrackedFiles
			outputs: [][]byte{
				[]byte("diff --git a/main.go b/main.go\n+changes"), // StagedDiff
				[]byte(""),                                        // UnstagedDiff
				[]byte(""),                                        // UntrackedFiles
			},
			errs: []error{nil, nil, nil},
		}

		c := New(mr)
		diff, err := c.Diff(context.Background())
		require.NoError(t, err)
		require.Contains(t, diff, "diff --git a/main.go b/main.go")
	})

	t.Run("interface_compatibility", func(t *testing.T) {
		// Test that Collector still implements all interfaces
		c := New(&mockRunner{})
		
		// Test interface implementations
		var _ GitReader = c
		var _ ChangeAnalyzer = c
		var _ FileContentProvider = c
		var _ EnhancedDiffProvider = c
		var _ LegacyCollectorInterface = c
	})
}
