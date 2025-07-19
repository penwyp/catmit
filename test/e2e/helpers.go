package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHelper provides utilities for E2E tests
type TestHelper struct {
	t      *testing.T
	binPath string
	repos   []string
}

// NewTestHelper creates a new test helper
func NewTestHelper(t *testing.T) *TestHelper {
	return &TestHelper{
		t:       t,
		binPath: buildBinary(t),
		repos:   []string{},
	}
}

// Cleanup removes all created repositories
func (h *TestHelper) Cleanup() {
	for _, repo := range h.repos {
		os.RemoveAll(repo)
	}
}

// CreateGitRepo creates a new git repository with initial setup
func (h *TestHelper) CreateGitRepo(config RepoConfig) string {
	dir := h.t.TempDir()
	h.repos = append(h.repos, dir)
	
	// Initialize repo
	h.runGit(dir, "init")
	h.runGit(dir, "config", "user.email", config.UserEmail)
	h.runGit(dir, "config", "user.name", config.UserName)
	
	// Create initial commit if requested
	if config.InitialCommit {
		readmePath := filepath.Join(dir, "README.md")
		err := os.WriteFile(readmePath, []byte("# Test Repository\n"), 0644)
		require.NoError(h.t, err)
		h.runGit(dir, "add", "README.md")
		h.runGit(dir, "commit", "-m", "chore: initial commit")
	}
	
	// Add remotes
	for name, url := range config.Remotes {
		h.runGit(dir, "remote", "add", name, url)
	}
	
	// Create and checkout branch if specified
	if config.Branch != "" && config.Branch != "main" {
		h.runGit(dir, "checkout", "-b", config.Branch)
		if config.SetUpstream && len(config.Remotes) > 0 {
			h.runGit(dir, "branch", "--set-upstream-to=origin/main", config.Branch)
		}
	}
	
	return dir
}

// RepoConfig holds configuration for creating a test repository
type RepoConfig struct {
	UserEmail     string
	UserName      string
	InitialCommit bool
	Remotes       map[string]string
	Branch        string
	SetUpstream   bool
}

// DefaultRepoConfig returns a default repo configuration
func DefaultRepoConfig() RepoConfig {
	return RepoConfig{
		UserEmail:     "test@example.com",
		UserName:      "Test User",
		InitialCommit: true,
		Remotes:       map[string]string{},
		Branch:        "main",
		SetUpstream:   false,
	}
}

// runGit executes a git command in the specified directory
func (h *TestHelper) runGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("git %v failed: %v\nOutput: %s", args, err, out)
	}
}

// RunCatmit executes catmit with the given arguments and environment
func (h *TestHelper) RunCatmit(dir string, args []string, env map[string]string) (string, error) {
	cmd := exec.Command(h.binPath, args...)
	cmd.Dir = dir
	
	// Merge environment variables
	cmdEnv := os.Environ()
	for k, v := range env {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = cmdEnv
	
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// AddFile creates a file in the repository and optionally stages it
func (h *TestHelper) AddFile(repoDir, filename, content string, stage bool) {
	filePath := filepath.Join(repoDir, filename)
	
	// Create directory if needed
	dir := filepath.Dir(filePath)
	if dir != "." && dir != repoDir {
		err := os.MkdirAll(dir, 0755)
		require.NoError(h.t, err)
	}
	
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(h.t, err)
	
	if stage {
		h.runGit(repoDir, "add", filename)
	}
}

// GetLastCommitMessage returns the last commit message
func (h *TestHelper) GetLastCommitMessage(repoDir string) string {
	cmd := exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	require.NoError(h.t, err)
	return strings.TrimSpace(string(out))
}

// GetCommitCount returns the number of commits in the repository
func (h *TestHelper) GetCommitCount(repoDir string) int {
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		// No commits yet
		return 0
	}
	
	count := 0
	fmt.Sscanf(string(out), "%d", &count)
	return count
}

// HasStagedChanges checks if there are staged changes
func (h *TestHelper) HasStagedChanges(repoDir string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = repoDir
	err := cmd.Run()
	return err != nil // git diff --cached --quiet returns 1 if there are changes
}

// MockCLIScript creates a mock CLI script with custom behavior
type MockCLIScript struct {
	Name      string
	Commands  map[string]MockCommand
}

// MockCommand represents a mocked CLI command response
type MockCommand struct {
	Output   string
	Error    string
	ExitCode int
}

// CreateMockCLI creates a mock CLI executable
func (h *TestHelper) CreateMockCLI(script MockCLIScript) string {
	mockBin := filepath.Join(h.t.TempDir(), script.Name)
	
	scriptContent := `#!/bin/bash
args="$@"
case "$args" in
`
	for pattern, cmd := range script.Commands {
		scriptContent += fmt.Sprintf(`  %s)`, pattern)
		if cmd.Output != "" {
			scriptContent += fmt.Sprintf(`
    echo '%s'`, cmd.Output)
		}
		if cmd.Error != "" {
			scriptContent += fmt.Sprintf(`
    echo '%s' >&2`, cmd.Error)
		}
		if cmd.ExitCode != 0 {
			scriptContent += fmt.Sprintf(`
    exit %d`, cmd.ExitCode)
		} else {
			scriptContent += `
    exit 0`
		}
		scriptContent += `
    ;;
`
	}
	
	scriptContent += `  *)
    echo "Unknown command: $args" >&2
    exit 1
    ;;
esac
`
	
	err := os.WriteFile(mockBin, []byte(scriptContent), 0755)
	require.NoError(h.t, err)
	
	return mockBin
}

// AssertContains checks that the output contains the expected string
func (h *TestHelper) AssertContains(output, expected string) {
	require.Contains(h.t, output, expected)
}

// AssertNotContains checks that the output does not contain the string
func (h *TestHelper) AssertNotContains(output, unexpected string) {
	require.NotContains(h.t, output, unexpected)
}

// AssertNoError checks that there is no error
func (h *TestHelper) AssertNoError(err error, msg ...string) {
	if len(msg) > 0 {
		require.NoError(h.t, err, msg[0])
	} else {
		require.NoError(h.t, err)
	}
}

// AssertError checks that there is an error
func (h *TestHelper) AssertError(err error) {
	require.Error(h.t, err)
}

// AssertExitCode checks the exit code of an exec.ExitError
func (h *TestHelper) AssertExitCode(err error, expectedCode int) {
	exitErr, ok := err.(*exec.ExitError)
	require.True(h.t, ok, "expected exec.ExitError, got %T", err)
	require.Equal(h.t, expectedCode, exitErr.ExitCode())
}