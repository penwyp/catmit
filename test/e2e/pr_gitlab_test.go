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
	"testing"

	"github.com/stretchr/testify/require"
)

// mockGitLabCLI creates a mock glab CLI that simulates GitLab CLI behavior
func mockGitLabCLI(t *testing.T, responses map[string]string) string {
	t.Helper()
	mockBin := filepath.Join(t.TempDir(), "glab")
	
	script := `#!/bin/bash
args="$@"
case "$args" in
  "version")
    echo 'glab version 1.31.0 (2024-01-15)'
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

func TestE2E_PRCreation_GitLab(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock LLM server
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "feat: add GitLab MR support",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer llmServer.Close()
	
	// Mock GitLab CLI with more generic command matching
	mockBin := filepath.Join(t.TempDir(), "glab")
	script := `#!/bin/bash
args="$@"
echo "DEBUG: glab called with args: $args" >&2
if [[ "$1" == "version" ]]; then
    echo 'glab version 1.31.0 (2024-01-15)'
    exit 0
elif [[ "$1" == "auth" ]] && [[ "$2" == "status" ]]; then
    echo '✓ Logged in to gitlab.com as testuser'
    exit 0
elif [[ "$1" == "mr" ]] && [[ "$2" == "create" ]]; then
    echo "https://gitlab.com/owner/repo/-/merge_requests/456"
    exit 0
else
    echo "Unknown command: $*" >&2
    exit 1
fi
`
	err := os.WriteFile(mockBin, []byte(script), 0755)
	require.NoError(t, err)
	
	repo := setupPRTestRepo(t, "https://gitlab.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "feature.go")
	err = os.WriteFile(testFile, []byte("package main\n\nfunc GitLabFeature() {}\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "feature.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with --pr flag
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--pr")
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
	require.Contains(t, string(logOut), "feat: add GitLab MR support")
	
	// Verify MR URL is shown
	require.Contains(t, out.String(), "Pull request created successfully")
	require.Contains(t, out.String(), "PR URL: https://gitlab.com/owner/repo/-/merge_requests/456")
}

func TestE2E_GitLabAuth(t *testing.T) {
	bin := buildBinary(t)
	
	// Mock GitLab CLI
	glabMock := mockGitLabCLI(t, map[string]string{
		"auth status": "✓ Logged in to gitlab.com as testuser",
		"version":     "glab version 1.31.0 (2024-01-15)",
	})
	
	// Create repo with GitLab remote
	repo := initGitRepo(t)
	
	cmd := exec.Command("git", "remote", "add", "origin", "https://gitlab.com/owner/repo.git")
	cmd.Dir = repo
	err := cmd.Run()
	require.NoError(t, err)
	
	// Run auth status command
	authCmd := exec.Command(bin, "auth", "status")
	authCmd.Dir = repo
	authCmd.Env = append(os.Environ(),
		"PATH="+filepath.Dir(glabMock)+":"+os.Getenv("PATH"),
	)
	
	var out bytes.Buffer
	authCmd.Stdout = &out
	authCmd.Stderr = &out
	
	err = authCmd.Run()
	require.NoError(t, err, out.String())
	
	// Verify output contains auth status
	output := out.String()
	require.Contains(t, output, "Remote")
	require.Contains(t, output, "Provider")
	require.Contains(t, output, "origin")
	require.Contains(t, output, "gitlab")
	require.Contains(t, output, "✓ Authenticated")
}

func TestE2E_PRCreation_GitLab_ExistingMR(t *testing.T) {
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
	
	// Mock GitLab CLI that returns MR already exists error
	mockBin := filepath.Join(t.TempDir(), "glab")
	script := `#!/bin/bash
if [[ "$*" == "auth status" ]]; then
    echo "✓ Logged in to gitlab.com as testuser"
    exit 0
elif [[ "$*" == "version" ]]; then
    echo "glab version 1.31.0 (2024-01-15)"
    exit 0
elif [[ "$*" == mr\ create* ]]; then
    echo "a merge request for branch \"feature-branch\" into branch \"main\" already exists:" >&2
    echo "https://gitlab.com/owner/repo/-/merge_requests/100" >&2
    exit 1
else
    echo "Unknown command: $*" >&2
    exit 1
fi
`
	err := os.WriteFile(mockBin, []byte(script), 0755)
	require.NoError(t, err)
	
	repo := setupPRTestRepo(t, "https://gitlab.com/owner/repo.git")
	
	// Create a file and stage it
	testFile := filepath.Join(repo, "dup.go")
	err = os.WriteFile(testFile, []byte("package main\n"), 0644)
	require.NoError(t, err)
	
	cmd := exec.Command("git", "add", "dup.go")
	cmd.Dir = repo
	err = cmd.Run()
	require.NoError(t, err)
	
	// Run catmit with --pr flag
	catmitCmd := exec.Command(bin, "-y", "-p=false", "--pr")
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
	
	// Verify existing MR URL is shown
	require.Contains(t, out.String(), "Pull request already exists")
	require.Contains(t, out.String(), "PR URL: https://gitlab.com/owner/repo/-/merge_requests/100")
}