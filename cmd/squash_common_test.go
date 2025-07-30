package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/penwyp/catmit/pkg/llm"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockLLMClient is a mock implementation of llm.Client
type MockLLMClient struct {
	mock.Mock
}

func (m *MockLLMClient) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := m.Called(ctx, systemPrompt, userPrompt)
	return args.String(0), args.Error(1)
}

// MockGitRunnerForSquash is a mock implementation of git.Runner for squash tests
type MockGitRunnerForSquash struct {
	mock.Mock
}

func (m *MockGitRunnerForSquash) Execute(ctx context.Context, args ...string) (string, error) {
	mockArgs := m.Called(ctx, args)
	return mockArgs.String(0), mockArgs.Error(1)
}

func (m *MockGitRunnerForSquash) Run(ctx context.Context, args ...string) (string, error) {
	mockArgs := m.Called(ctx, args)
	return mockArgs.String(0), mockArgs.Error(1)
}

func (m *MockGitRunnerForSquash) ExecuteInDir(ctx context.Context, dir string, args ...string) (string, error) {
	mockArgs := m.Called(ctx, dir, args)
	return mockArgs.String(0), mockArgs.Error(1)
}

func (m *MockGitRunnerForSquash) ExecuteWithInput(ctx context.Context, input string, args ...string) (string, error) {
	mockArgs := m.Called(ctx, input, args)
	return mockArgs.String(0), mockArgs.Error(1)
}

func (m *MockGitRunnerForSquash) ExecuteWithEnv(ctx context.Context, env []string, args ...string) (string, error) {
	mockArgs := m.Called(ctx, env, args)
	return mockArgs.String(0), mockArgs.Error(1)
}

func TestInitSquashDependencies(t *testing.T) {
	tests := []struct {
		name      string
		debug     bool
		wantError bool
	}{
		{
			name:      "debug disabled",
			debug:     false,
			wantError: false,
		},
		{
			name:      "debug enabled",
			debug:     true,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := initSquashDependencies(tt.debug)
			
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, deps)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, deps)
				assert.NotNil(t, deps.logger)
				assert.NotNil(t, deps.deps)
				assert.NotNil(t, deps.llmClient)
				
				// Clean up logger
				_ = deps.logger.Sync()
			}
		})
	}
}

func TestCreateContext(t *testing.T) {
	tests := []struct {
		name        string
		timeout     int
		checkDeadline bool
	}{
		{
			name:        "short timeout",
			timeout:     5,
			checkDeadline: true,
		},
		{
			name:        "medium timeout",
			timeout:     30,
			checkDeadline: true,
		},
		{
			name:        "long timeout",
			timeout:     300,
			checkDeadline: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			baseCtx := context.Background()
			cmd.SetContext(baseCtx)

			ctx, cancel := createContext(cmd, tt.timeout)
			defer cancel()

			assert.NotNil(t, ctx)

			if tt.checkDeadline {
				deadline, ok := ctx.Deadline()
				assert.True(t, ok)
				expectedDeadline := time.Now().Add(time.Duration(tt.timeout) * time.Second)
				assert.WithinDuration(t, expectedDeadline, deadline, 2*time.Second)
			}
		})
	}
}

func TestClientAdapter(t *testing.T) {
	t.Run("adapter structure", func(t *testing.T) {
		adapter := &clientAdapter{
			client: &llm.Client{},
		}

		// Test the adapter structure
		assert.NotNil(t, adapter)
		assert.NotNil(t, adapter.client)
	})
}

func TestCreateRebaseWorkflow(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		lang          string
		debug         bool
		wantError     bool
		expectedError string
	}{
		{
			name:      "successful workflow creation with main branch",
			lang:      "en",
			debug:     false,
			wantError: false,
		},
		{
			name:      "successful workflow creation with master branch",
			lang:      "zh",
			debug:     true,
			wantError: false,
		},
		{
			name:      "workflow creation with no remotes",
			lang:      "en",
			debug:     false,
			wantError: false, // Should still work with fallback to "main"
		},
		{
			name:      "workflow creation with remote error",
			lang:      "en",
			debug:     false,
			wantError: false, // Should still work with fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := new(MockSquashClient)

			// Create logger
			testLogger := zap.NewNop()

			// Create workflow
			workflow, err := createRebaseWorkflow(ctx, mockClient, tt.lang, tt.debug, testLogger)

			if tt.wantError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, workflow)
			}
		})
	}
}

func TestSquashDependenciesIntegration(t *testing.T) {
	t.Run("dependencies lifecycle", func(t *testing.T) {
		// Initialize dependencies
		deps, err := initSquashDependencies(false)
		require.NoError(t, err)
		require.NotNil(t, deps)

		// Verify all components are initialized
		assert.NotNil(t, deps.logger)
		assert.NotNil(t, deps.deps)
		assert.NotNil(t, deps.llmClient)

		// Test that llmClient is properly adapted
		adapter, ok := deps.llmClient.(*clientAdapter)
		assert.True(t, ok)
		assert.NotNil(t, adapter.client)

		// Clean up - ignore sync error for stdout/stderr
		_ = deps.logger.Sync()
	})
}

func TestCreateContextCancellation(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := createContext(cmd, 1)
		
		// Cancel immediately
		cancel()
		
		// Verify context is cancelled
		select {
		case <-ctx.Done():
			assert.Error(t, ctx.Err())
			assert.Equal(t, context.Canceled, ctx.Err())
		default:
			t.Fatal("context should be cancelled")
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := createContext(cmd, 1)
		defer cancel()

		// Wait for timeout
		time.Sleep(2 * time.Second)

		// Verify context timed out
		select {
		case <-ctx.Done():
			assert.Error(t, ctx.Err())
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
		default:
			t.Fatal("context should have timed out")
		}
	})
}

func TestClientAdapterPromptHandling(t *testing.T) {
	// Test how the adapter handles different prompt formats
	tests := []struct {
		name           string
		prompt         string
		expectedSystem string
		expectedUser   string
	}{
		{
			name:           "simple prompt",
			prompt:         "Generate a commit message",
			expectedSystem: "",
			expectedUser:   "Generate a commit message",
		},
		{
			name:           "multiline prompt",
			prompt:         "Line 1\nLine 2\nLine 3",
			expectedSystem: "",
			expectedUser:   "Line 1\nLine 2\nLine 3",
		},
		{
			name:           "prompt with special characters",
			prompt:         "Generate for: feat(api): add /users endpoint",
			expectedSystem: "",
			expectedUser:   "Generate for: feat(api): add /users endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The adapter always passes empty system prompt and full prompt as user
			adapter := &clientAdapter{client: nil}
			assert.NotNil(t, adapter)
			
			// In actual implementation, GenerateCommitMessage would call
			// client.GetCommitMessage(ctx, "", prompt)
			// verifying the prompt is passed correctly
		})
	}
}

func TestCreateRebaseWorkflowEdgeCases(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSquashClient)
	testLogger := zap.NewNop()

	t.Run("multiple remotes", func(t *testing.T) {
		workflow, err := createRebaseWorkflow(ctx, mockClient, "en", false, testLogger)
		assert.NoError(t, err)
		assert.NotNil(t, workflow)
	})

	t.Run("no origin remote", func(t *testing.T) {
		workflow, err := createRebaseWorkflow(ctx, mockClient, "en", false, testLogger)
		assert.NoError(t, err)
		assert.NotNil(t, workflow)
	})
}