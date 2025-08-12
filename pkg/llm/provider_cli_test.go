package llm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCLIProvider_WithBinaryName tests creating provider with a binary name.
func TestNewCLIProvider_WithBinaryName(t *testing.T) {
	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	// Test with a commonly available command
	testCmd := "echo"
	if runtime.GOOS == "windows" {
		testCmd = "cmd"
	}

	os.Setenv("CATMIT_LLM_CLI_TOOL", testCmd)

	provider := NewCLIProvider(nil)
	assert.NotNil(t, provider)
	assert.NotEmpty(t, provider.toolPath)
	assert.Equal(t, testCmd, provider.toolName)
}

// TestNewCLIProvider_WithAbsolutePath tests creating provider with an absolute path.
func TestNewCLIProvider_WithAbsolutePath(t *testing.T) {
	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	// Find echo command path
	echoPath, err := exec.LookPath("echo")
	require.NoError(t, err)

	os.Setenv("CATMIT_LLM_CLI_TOOL", echoPath)

	provider := NewCLIProvider(nil)
	assert.NotNil(t, provider)
	assert.Equal(t, echoPath, provider.toolPath)
	assert.Equal(t, "echo", provider.toolName)
}

// TestNewCLIProvider_MissingEnvVar tests panic when env var is not set.
func TestNewCLIProvider_MissingEnvVar(t *testing.T) {
	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	os.Unsetenv("CATMIT_LLM_CLI_TOOL")

	assert.Panics(t, func() {
		NewCLIProvider(nil)
	})
}

// TestNewCLIProvider_NonExistentBinary tests panic when binary doesn't exist.
func TestNewCLIProvider_NonExistentBinary(t *testing.T) {
	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	os.Setenv("CATMIT_LLM_CLI_TOOL", "nonexistent_binary_12345")

	assert.Panics(t, func() {
		NewCLIProvider(nil)
	})
}

// TestNewCLIProvider_NonExistentPath tests panic when absolute path doesn't exist.
func TestNewCLIProvider_NonExistentPath(t *testing.T) {
	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	os.Setenv("CATMIT_LLM_CLI_TOOL", "/nonexistent/path/to/binary")

	assert.Panics(t, func() {
		NewCLIProvider(nil)
	})
}

// TestCLIProvider_buildFullPrompt tests prompt building.
func TestCLIProvider_buildFullPrompt(t *testing.T) {
	provider := &CLIProvider{
		toolPath: "/usr/bin/echo",
		toolName: "echo",
	}

	tests := []struct {
		name         string
		systemPrompt string
		userPrompt   string
		expected     string
	}{
		{
			name:         "Both prompts",
			systemPrompt: "You are a helpful assistant",
			userPrompt:   "Write a commit message",
			expected:     "You are a helpful assistant\n\nWrite a commit message",
		},
		{
			name:         "Only system prompt",
			systemPrompt: "You are a helpful assistant",
			userPrompt:   "",
			expected:     "You are a helpful assistant",
		},
		{
			name:         "Only user prompt",
			systemPrompt: "",
			userPrompt:   "Write a commit message",
			expected:     "Write a commit message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.buildFullPrompt(tt.systemPrompt, tt.userPrompt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCLIProvider_cleanOutput tests output cleaning.
func TestCLIProvider_cleanOutput(t *testing.T) {
	provider := &CLIProvider{
		toolPath: "/usr/bin/echo",
		toolName: "echo",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ANSI escape codes",
			input:    "\x1b[31mRed text\x1b[0m normal text",
			expected: "Red text normal text",
		},
		{
			name:     "Control characters",
			input:    "Hello\x00World\x07Bell\x1BEscape",
			expected: "HelloWorldBellEscape",
		},
		{
			name:     "Whitespace trimming",
			input:    "  \n\n  Content  \n\n  ",
			expected: "Content",
		},
		{
			name:     "Mixed",
			input:    "\x1b[1mBold\x1b[0m \x00 text\n  ",
			expected: "Bold  text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.cleanOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCLIProvider_buildCommand tests command building for different tools.
func TestCLIProvider_buildCommand(t *testing.T) {
	ctx := context.Background()
	prompt := "Test prompt"

	tests := []struct {
		name          string
		toolName      string
		streaming     bool
		expectedArgs  []string
		checkStdin    bool
	}{
		{
			name:         "Claude non-streaming",
			toolName:     "claude",
			streaming:    false,
			expectedArgs: []string{"-p", prompt},
			checkStdin:   false,
		},
		{
			name:         "Claude streaming",
			toolName:     "claude",
			streaming:    true,
			expectedArgs: []string{"-p", prompt, "--output-format", "stream-json"},
			checkStdin:   false,
		},
		{
			name:         "Gemini",
			toolName:     "gemini",
			streaming:    false,
			expectedArgs: []string{"-p", prompt},
			checkStdin:   false,
		},
		{
			name:         "AIChat",
			toolName:     "aichat",
			streaming:    false,
			expectedArgs: []string{"-S"},
			checkStdin:   true,
		},
		{
			name:         "Unknown tool",
			toolName:     "unknown",
			streaming:    false,
			expectedArgs: []string{},
			checkStdin:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &CLIProvider{
				toolPath: "/usr/bin/" + tt.toolName,
				toolName: tt.toolName,
			}

			cmd := provider.buildCommand(ctx, prompt, tt.streaming)
			assert.NotNil(t, cmd)
			
			// Check args
			if len(tt.expectedArgs) > 0 {
				assert.Equal(t, tt.expectedArgs, cmd.Args[1:]) // Skip the command itself
			}

			// Check if stdin is needed
			assert.Equal(t, tt.checkStdin, provider.needsStdinInput())
		})
	}
}

// TestCLIProvider_processStreamLine tests stream line processing.
func TestCLIProvider_processStreamLine(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		line     string
		expected string
	}{
		{
			name:     "Claude JSON content",
			toolName: "claude",
			line:     `{"content":"Hello world","type":"text"}`,
			expected: "Hello world",
		},
		{
			name:     "Claude no content",
			toolName: "claude",
			line:     `{"type":"meta"}`,
			expected: "",
		},
		{
			name:     "Ollama loading indicator",
			toolName: "ollama",
			line:     ">>> Loading model...",
			expected: "",
		},
		{
			name:     "Ollama content",
			toolName: "ollama",
			line:     "This is the response",
			expected: "This is the response",
		},
		{
			name:     "Generic tool",
			toolName: "generic",
			line:     "\x1b[1mBold text\x1b[0m",
			expected: "Bold text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &CLIProvider{
				toolPath: "/usr/bin/" + tt.toolName,
				toolName: tt.toolName,
			}

			result := provider.processStreamLine(tt.line)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCLIProvider_GetCompletion_Echo tests basic completion with echo command.
func TestCLIProvider_GetCompletion_Echo(t *testing.T) {
	// Skip if echo is not available
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo command not available")
	}

	// Create a simple test script that echoes input
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test_cli")
	
	// Create script content based on OS
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptPath += ".bat"
		scriptContent = "@echo off\necho Test output: %*"
	} else {
		scriptContent = "#!/bin/sh\necho \"Test output: $@\""
	}

	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	os.Setenv("CATMIT_LLM_CLI_TOOL", scriptPath)

	provider := NewCLIProvider(nil)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := provider.GetCompletion(ctx, "System prompt", "User prompt")
	require.NoError(t, err)
	assert.Contains(t, result, "Test output")
}

// TestCLIProvider_GetCompletionStream tests streaming with a mock tool.
func TestCLIProvider_GetCompletionStream(t *testing.T) {
	// Create a test script that outputs multiple lines
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "stream_cli")
	
	// Create script content based on OS
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptPath += ".bat"
		scriptContent = "@echo off\necho Line 1\necho Line 2\necho Line 3"
	} else {
		scriptContent = "#!/bin/sh\necho 'Line 1'\necho 'Line 2'\necho 'Line 3'"
	}

	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	os.Setenv("CATMIT_LLM_CLI_TOOL", scriptPath)

	provider := NewCLIProvider(nil)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	contentChan, errChan := provider.GetCompletionStream(ctx, "System", "User")

	var lines []string
	for {
		select {
		case content, ok := <-contentChan:
			if !ok {
				goto done
			}
			lines = append(lines, content)
		case err := <-errChan:
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("Context timeout")
		}
	}
done:

	assert.Len(t, lines, 3)
	for i, line := range lines {
		assert.True(t, strings.Contains(line, "Line"))
		assert.True(t, strings.Contains(line, string(rune('1'+i))))
	}
}

// TestCLIProvider_ContextCancellation tests handling of context cancellation.
func TestCLIProvider_ContextCancellation(t *testing.T) {
	// Create a slow script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "slow_cli")
	
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptPath += ".bat"
		scriptContent = "@echo off\nping -n 10 127.0.0.1 > nul\necho Done"
	} else {
		scriptContent = "#!/bin/sh\nsleep 10\necho Done"
	}

	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Save original env
	originalTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	defer os.Setenv("CATMIT_LLM_CLI_TOOL", originalTool)

	os.Setenv("CATMIT_LLM_CLI_TOOL", scriptPath)

	provider := NewCLIProvider(nil)
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	contentChan, errChan := provider.GetCompletionStream(ctx, "System", "User")

	select {
	case <-contentChan:
		// Should be closed due to cancellation
	case <-errChan:
		// May or may not have an error
	case <-time.After(1 * time.Second):
		t.Fatal("Should have cancelled quickly")
	}
}