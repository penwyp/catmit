package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestErrorHandler_HandlePRError 测试PR错误处理
func TestErrorHandler_HandlePRError(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		expectedMessage    string
		expectedSuggestion string
		expectedExitCode   int
	}{
		{
			name:               "CLI not installed",
			err:                fmt.Errorf("gh is not installed"),
			expectedMessage:    "GitHub CLI (gh) is not installed",
			expectedSuggestion: "brew install gh",
			expectedExitCode:   ExitCodeCLINotInstalled,
		},
		{
			name:               "CLI not authenticated",
			err:                fmt.Errorf("gh is not authenticated"),
			expectedMessage:    "GitHub CLI (gh) is not authenticated",
			expectedSuggestion: "gh auth login",
			expectedExitCode:   ExitCodeCLINotAuthenticated,
		},
		{
			name:               "tea not installed",
			err:                fmt.Errorf("tea is not installed"),
			expectedMessage:    "Gitea CLI (tea) is not installed",
			expectedSuggestion: "go install gitea.com/gitea/tea@latest",
			expectedExitCode:   ExitCodeCLINotInstalled,
		},
		{
			name:               "tea not authenticated",
			err:                fmt.Errorf("tea is not authenticated"),
			expectedMessage:    "Gitea CLI (tea) is not authenticated",
			expectedSuggestion: "tea login add",
			expectedExitCode:   ExitCodeCLINotAuthenticated,
		},
		{
			name:               "PR already exists",
			err:                fmt.Errorf("a pull request for branch \"feature\" into branch \"main\" already exists"),
			expectedMessage:    "A pull request already exists for this branch",
			expectedSuggestion: "View existing PRs",
			expectedExitCode:   ExitCodePRAlreadyExists,
		},
		{
			name:               "Network error",
			err:                fmt.Errorf("Post \"https://api.github.com/repos/owner/repo/pulls\": dial tcp: lookup api.github.com: no such host"),
			expectedMessage:    "Network error occurred",
			expectedSuggestion: "Check your internet connection",
			expectedExitCode:   ExitCodeNetworkError,
		},
		{
			name:               "Permission denied",
			err:                fmt.Errorf("HTTP 403: Resource not accessible by integration"),
			expectedMessage:    "Permission denied",
			expectedSuggestion: "Check repository permissions",
			expectedExitCode:   ExitCodePermissionDenied,
		},
		{
			name:               "Generic error",
			err:                fmt.Errorf("something went wrong"),
			expectedMessage:    "Error: something went wrong",
			expectedSuggestion: "",
			expectedExitCode:   ExitCodeGenericError,
		},
		{
			name:               "Unsupported provider",
			err:                fmt.Errorf("unsupported provider: gitlab"),
			expectedMessage:    "GitLab is not supported yet",
			expectedSuggestion: "Supported providers: GitHub, Gitea",
			expectedExitCode:   ExitCodeUnsupportedProvider,
		},
		{
			name:               "No remote",
			err:                fmt.Errorf("failed to get remote URL: remote 'origin' not found"),
			expectedMessage:    "Git remote 'origin' not found",
			expectedSuggestion: "git remote add origin <url>",
			expectedExitCode:   ExitCodeGitError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler()
			prError := handler.HandlePRError(tt.err)
			
			assert.Equal(t, tt.expectedMessage, prError.Message)
			assert.Contains(t, prError.Suggestion, tt.expectedSuggestion)
			assert.Equal(t, tt.expectedExitCode, prError.ExitCode)
		})
	}
}

// TestErrorHandler_FormatError 测试错误格式化
func TestErrorHandler_FormatError(t *testing.T) {
	tests := []struct {
		name           string
		prError        PRError
		expectedOutput []string
	}{
		{
			name: "Error with suggestion",
			prError: PRError{
				Message:    "GitHub CLI (gh) is not installed",
				Suggestion: "Install with:\n  brew install gh\n  https://github.com/cli/cli#installation",
				ExitCode:   ExitCodeCLINotInstalled,
			},
			expectedOutput: []string{
				"Error: GitHub CLI (gh) is not installed",
				"Install with:",
				"brew install gh",
			},
		},
		{
			name: "Error without suggestion",
			prError: PRError{
				Message:  "Something went wrong",
				ExitCode: ExitCodeGenericError,
			},
			expectedOutput: []string{
				"Error: Something went wrong",
			},
		},
		{
			name: "Error with details",
			prError: PRError{
				Message:    "Failed to create PR",
				Details:    "HTTP 403: Forbidden",
				Suggestion: "Check your authentication status with: gh auth status",
				ExitCode:   ExitCodePermissionDenied,
			},
			expectedOutput: []string{
				"Error: Failed to create PR",
				"Details: HTTP 403: Forbidden",
				"Check your authentication status with:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler()
			output := handler.FormatError(tt.prError)
			
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

// TestErrorHandler_IsRetryableError 测试可重试错误判断
func TestErrorHandler_IsRetryableError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedRetry bool
	}{
		{
			name:         "Network timeout",
			err:          fmt.Errorf("request timeout"),
			expectedRetry: true,
		},
		{
			name:         "Connection refused",
			err:          fmt.Errorf("connection refused"),
			expectedRetry: true,
		},
		{
			name:         "DNS error",
			err:          fmt.Errorf("no such host"),
			expectedRetry: true,
		},
		{
			name:         "Authentication error",
			err:          fmt.Errorf("401 Unauthorized"),
			expectedRetry: false,
		},
		{
			name:         "Permission error",
			err:          fmt.Errorf("403 Forbidden"),
			expectedRetry: false,
		},
		{
			name:         "PR already exists",
			err:          fmt.Errorf("pull request already exists"),
			expectedRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler()
			isRetryable := handler.IsRetryableError(tt.err)
			assert.Equal(t, tt.expectedRetry, isRetryable)
		})
	}
}

// TestErrorHandler_WrapError 测试错误包装
func TestErrorHandler_WrapError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		context        string
		expectedPrefix string
	}{
		{
			name:           "Wrap simple error",
			err:            errors.New("file not found"),
			context:        "reading config",
			expectedPrefix: "reading config:",
		},
		{
			name:           "Wrap nil error",
			err:            nil,
			context:        "some operation",
			expectedPrefix: "",
		},
		{
			name:           "Wrap formatted error",
			err:            fmt.Errorf("failed to connect to %s", "github.com"),
			context:        "creating PR",
			expectedPrefix: "creating PR:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler()
			wrapped := handler.WrapError(tt.err, tt.context)
			
			if tt.err == nil {
				assert.Nil(t, wrapped)
			} else {
				assert.NotNil(t, wrapped)
				assert.True(t, strings.HasPrefix(wrapped.Error(), tt.expectedPrefix))
				assert.Contains(t, wrapped.Error(), tt.err.Error())
			}
		})
	}
}