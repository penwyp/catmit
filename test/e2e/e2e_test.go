package e2e

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// buildBinary 构建 catmit 可执行文件并返回路径。
func buildBinary(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "catmit-bin")

	cmd := exec.Command("go", "build", "-o", binPath, "github.com/penwyp/catmit")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v, output: %s", err, string(out))
	}
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	return binPath
}

// initGitRepo 创建临时 Git 仓库并返回路径。
func initGitRepo(t *testing.T) string {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v, %s", args, err, out)
		}
	}
	run("init")
	// 配置用户信息避免 commit 失败
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "tester")

	// 初始提交
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0644)
	run("add", "README.md")
	run("commit", "-m", "chore: init")

	return dir
}

func TestE2E_HappyPathYes(t *testing.T) {
	bin := buildBinary(t)

	// Mock API server 返回成功
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat(test): add file"}}]}`))
	}))
	defer server.Close()

	repo := initGitRepo(t)
	// 创建文件并 stage
	_ = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0644)
	_ = exec.Command("git", "-C", repo, "add", "file.txt").Run()

	cmd := exec.Command(bin, "-y", "--push=false")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"CATMIT_LLM_API_KEY=dummy",
		"CATMIT_LLM_API_URL="+server.URL,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	require.NoError(t, err, out.String())

	// 检查最新 commit message
	logCmd := exec.Command("git", "-C", repo, "log", "-1", "--pretty=%s")
	logOut, _ := logCmd.Output()
	require.Contains(t, string(logOut), "feat(test): add file")
}

func TestE2E_DryRun(t *testing.T) {
	bin := buildBinary(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat(test): add file"}}]}`))
	}))
	defer server.Close()

	repo := initGitRepo(t)
	_ = os.WriteFile(filepath.Join(repo, "a.go"), []byte("package main"), 0644)
	_ = exec.Command("git", "-C", repo, "add", "a.go").Run()

	cmd := exec.Command(bin, "--dry-run")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "CATMIT_LLM_API_KEY=dummy", "CATMIT_LLM_API_URL="+server.URL)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	require.NoError(t, err, out.String())
	require.Contains(t, out.String(), "feat(test): add file")

	// 确认没有 commit
	logCmd := exec.Command("git", "-C", repo, "log", "--pretty=%s")
	logOut, _ := logCmd.Output()
	require.NotContains(t, string(logOut), "feat(test): add file")
}

func TestE2E_Timeout(t *testing.T) {
	bin := buildBinary(t)
	// Mock server 延迟 2s
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer server.Close()

	repo := initGitRepo(t)
	_ = os.WriteFile(filepath.Join(repo, "b.txt"), []byte("x"), 0644)
	_ = exec.Command("git", "-C", repo, "add", "b.txt").Run()

	cmd := exec.Command(bin, "-y", "-t", "1", "--push=false")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "CATMIT_LLM_API_KEY=dummy", "CATMIT_LLM_API_URL="+server.URL)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()

	if exitErr, ok := err.(*exec.ExitError); ok {
		require.Equal(t, 124, exitErr.ExitCode(), out.String())
	} else {
		t.Fatalf("expected exit error with code 124, got %v\n%s", err, out.String())
	}
}

func TestE2E_NothingToCommit(t *testing.T) {
	bin := buildBinary(t)

	// No need for API server since we won't reach that point
	repo := initGitRepo(t)

	// Test with -y flag
	t.Run("yes_mode", func(t *testing.T) {
		cmd := exec.Command(bin, "-y", "--push=false")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "CATMIT_LLM_API_KEY=dummy")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		err := cmd.Run()
		require.NoError(t, err, out.String())
		require.Contains(t, out.String(), "Nothing to commit")
	})

	// Test with --dry-run flag
	t.Run("dry_run_mode", func(t *testing.T) {
		cmd := exec.Command(bin, "--dry-run")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "CATMIT_LLM_API_KEY=dummy")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		err := cmd.Run()
		require.NoError(t, err, out.String())
		require.Contains(t, out.String(), "Nothing to commit")
	})
}
