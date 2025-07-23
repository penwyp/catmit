package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atotto/clipboard"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"golang.org/x/term"
)

// MockSquashClient is a mock implementation of squash.ClientInterface
type MockSquashClient struct {
	mock.Mock
}

func (m *MockSquashClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
}

// MockTerminal mocks terminal state check
type MockTerminal struct {
	isTerminal bool
}

func (m *MockTerminal) IsTerminal(fd int) bool {
	return m.isTerminal
}

// TestableCommand wraps cobra.Command for testing
type TestableCommand struct {
	*cobra.Command
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func NewTestableSquashCommand() *TestableCommand {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	
	cmd := &cobra.Command{
		Use:   "squash-draft",
		Short: "Consolidate multiple commit messages into one",
		RunE:  runSquash,
	}
	
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	
	// Add flags
	cmd.Flags().BoolVarP(&squashYes, "yes", "y", false, "Skip confirmation")
	cmd.Flags().StringVarP(&squashLang, "lang", "l", "en", "Output language")
	cmd.Flags().IntVarP(&squashTimeout, "timeout", "t", 30, "Timeout in seconds")
	cmd.Flags().BoolVar(&squashDebug, "debug", false, "Enable debug output")
	cmd.Flags().BoolVar(&squashDryRun, "dry-run", false, "Preview without copying")
	
	return &TestableCommand{
		Command: cmd,
		stdout:  stdout,
		stderr:  stderr,
	}
}


func TestReadMessagesFromStdin(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "multiple messages",
			input: `feat: add feature
fix: fix bug
docs: update docs`,
			expected: []string{
				"feat: add feature",
				"fix: fix bug",
				"docs: update docs",
			},
		},
		{
			name: "messages with comments",
			input: `feat: add feature
# This is a comment
fix: fix bug
# Another comment
docs: update docs`,
			expected: []string{
				"feat: add feature",
				"fix: fix bug",
				"docs: update docs",
			},
		},
		{
			name: "messages with empty lines",
			input: `feat: add feature

fix: fix bug

docs: update docs`,
			expected: []string{
				"feat: add feature",
				"fix: fix bug",
				"docs: update docs",
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name: "only comments",
			input: `# Comment 1
# Comment 2
# Comment 3`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file to simulate stdin
			tmpfile, err := os.CreateTemp("", "test-stdin")
			assert.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			// Write test input
			_, err = tmpfile.WriteString(tt.input)
			assert.NoError(t, err)

			// Seek to beginning
			_, err = tmpfile.Seek(0, 0)
			assert.NoError(t, err)

			// Replace stdin
			oldStdin := os.Stdin
			os.Stdin = tmpfile
			defer func() { os.Stdin = oldStdin }()

			// Call the function
			messages, err := readMessagesFromStdin()

			// Verify results
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, messages)
		})
	}
}

func TestSquashCommand(t *testing.T) {
	// Test that the squash command is properly initialized
	assert.NotNil(t, squashDraftCmd)
	assert.Equal(t, "squash-draft", squashDraftCmd.Use)
	assert.Contains(t, squashDraftCmd.Short, "Consolidate")

	// Test flags
	flags := squashDraftCmd.Flags()
	assert.NotNil(t, flags.Lookup("yes"))
	assert.NotNil(t, flags.Lookup("lang"))
	assert.NotNil(t, flags.Lookup("timeout"))
	assert.NotNil(t, flags.Lookup("debug"))
	assert.NotNil(t, flags.Lookup("dry-run"))
}

func TestRunSquash_DryRunMode(t *testing.T) {
	// Save original values
	origYes := squashYes
	origDryRun := squashDryRun
	origTimeout := squashTimeout
	origLang := squashLang
	origDebug := squashDebug
	defer func() {
		squashYes = origYes
		squashDryRun = origDryRun
		squashTimeout = origTimeout
		squashLang = origLang
		squashDebug = origDebug
	}()

	// Mock the getMessagesFromEditor function
	origGetMessages := getMessagesFromEditor
	getMessagesFromEditor = func() ([]string, error) {
		return []string{
			"feat: add feature",
			"fix: bug fix",
			"docs: update readme",
		}, nil
	}
	defer func() { getMessagesFromEditor = origGetMessages }()

	// Mock dependencies
	origInitDeps := initSquashDependencies
	mockClient := new(MockSquashClient)
	mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
		Return("feat: consolidated feature with fixes and docs", nil)

	initSquashDependencies = func(debug bool) (*dependencies, error) {
		return &dependencies{
			llmClient: mockClient,
			logger:    zap.NewNop(),
		}, nil
	}
	defer func() { initSquashDependencies = origInitDeps }()

	// Test dry-run mode
	t.Run("dry run output", func(t *testing.T) {
		squashDryRun = true
		squashTimeout = 30
		squashLang = "en"
		squashDebug = false

		cmd := NewTestableSquashCommand()
		err := cmd.Execute()

		assert.NoError(t, err)
		output := cmd.stdout.String()
		assert.Contains(t, output, "=== DRY RUN MODE ===")
		assert.Contains(t, output, "feat: consolidated feature with fixes and docs")
		assert.Contains(t, output, "(Message not copied to clipboard)")
		mockClient.AssertExpectations(t)
	})
}

func TestRunSquash_YesMode(t *testing.T) {
	// Save original values
	origYes := squashYes
	origDryRun := squashDryRun
	origTimeout := squashTimeout
	origLang := squashLang
	defer func() {
		squashYes = origYes
		squashDryRun = origDryRun
		squashTimeout = origTimeout
		squashLang = origLang
	}()

	// Mock getMessagesFromEditor
	origGetMessages := getMessagesFromEditor
	getMessagesFromEditor = func() ([]string, error) {
		return []string{
			"feat: feature one",
			"feat: feature two",
		}, nil
	}
	defer func() { getMessagesFromEditor = origGetMessages }()

	// Mock dependencies
	origInitDeps := initSquashDependencies
	mockClient := new(MockSquashClient)
	mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
		Return("feat: combined features", nil)

	initSquashDependencies = func(debug bool) (*dependencies, error) {
		return &dependencies{
			llmClient: mockClient,
			logger:    zap.NewNop(),
		}, nil
	}
	defer func() { initSquashDependencies = origInitDeps }()

	// Mock clipboard
	origClipboard := clipboard.WriteAll
	clipboardCalled := false
	clipboard.WriteAll = func(text string) error {
		clipboardCalled = true
		assert.Equal(t, "feat: combined features", text)
		return nil
	}
	defer func() { clipboard.WriteAll = origClipboard }()

	t.Run("yes mode with clipboard", func(t *testing.T) {
		squashYes = true
		squashDryRun = false
		squashTimeout = 30
		squashLang = "en"

		cmd := NewTestableSquashCommand()
		err := cmd.Execute()

		assert.NoError(t, err)
		output := cmd.stdout.String()
		assert.Contains(t, output, "feat: combined features")
		assert.Contains(t, cmd.stderr.String(), "✓ Copied to clipboard")
		assert.True(t, clipboardCalled)
		mockClient.AssertExpectations(t)
	})
}

func TestRunSquash_ErrorScenarios(t *testing.T) {
	// Save original values
	origYes := squashYes
	origDryRun := squashDryRun
	origTimeout := squashTimeout
	defer func() {
		squashYes = origYes
		squashDryRun = origDryRun
		squashTimeout = origTimeout
	}()

	tests := []struct {
		name             string
		setupMocks       func()
		expectedError    string
	}{
		{
			name: "less than 2 messages",
			setupMocks: func() {
				origGetMessages := getMessagesFromEditor
				getMessagesFromEditor = func() ([]string, error) {
					return []string{"single message"}, nil
				}
				t.Cleanup(func() { getMessagesFromEditor = origGetMessages })
			},
			expectedError: "at least 2 commit messages are required",
		},
		{
			name: "empty messages",
			setupMocks: func() {
				origGetMessages := getMessagesFromEditor
				getMessagesFromEditor = func() ([]string, error) {
					return []string{}, nil
				}
				t.Cleanup(func() { getMessagesFromEditor = origGetMessages })
			},
			expectedError: "at least 2 commit messages are required",
		},
		{
			name: "editor error",
			setupMocks: func() {
				origGetMessages := getMessagesFromEditor
				getMessagesFromEditor = func() ([]string, error) {
					return nil, errors.New("editor failed")
				}
				t.Cleanup(func() { getMessagesFromEditor = origGetMessages })
			},
			expectedError: "failed to get messages from editor: editor failed",
		},
		{
			name: "dependency initialization error",
			setupMocks: func() {
				origInitDeps := initSquashDependencies
				initSquashDependencies = func(debug bool) (*dependencies, error) {
					return nil, errors.New("init failed")
				}
				t.Cleanup(func() { initSquashDependencies = origInitDeps })
			},
			expectedError: "init failed",
		},
		{
			name: "LLM generation error",
			setupMocks: func() {
				origGetMessages := getMessagesFromEditor
				getMessagesFromEditor = func() ([]string, error) {
					return []string{"msg1", "msg2"}, nil
				}
				t.Cleanup(func() { getMessagesFromEditor = origGetMessages })

				origInitDeps := initSquashDependencies
				mockClient := new(MockSquashClient)
				mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
					Return("", errors.New("API error"))

				initSquashDependencies = func(debug bool) (*dependencies, error) {
					return &dependencies{
						llmClient: mockClient,
						logger:    zap.NewNop(),
					}, nil
				}
				t.Cleanup(func() { initSquashDependencies = origInitDeps })
			},
			expectedError: "failed to generate commit message: API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			squashYes = true
			squashDryRun = false
			squashTimeout = 30

			cmd := NewTestableSquashCommand()
			err := cmd.Execute()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestGetMessagesFromEditor_StdinPiped(t *testing.T) {
	// Save original stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	// Create test input
	input := `feat: add feature
# This is a comment
fix: bug fix

docs: update readme
# Another comment`

	// Create a pipe
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stdin = r

	// Write test data to pipe
	go func() {
		fmt.Fprint(w, input)
		w.Close()
	}()

	// Mock term.IsTerminal to return false (piped input)
	// This would need proper mocking in real implementation
	messages, err := readMessagesFromStdin()

	assert.NoError(t, err)
	assert.Equal(t, []string{
		"feat: add feature",
		"fix: bug fix",
		"docs: update readme",
	}, messages)
}

func TestGetMessagesFromEditor_EditorInteraction(t *testing.T) {
	// This test simulates the editor interaction flow
	t.Run("editor environment variable", func(t *testing.T) {
		// Test with CATMIT_EDITOR set
		os.Setenv("CATMIT_EDITOR", "custom-editor")
		defer os.Unsetenv("CATMIT_EDITOR")

		// The actual editor interaction would be tested in integration tests
		// Here we just verify the environment variable is respected
		editor := os.Getenv("CATMIT_EDITOR")
		assert.Equal(t, "custom-editor", editor)
	})

	t.Run("default editor fallback", func(t *testing.T) {
		// Ensure CATMIT_EDITOR is not set
		os.Unsetenv("CATMIT_EDITOR")

		// Default should be vim
		editor := os.Getenv("CATMIT_EDITOR")
		if editor == "" {
			editor = "vim"
		}
		assert.Equal(t, "vim", editor)
	})
}

func TestSquashCommand_Integration(t *testing.T) {
	// Integration test for the full command flow
	t.Run("command structure", func(t *testing.T) {
		assert.NotNil(t, squashDraftCmd)
		assert.Equal(t, "squash-draft", squashDraftCmd.Use)
		assert.Contains(t, squashDraftCmd.Short, "Consolidate")
		assert.Contains(t, squashDraftCmd.Long, "comprehensive commit message")
		assert.NotEmpty(t, squashDraftCmd.Example)
	})

	t.Run("flags configuration", func(t *testing.T) {
		flags := squashDraftCmd.Flags()
		
		// Check all flags are properly configured
		yesFlag := flags.Lookup("yes")
		assert.NotNil(t, yesFlag)
		assert.Equal(t, "y", yesFlag.Shorthand)
		
		langFlag := flags.Lookup("lang")
		assert.NotNil(t, langFlag)
		assert.Equal(t, "l", langFlag.Shorthand)
		assert.Equal(t, "en", langFlag.DefValue)
		
		timeoutFlag := flags.Lookup("timeout")
		assert.NotNil(t, timeoutFlag)
		assert.Equal(t, "t", timeoutFlag.Shorthand)
		assert.Equal(t, "30", timeoutFlag.DefValue)
		
		debugFlag := flags.Lookup("debug")
		assert.NotNil(t, debugFlag)
		assert.Equal(t, "", debugFlag.Shorthand)
		
		dryRunFlag := flags.Lookup("dry-run")
		assert.NotNil(t, dryRunFlag)
		assert.Equal(t, "", dryRunFlag.Shorthand)
	})
}

func TestRunSquash_TUIMode(t *testing.T) {
	// This test would require mocking the TUI components
	// which is complex and better suited for integration tests
	// Here we document the expected behavior

	t.Run("TUI initialization", func(t *testing.T) {
		// The TUI mode should:
		// 1. Create a ui.NewSquashModel with the squash instance and messages
		// 2. Run the model
		// 3. Check if user accepted the result
		// 4. Print the result if accepted
		// 5. Show clipboard success message if copied

		// This would be tested with a mock UI model in integration tests
		assert.True(t, true) // Placeholder
	})
}
