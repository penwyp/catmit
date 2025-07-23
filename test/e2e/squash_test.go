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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSquashCommand_E2E(t *testing.T) {
	// Build the binary for testing
	binPath := buildSquashBinary(t)

	// Set up a mock LLM server
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

	// Set environment variables
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
			args: []string{"squash", "--yes"},
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
			args: []string{"squash", "--yes", "--lang", "en"},
			input: `feat: add feature
fix: fix bug

`,
			expectedOutput: []string{
				"feat: implement authentication system",
			},
		},
		{
			name: "error with single message",
			args: []string{"squash", "--yes"},
			input: `feat: single commit

`,
			expectedError: "at least 2 commit messages are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binPath, tt.args...)
			cmd.Env = env

			// Set input if provided
			if tt.input != "" {
				cmd.Stdin = strings.NewReader(tt.input)
			}

			// Capture output
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			// Run the command
			err := cmd.Run()

			// Check results
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
	// Skip in CI environment
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping editor test in CI environment")
	}

	// Build the binary for testing
	binPath := buildSquashBinary(t)

	// Create a test editor script
	editorScript := createTestEditor(t, `feat: add authentication
fix: resolve login error
docs: update guide`)

	// Set up a mock LLM server
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

	// Set environment variables
	env := []string{
		fmt.Sprintf("CATMIT_LLM_API_KEY=%s", "test-key"),
		fmt.Sprintf("CATMIT_LLM_API_URL=%s", server.URL),
		fmt.Sprintf("CATMIT_LLM_MODEL=%s", "test-model"),
		fmt.Sprintf("EDITOR=%s", editorScript),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	// Run the command
	cmd := exec.Command(binPath, "squash", "--yes")
	cmd.Env = env
	// Since we're not in a terminal, it will read from stdin
	cmd.Stdin = strings.NewReader(`feat: add authentication
fix: resolve login error
docs: update guide

`)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Logf("stderr: %s", stderr.String())
		t.Logf("stdout: %s", stdout.String())
	}
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "feat: complete authentication implementation")
}

func TestSquashCommand_Timeout(t *testing.T) {
	// Build the binary for testing
	binPath := buildSquashBinary(t)

	// Set up a slow-responding server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally delay longer than the timeout
		select {
		case <-r.Context().Done():
			// Context cancelled, return immediately
			return
		case <-time.After(5 * time.Second):
			// This shouldn't happen as timeout is 1 second
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Set environment variables
	env := []string{
		fmt.Sprintf("CATMIT_LLM_API_KEY=%s", "test-key"),
		fmt.Sprintf("CATMIT_LLM_API_URL=%s", server.URL),
		fmt.Sprintf("CATMIT_LLM_MODEL=%s", "test-model"),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	// Run the command with a very short timeout
	cmd := exec.Command(binPath, "squash", "--yes", "--timeout", "1")
	cmd.Env = env
	cmd.Stdin = strings.NewReader("feat: test\nfix: bug\n\n")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	assert.Error(t, err)
	// Timeout error may contain "context deadline exceeded" or similar info
	stderrStr := stderr.String()
	assert.True(t, strings.Contains(stderrStr, "timeout") || strings.Contains(stderrStr, "deadline"))
}

// createTestEditor creates a test editor script
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

// buildSquashBinary builds the binary for testing
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
