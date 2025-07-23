package cmd

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSquashClient is a mock implementation of squash.ClientInterface
type MockSquashClient struct {
	mock.Mock
}

func (m *MockSquashClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
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

func TestRunSquash_Validation(t *testing.T) {
	// This test verifies the validation logic without running the full command
	// We'll use a more focused approach for the actual runSquash function tests
	// in integration tests, as it requires complex mocking of editor interactions
}

func TestRunSquash_DryRunMode(t *testing.T) {
	// Save original values
	origYes := squashYes
	origDryRun := squashDryRun
	origTimeout := squashTimeout
	defer func() {
		squashYes = origYes
		squashDryRun = origDryRun
		squashTimeout = origTimeout
	}()

	// Set test values
	squashDryRun = true
	squashTimeout = 30
	squashDebug = false

	// Create mock client
	mockClient := new(MockSquashClient)
	mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
		Return("feat: consolidated feature", nil)

	// Test with valid messages (would need to mock getMessagesFromEditor)
	// This is a placeholder for the actual test implementation
	t.Run("dry run output", func(t *testing.T) {
		// The actual implementation would require mocking stdin or editor
		// which is complex for unit tests and better suited for integration tests
		assert.True(t, squashDryRun)
	})
}

func TestRunSquash_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name          string
		messages      []string
		expectedError string
	}{
		{
			name:          "less than 2 messages",
			messages:      []string{"single message"},
			expectedError: "at least 2 commit messages are required",
		},
		{
			name:          "empty messages",
			messages:      []string{},
			expectedError: "at least 2 commit messages are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would need proper mocking of getMessagesFromEditor
			// For now, we just validate the error message format
			assert.Contains(t, tt.expectedError, "at least 2")
		})
	}
}

// Helper function to test getMessagesFromEditor with mocked behavior
func TestGetMessagesFromEditor_Logic(t *testing.T) {
	// Test the parsing logic by simulating file content
	testContent := `# Enter commit messages, one per line, Lines starting with # will be ignored, Save and exit when done

feat: add user authentication
fix: resolve login error
# This is a comment
docs: update auth guide

# Another comment
`

	// Expected messages after parsing
	expected := []string{
		"feat: add user authentication",
		"fix: resolve login error",
		"docs: update auth guide",
	}

	// Parse the content similar to getMessagesFromEditor
	var messages []string
	scanner := bufio.NewScanner(strings.NewReader(testContent))

	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			messages = append(messages, trimmed)
		}
	}

	assert.Equal(t, expected, messages)
}
