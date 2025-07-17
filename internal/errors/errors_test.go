package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCatmitError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CatmitError
		expected string
	}{
		{
			name: "error without cause",
			err: &CatmitError{
				Type:    ErrTypeGit,
				Message: "git operation failed",
			},
			expected: "git operation failed",
		},
		{
			name: "error with cause",
			err: &CatmitError{
				Type:    ErrTypeGit,
				Message: "git operation failed",
				Cause:   errors.New("permission denied"),
			},
			expected: "git operation failed: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestCatmitError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &CatmitError{
		Type:    ErrTypeGit,
		Message: "wrapper error",
		Cause:   cause,
	}

	assert.Equal(t, cause, err.Unwrap())
}

func TestCatmitError_WithSuggestion(t *testing.T) {
	err := New(ErrTypeGit, "test error")
	suggestion := "try this solution"
	
	result := err.WithSuggestion(suggestion)
	
	assert.Equal(t, suggestion, result.Suggestion)
	assert.Same(t, err, result)
}

func TestCatmitError_IsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      *CatmitError
		expected bool
	}{
		{
			name: "retryable error",
			err: &CatmitError{
				Type:      ErrTypeNetwork,
				Message:   "network error",
				Retryable: true,
			},
			expected: true,
		},
		{
			name: "non-retryable error",
			err: &CatmitError{
				Type:      ErrTypeGit,
				Message:   "git error",
				Retryable: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.IsRetryable())
		})
	}
}

func TestNew(t *testing.T) {
	err := New(ErrTypeGit, "test message")
	
	assert.Equal(t, ErrTypeGit, err.Type)
	assert.Equal(t, "test message", err.Message)
	assert.Nil(t, err.Cause)
	assert.False(t, err.Retryable)
}

func TestWrap(t *testing.T) {
	cause := errors.New("original error")
	err := Wrap(ErrTypeGit, "wrapped message", cause)
	
	assert.Equal(t, ErrTypeGit, err.Type)
	assert.Equal(t, "wrapped message", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.False(t, err.Retryable)
}

func TestNewRetryable(t *testing.T) {
	err := NewRetryable(ErrTypeNetwork, "network error")
	
	assert.Equal(t, ErrTypeNetwork, err.Type)
	assert.Equal(t, "network error", err.Message)
	assert.Nil(t, err.Cause)
	assert.True(t, err.Retryable)
}

func TestWrapRetryable(t *testing.T) {
	cause := errors.New("timeout")
	err := WrapRetryable(ErrTypeTimeout, "request timeout", cause)
	
	assert.Equal(t, ErrTypeTimeout, err.Type)
	assert.Equal(t, "request timeout", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *CatmitError
		errType    ErrorType
		retryable  bool
		suggestion string
	}{
		{
			name:       "ErrNoGitRepo",
			err:        ErrNoGitRepo,
			errType:    ErrTypeGit,
			retryable:  false,
			suggestion: "请在 Git 仓库中运行此命令",
		},
		{
			name:       "ErrNetworkTimeout",
			err:        ErrNetworkTimeout,
			errType:    ErrTypeTimeout,
			retryable:  true,
			suggestion: "检查网络连接并重试",
		},
		{
			name:       "ErrLLMAPIKey",
			err:        ErrLLMAPIKey,
			errType:    ErrTypeLLM,
			retryable:  false,
			suggestion: "设置环境变量 CATMIT_LLM_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.errType, tt.err.Type)
			assert.Equal(t, tt.retryable, tt.err.Retryable)
			assert.Equal(t, tt.suggestion, tt.err.Suggestion)
		})
	}
}

func TestIs(t *testing.T) {
	err1 := New(ErrTypeGit, "error 1")
	err2 := fmt.Errorf("wrapped: %w", err1)
	
	assert.True(t, Is(err2, err1))
	assert.False(t, Is(err1, ErrNoGitRepo))
}

func TestAs(t *testing.T) {
	originalErr := &CatmitError{
		Type:    ErrTypeGit,
		Message: "git error",
	}
	wrappedErr := fmt.Errorf("wrapped: %w", originalErr)
	
	var target *CatmitError
	assert.True(t, As(wrappedErr, &target))
	assert.Equal(t, originalErr, target)
}

func TestGetType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{
			name:     "CatmitError",
			err:      New(ErrTypeGit, "test"),
			expected: ErrTypeGit,
		},
		{
			name:     "wrapped CatmitError",
			err:      fmt.Errorf("wrapped: %w", New(ErrTypeNetwork, "test")),
			expected: ErrTypeNetwork,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: ErrTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetType(tt.err))
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "retryable CatmitError",
			err:      NewRetryable(ErrTypeNetwork, "network error"),
			expected: true,
		},
		{
			name:     "non-retryable CatmitError",
			err:      New(ErrTypeGit, "git error"),
			expected: false,
		},
		{
			name:     "wrapped retryable error",
			err:      fmt.Errorf("wrapped: %w", NewRetryable(ErrTypeTimeout, "timeout")),
			expected: true,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsRetryable(tt.err))
		})
	}
}

func TestGetSuggestion(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "error with suggestion",
			err:      New(ErrTypeGit, "test").WithSuggestion("try this"),
			expected: "try this",
		},
		{
			name:     "error without suggestion",
			err:      New(ErrTypeGit, "test"),
			expected: "",
		},
		{
			name:     "wrapped error with suggestion",
			err:      fmt.Errorf("wrapped: %w", New(ErrTypeGit, "test").WithSuggestion("help")),
			expected: "help",
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetSuggestion(tt.err))
		})
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "CatmitError without suggestion",
			err:      New(ErrTypeGit, "git error"),
			expected: "git error",
		},
		{
			name:     "CatmitError with suggestion",
			err:      New(ErrTypeGit, "git error").WithSuggestion("try git init"),
			expected: "git error\n💡 try git init",
		},
		{
			name:     "CatmitError with cause and suggestion",
			err:      Wrap(ErrTypeGit, "git failed", errors.New("permission denied")).WithSuggestion("check permissions"),
			expected: "git failed: permission denied\n💡 check permissions",
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: "standard error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatError(tt.err))
		})
	}
}