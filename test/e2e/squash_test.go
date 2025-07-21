package e2e

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSquashCommand_E2E(t *testing.T) {
	// 构建二进制文件
	binPath := buildSquashBinary(t)

	// 设置模拟 LLM 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"choices": [{
				"message": {
					"content": "feat: implement authentication system\n\n- Add user authentication\n- Fix login errors\n- Update documentation"
				}
			}]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	// 设置环境变量
	env := []string{
		fmt.Sprintf("CATMIT_LLM_API_KEY=%s", "test-key"),
		fmt.Sprintf("CATMIT_LLM_API_URL=%s", server.URL),
		fmt.Sprintf("CATMIT_LLM_MODEL=%s", "test-model"),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	tests := []struct {
		name           string
		args           []string
		input          string
		expectedOutput []string
		expectedError  string
	}{
		{
			name: "interactive mode",
			args: []string{"squash", "--no-confirm"},
			input: `feat: add user authentication
fix: resolve login error
docs: update auth guide

`,
			expectedOutput: []string{
				"feat: implement authentication system",
				"Add user authentication",
				"Fix login errors",
				"Update documentation",
			},
		},
		{
			name: "with language flag",
			args: []string{"squash", "--no-confirm", "--lang", "en"},
			input: `feat: add feature
fix: fix bug

`,
			expectedOutput: []string{
				"feat: implement authentication system",
			},
		},
		{
			name: "error with single message",
			args: []string{"squash", "--no-confirm"},
			input: `feat: single commit

`,
			expectedError: "at least 2 commit messages are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binPath, tt.args...)
			cmd.Env = env

			// 设置输入
			if tt.input != "" {
				cmd.Stdin = strings.NewReader(tt.input)
			}

			// 捕获输出
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			// 执行命令
			err := cmd.Run()

			// 检查结果
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, stderr.String(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				output := stdout.String()
				for _, expected := range tt.expectedOutput {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestSquashCommand_EditorMode(t *testing.T) {
	// 跳过 CI 环境
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping editor test in CI environment")
	}

	// 构建二进制文件
	binPath := buildSquashBinary(t)

	// 创建一个测试编辑器脚本
	editorScript := createTestEditor(t, `feat: add authentication
fix: resolve login error
docs: update guide`)

	// 设置模拟 LLM 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"choices": [{
				"message": {
					"content": "feat: complete authentication implementation"
				}
			}]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	// 设置环境变量
	env := []string{
		fmt.Sprintf("CATMIT_LLM_API_KEY=%s", "test-key"),
		fmt.Sprintf("CATMIT_LLM_API_URL=%s", server.URL),
		fmt.Sprintf("CATMIT_LLM_MODEL=%s", "test-model"),
		fmt.Sprintf("EDITOR=%s", editorScript),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	// 执行命令
	cmd := exec.Command(binPath, "squash", "--editor", "--no-confirm")
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "feat: complete authentication implementation")
}

func TestSquashCommand_Timeout(t *testing.T) {
	// 构建二进制文件
	binPath := buildSquashBinary(t)

	// 设置一个慢响应的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 故意延迟超过超时时间
		<-r.Context().Done()
	}))
	defer server.Close()

	// 设置环境变量
	env := []string{
		fmt.Sprintf("CATMIT_LLM_API_KEY=%s", "test-key"),
		fmt.Sprintf("CATMIT_LLM_API_URL=%s", server.URL),
		fmt.Sprintf("CATMIT_LLM_MODEL=%s", "test-model"),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	// 执行命令，设置很短的超时
	cmd := exec.Command(binPath, "squash", "--no-confirm", "--timeout", "1")
	cmd.Env = env
	cmd.Stdin = strings.NewReader("feat: test\nfix: bug\n\n")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	assert.Error(t, err)
	// 超时错误可能包含 "context deadline exceeded" 或类似信息
	stderrStr := stderr.String()
	assert.True(t, strings.Contains(stderrStr, "timeout") || strings.Contains(stderrStr, "deadline"))
}

// createTestEditor 创建一个测试用的编辑器脚本
func createTestEditor(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	editorPath := filepath.Join(tmpDir, "test-editor.sh")

	script := fmt.Sprintf(`#!/bin/bash
echo "%s" > "$1"
`, content)

	err := os.WriteFile(editorPath, []byte(script), 0755)
	require.NoError(t, err)

	return editorPath
}

// buildSquashBinary 构建测试用的二进制文件
func buildSquashBinary(t *testing.T) string {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "catmit")

	cmd := exec.Command("go", "build", "-o", binPath, "../../main.go")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, output)
	}

	return binPath
}