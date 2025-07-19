package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockGitHubCLI creates a mock gh CLI that simulates GitHub CLI behavior
func mockGitHubCLI(t *testing.T, responses map[string]string) string {
	t.Helper()
	mockBin := filepath.Join(t.TempDir(), "gh")
	
	script := `#!/bin/bash
args="$@"
case "$args" in
  "version")
    echo 'gh version 2.40.0 (2024-01-15)'
    exit 0
    ;;
`
	for args, response := range responses {
		script += fmt.Sprintf(`  "%s")
    echo '%s'
    exit 0
    ;;
`, args, response)
	}
	
	script += `  *)
    echo "Unknown command: $args" >&2
    exit 1
    ;;
esac
`
	err := os.WriteFile(mockBin, []byte(script), 0755)
	require.NoError(t, err)
	return mockBin
}

// mockGiteaCLI creates a mock tea CLI that simulates Gitea CLI behavior
func mockGiteaCLI(t *testing.T, responses map[string]string) string {
	t.Helper()
	mockBin := filepath.Join(t.TempDir(), "tea")
	
	script := `#!/bin/bash
args="$@"
case "$args" in
  "version")
    echo 'tea version 0.9.0'
    exit 0
    ;;
`
	for args, response := range responses {
		script += fmt.Sprintf(`  "%s")
    echo '%s'
    exit 0
    ;;
`, args, response)
	}
	
	script += `  *)
    echo "Unknown command: $args" >&2
    exit 1
    ;;
esac
`
	err := os.WriteFile(mockBin, []byte(script), 0755)
	require.NoError(t, err)
	return mockBin
}

// setupPRTestRepo creates a git repo with a remote for PR testing
func setupPRTestRepo(t *testing.T, remoteURL string) string {
	dir := initGitRepo(t)
	
	// Create a bare repository to act as the remote
	bareDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	
	// Add the bare repo as remote with the desired URL
	// We'll use the config to override the actual push URL
	cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	
	// Set the actual push URL to the local bare repo
	cmd = exec.Command("git", "config", "remote.origin.pushurl", bareDir)
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	
	// Get current branch name
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	branchOut, err := cmd.Output()
	require.NoError(t, err)
	currentBranch := strings.TrimSpace(string(branchOut))
	
	// Push current branch to establish it
	cmd = exec.Command("git", "push", "-u", "origin", currentBranch)
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	
	// Create and checkout feature branch
	cmd = exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	
	return dir
}

func TestE2E_PRCreation_GitHub(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock LLM server
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "feat: add new feature for testing PR creation",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer llmServer.Close()
	
	// Mock GitHub CLI
	ghMock := mockGitHubCLI(t, map[string]string{
		"auth status":                                     "Logged in to github.com as testuser",
		"pr create --fill --base main --draft=false":      "https://github.com/owner/repo/pull/123",
	})
	
	repo := setupPRTestRepo(t, "https://github.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "feature.go")
	err := os.WriteFile(testFile, []byte("package main\n\nfunc NewFeature() {}\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "feature.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with --create-pr flag
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--create-pr")
	catmitCmd.Dir = repo
	catmitCmd.Env = append(os.Environ(),
		"CATMIT_LLM_API_KEY=test-key",
		"CATMIT_LLM_API_URL="+llmServer.URL,
		"PATH="+filepath.Dir(ghMock)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	catmitCmd.Stdout = &out
	catmitCmd.Stderr = &out
	
	err = catmitCmd.Run()
	require.NoError(t, err, out.String())
	
	// Verify commit was created
	logCmd := exec.Command("git", "log", "-1", "--pretty=%s")
	logCmd.Dir = repo
	logOut, err := logCmd.Output()
	require.NoError(t, err)
	require.Contains(t, string(logOut), "feat: add new feature")
	
	// Verify PR URL is shown
	require.Contains(t, out.String(), "Pull request created successfully")
	require.Contains(t, out.String(), "PR URL: https://github.com/owner/repo/pull/123")
}

func TestE2E_PRCreation_Gitea(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock LLM server
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "fix: resolve critical bug in authentication",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer llmServer.Close()
	
	// Mock Gitea API for provider detection
	giteaAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/version") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"version": "1.21.0"})
		}
	}))
	defer giteaAPIServer.Close()
	
	// Mock Gitea CLI with more generic command matching
	mockBin := filepath.Join(t.TempDir(), "tea")
	script := `#!/bin/bash
args="$@"
echo "DEBUG: tea called with args: $args" >&2
case "$args" in
  "version")
    echo 'tea version 0.9.0'
    exit 0
    ;;
  "login list")
    echo 'URL  | NAME    | DEFAULT | SSH KEY | USER    | ACCESS TOKEN'
    echo 'gitea.example.com | Default | true    | false   | testuser | ****************'
    exit 0
    ;;
  pr\ create*)
    echo "https://gitea.example.com/owner/repo/pulls/42"
    exit 0
    ;;
  *)
    echo "Unknown command: $args" >&2
    exit 1
    ;;
esac
`
	err := os.WriteFile(mockBin, []byte(script), 0755)
	require.NoError(t, err)
	
	// Use a more realistic Gitea URL for provider detection
	repo := setupPRTestRepo(t, "https://gitea.example.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "auth_fix.go")
	err = os.WriteFile(testFile, []byte("package auth\n\nfunc FixBug() {}\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "auth_fix.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with --create-pr flag
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--create-pr")
	catmitCmd.Dir = repo
	catmitCmd.Env = append(os.Environ(),
		"CATMIT_LLM_API_KEY=test-key",
		"CATMIT_LLM_API_URL="+llmServer.URL,
		"PATH="+filepath.Dir(mockBin)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	catmitCmd.Stdout = &out
	catmitCmd.Stderr = &out
	
	err = catmitCmd.Run()
	require.NoError(t, err, out.String())
	
	// Verify commit was created
	logCmd := exec.Command("git", "log", "-1", "--pretty=%s")
	logCmd.Dir = repo
	logOut, err := logCmd.Output()
	require.NoError(t, err)
	require.Contains(t, string(logOut), "fix: resolve critical bug")
	
	// Verify PR URL is shown
	require.Contains(t, out.String(), "Pull request created successfully")
	require.Contains(t, out.String(), "PR URL: https://gitea.example.com/owner/repo/pulls/42")
}

func TestE2E_PRCreation_NoChanges(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock GitHub CLI
	ghMock := mockGitHubCLI(t, map[string]string{
		"auth status":             "Logged in to github.com as testuser",
		"pr create --fill --base main --draft=false": "https://github.com/owner/repo/pull/456",
	})
	
	repo := setupPRTestRepo(t, "https://github.com/owner/repo.git")
	
	// Run catmit with --create-pr flag but no changes
	catmitCmd := exec.Command(bin, "-y", "--create-pr")
	catmitCmd.Dir = repo
	catmitCmd.Env = append(os.Environ(),
		"CATMIT_LLM_API_KEY=test-key",
		"PATH="+filepath.Dir(ghMock)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	catmitCmd.Stdout = &out
	catmitCmd.Stderr = &out
	
	err := catmitCmd.Run()
	require.NoError(t, err, out.String())
	
	// When --create-pr is specified, it creates PR even with no changes
	// It should NOT print "Nothing to commit" because it returns after PR creation
	require.NotContains(t, out.String(), "Nothing to commit")
	require.Contains(t, out.String(), "Pull request created successfully")
	require.Contains(t, out.String(), "PR URL: https://github.com/owner/repo/pull/456")
}

func TestE2E_AuthStatus(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock GitHub and Gitea CLIs
	ghMock := mockGitHubCLI(t, map[string]string{
		"auth status": "Logged in to github.com as testuser",
		"version":     "gh version 2.40.0 (2024-01-15)",
	})
	
	teaMock := mockGiteaCLI(t, map[string]string{
		"login list": "gitea.example.com (testuser)",
		"version":    "tea version 0.9.0",
	})
	
	// Create repo with multiple remotes
	repo := initGitRepo(t)
	
	// Add GitHub remote
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/owner/repo.git")
	cmd.Dir = repo
	err := cmd.Run()
	require.NoError(t, err)
	
	// Add Gitea remote  
	cmd = exec.Command("git", "remote", "add", "gitea", "https://gitea.example.com/owner/repo.git")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Mock Gitea API server for provider detection
	giteaAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/version") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"version": "1.21.0"})
		}
	}))
	defer giteaAPIServer.Close()
	
	// Run auth status command
	authCmd := exec.Command(bin, "auth", "status")
	authCmd.Dir = repo
	authCmd.Env = append(os.Environ(),
		"PATH="+filepath.Dir(ghMock)+":"+filepath.Dir(teaMock)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	authCmd.Stdout = &out
	authCmd.Stderr = &out
	
	err = authCmd.Run()
	require.NoError(t, err, out.String())
	
	// Verify output contains auth status for both remotes
	output := out.String()
	require.Contains(t, output, "Remote")
	require.Contains(t, output, "Provider")
	require.Contains(t, output, "origin")
	require.Contains(t, output, "github")
	require.Contains(t, output, "gitea")
	require.Contains(t, output, "✓ Authenticated")
}

func TestE2E_PRCreation_ExistingPR(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock LLM server
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "feat: add duplicate feature",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer llmServer.Close()
	
	// Mock GitHub CLI that returns PR already exists error
	mockBin := filepath.Join(t.TempDir(), "gh")
	script := `#!/bin/bash
if [[ "$*" == "auth status" ]]; then
    echo "Logged in to github.com as testuser"
    exit 0
elif [[ "$*" == "version" ]]; then
    echo "gh version 2.40.0 (2024-01-15)"
    exit 0
elif [[ "$*" == pr\ create* ]]; then
    echo "a pull request for branch \"feature-branch\" into branch \"main\" already exists:" >&2
    echo "https://github.com/owner/repo/pull/100" >&2
    exit 1
else
    echo "Unknown command: $*" >&2
    exit 1
fi
`
	err := os.WriteFile(mockBin, []byte(script), 0755)
	require.NoError(t, err)
	
	repo := setupPRTestRepo(t, "https://github.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "dup.go")
	err = os.WriteFile(testFile, []byte("package main\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "dup.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with --create-pr flag
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--create-pr")
	catmitCmd.Dir = repo
	catmitCmd.Env = append(os.Environ(),
		"CATMIT_LLM_API_KEY=test-key",
		"CATMIT_LLM_API_URL="+llmServer.URL,
		"PATH="+filepath.Dir(mockBin)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	catmitCmd.Stdout = &out
	catmitCmd.Stderr = &out
	
	err = catmitCmd.Run()
	require.NoError(t, err, out.String())
	
	// Verify existing PR URL is shown
	require.Contains(t, out.String(), "Pull request already exists")
	require.Contains(t, out.String(), "PR URL: https://github.com/owner/repo/pull/100")
}

func TestE2E_PRCreation_CLINotInstalled(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock LLM server
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "feat: test cli not found",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer llmServer.Close()
	
	repo := setupPRTestRepo(t, "https://github.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "test.go")
	err := os.WriteFile(testFile, []byte("package main\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "test.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with limited PATH to simulate CLI not found
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--create-pr")
	catmitCmd.Dir = repo
	// Get minimal PATH with just system binaries
	minimalPath := "/usr/bin:/bin"
	catmitCmd.Env = []string{
		"CATMIT_LLM_API_KEY=test-key",
		"CATMIT_LLM_API_URL=" + llmServer.URL,
		"PATH=" + minimalPath,
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
	}
	
	var out bytes.Buffer
	catmitCmd.Stdout = &out
	catmitCmd.Stderr = &out
	
	err = catmitCmd.Run()
	// Should fail because PR creation fails
	require.Error(t, err)
	
	// Verify commit was created
	logCmd := exec.Command("git", "log", "-1", "--pretty=%s")
	logCmd.Dir = repo
	logOut, err := logCmd.Output()
	require.NoError(t, err)
	require.Contains(t, string(logOut), "feat: test cli not found")
	
	// Verify error message about installing CLI
	require.Contains(t, out.String(), "gh is not installed")
}

func TestE2E_PRCreation_Timeout(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock LLM server with delay
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "feat: timeout test",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer llmServer.Close()
	
	// Mock GitHub CLI
	ghMock := mockGitHubCLI(t, map[string]string{
		"auth status": "Logged in to github.com as testuser",
		"pr create --fill --base main": "https://github.com/owner/repo/pull/789",
	})
	
	repo := setupPRTestRepo(t, "https://github.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "timeout.go")
	err := os.WriteFile(testFile, []byte("package main\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "timeout.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with short timeout
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--create-pr", "-t", "1")
	catmitCmd.Dir = repo
	catmitCmd.Env = append(os.Environ(),
		"CATMIT_LLM_API_KEY=test-key",
		"CATMIT_LLM_API_URL="+llmServer.URL,
		"PATH="+filepath.Dir(ghMock)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	catmitCmd.Stdout = &out
	catmitCmd.Stderr = &out
	
	err = catmitCmd.Run()
	
	// Should timeout with exit code 124
	if exitErr, ok := err.(*exec.ExitError); ok {
		require.Equal(t, 124, exitErr.ExitCode(), out.String())
	} else {
		t.Fatalf("expected exit error with code 124, got %v\n%s", err, out.String())
	}
}