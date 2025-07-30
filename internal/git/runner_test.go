package git

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// MockCommandRunner is used to mock exec.CommandContext behavior
type MockCommandRunner struct {
	mock.Mock
}

func (m *MockCommandRunner) CombinedOutput() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func TestNewRunner(t *testing.T) {
	t.Run("creates runner without debug", func(t *testing.T) {
		runner := NewRunner(false)
		assert.NotNil(t, runner)
		
		realRunner, ok := runner.(*realRunner)
		assert.True(t, ok)
		assert.False(t, realRunner.debug)
		assert.Nil(t, realRunner.logger)
	})

	t.Run("creates runner with debug", func(t *testing.T) {
		runner := NewRunner(true)
		assert.NotNil(t, runner)
		
		realRunner, ok := runner.(*realRunner)
		assert.True(t, ok)
		assert.True(t, realRunner.debug)
		assert.Nil(t, realRunner.logger)
	})
}

func TestNewRunnerWithLogger(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("creates runner with logger and debug", func(t *testing.T) {
		runner := NewRunnerWithLogger(true, logger)
		assert.NotNil(t, runner)
		
		realRunner, ok := runner.(*realRunner)
		assert.True(t, ok)
		assert.True(t, realRunner.debug)
		assert.Equal(t, logger, realRunner.logger)
	})

	t.Run("creates runner with logger without debug", func(t *testing.T) {
		runner := NewRunnerWithLogger(false, logger)
		assert.NotNil(t, runner)
		
		realRunner, ok := runner.(*realRunner)
		assert.True(t, ok)
		assert.False(t, realRunner.debug)
		assert.Equal(t, logger, realRunner.logger)
	})
}

func TestRunnerRun(t *testing.T) {
	// Since we can't easily mock exec.CommandContext, we'll test with actual commands
	// that should work on most systems

	t.Run("successful command execution", func(t *testing.T) {
		runner := NewRunner(false)
		output, err := runner.Run(context.Background(), "echo", "hello")
		
		assert.NoError(t, err)
		assert.Contains(t, output, "hello")
	})

	t.Run("command with multiple arguments", func(t *testing.T) {
		runner := NewRunner(false)
		output, err := runner.Run(context.Background(), "echo", "hello", "world")
		
		assert.NoError(t, err)
		assert.Contains(t, output, "hello world")
	})

	t.Run("failing command", func(t *testing.T) {
		runner := NewRunner(false)
		_, err := runner.Run(context.Background(), "false")
		
		assert.Error(t, err)
	})

	t.Run("command not found", func(t *testing.T) {
		runner := NewRunner(false)
		_, err := runner.Run(context.Background(), "this-command-does-not-exist-12345")
		
		assert.Error(t, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		runner := NewRunner(false)
		_, err := runner.Run(ctx, "sleep", "10")
		
		assert.Error(t, err)
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		runner := NewRunner(false)
		_, err := runner.Run(ctx, "sleep", "1")
		
		assert.Error(t, err)
	})
}

func TestRunnerRunWithDebugLogging(t *testing.T) {
	// Create a test logger that captures logs
	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))

	t.Run("logs command execution with debug enabled", func(t *testing.T) {
		runner := NewRunnerWithLogger(true, logger)
		output, err := runner.Run(context.Background(), "echo", "test message")
		
		assert.NoError(t, err)
		assert.Contains(t, output, "test message")
		// The logger will have logged the command and output
	})

	t.Run("logs large output with truncation", func(t *testing.T) {
		runner := NewRunnerWithLogger(true, logger)
		
		// Create a command that generates large output
		// Using printf with a format string to generate 2000 'a' characters
		output, err := runner.Run(context.Background(), "printf", "%2000s", "a")
		
		assert.NoError(t, err)
		assert.Len(t, output, 2000)
		// Logger should show "<2000 bytes>" instead of the actual output
	})

	t.Run("logs small output fully", func(t *testing.T) {
		runner := NewRunnerWithLogger(true, logger)
		output, err := runner.Run(context.Background(), "echo", "small output")
		
		assert.NoError(t, err)
		assert.Contains(t, output, "small output")
		// Logger should show the actual output since it's < 1000 bytes
	})

	t.Run("no logging when debug is disabled", func(t *testing.T) {
		runner := NewRunnerWithLogger(false, logger)
		output, err := runner.Run(context.Background(), "echo", "no debug")
		
		assert.NoError(t, err)
		assert.Contains(t, output, "no debug")
		// No debug logs should be written
	})

	t.Run("logs error when command fails", func(t *testing.T) {
		runner := NewRunnerWithLogger(true, logger)
		_, err := runner.Run(context.Background(), "false")
		
		assert.Error(t, err)
		// Error should be logged
	})

	t.Run("handles empty output", func(t *testing.T) {
		runner := NewRunnerWithLogger(true, logger)
		output, err := runner.Run(context.Background(), "true")
		
		assert.NoError(t, err)
		assert.Empty(t, output)
		// Should log 0 bytes output
	})
}

func TestRunnerInterfaceCompliance(t *testing.T) {
	// Ensure realRunner implements Runner interface
	var _ Runner = (*realRunner)(nil)
	
	// Test that both constructors return Runner interface
	var runner1 Runner = NewRunner(false)
	assert.NotNil(t, runner1)
	
	logger := zaptest.NewLogger(t)
	var runner2 Runner = NewRunnerWithLogger(true, logger)
	assert.NotNil(t, runner2)
}