package errors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultHandler_Handle(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		verbose     bool
		expectNil   bool
		expectType  ErrorType
	}{
		{
			name:      "nil error",
			err:       nil,
			verbose:   false,
			expectNil: true,
		},
		{
			name:       "CatmitError",
			err:        New(ErrTypeGit, "git error"),
			verbose:    false,
			expectNil:  false,
			expectType: ErrTypeGit,
		},
		{
			name:       "standard error - git",
			err:        errors.New("not a git repository"),
			verbose:    false,
			expectNil:  false,
			expectType: ErrTypeGit,
		},
		{
			name:       "standard error - network",
			err:        errors.New("connection timeout"),
			verbose:    true,
			expectNil:  false,
			expectType: ErrTypeTimeout,
		},
		{
			name:       "standard error - auth",
			err:        errors.New("unauthorized access"),
			verbose:    false,
			expectNil:  false,
			expectType: ErrTypeAuth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.verbose)
			result := handler.Handle(tt.err)
			
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				var catmitErr *CatmitError
				assert.True(t, As(result, &catmitErr))
				assert.Equal(t, tt.expectType, catmitErr.Type)
			}
		})
	}
}

func TestDefaultHandler_HandleWithRetry(t *testing.T) {
	t.Run("non-retryable error", func(t *testing.T) {
		handler := NewHandler(false)
		err := New(ErrTypeGit, "git error")
		callCount := 0
		
		result := handler.HandleWithRetry(context.Background(), err, func() error {
			callCount++
			return err
		})
		
		assert.NotNil(t, result)
		assert.Equal(t, 0, callCount) // 不应该调用操作函数
	})
	
	t.Run("retryable error - success on retry", func(t *testing.T) {
		handler := &DefaultHandler{
			MaxRetries:    3,
			RetryInterval: time.Millisecond,
			Verbose:       false,
		}
		err := NewRetryable(ErrTypeNetwork, "network error")
		callCount := 0
		
		result := handler.HandleWithRetry(context.Background(), err, func() error {
			callCount++
			if callCount == 2 {
				return nil // 第二次成功
			}
			return err
		})
		
		assert.Nil(t, result)
		assert.Equal(t, 2, callCount)
	})
	
	t.Run("retryable error - all retries fail", func(t *testing.T) {
		handler := &DefaultHandler{
			MaxRetries:    2,
			RetryInterval: time.Millisecond,
			Verbose:       false,
		}
		err := NewRetryable(ErrTypeNetwork, "network error")
		callCount := 0
		
		result := handler.HandleWithRetry(context.Background(), err, func() error {
			callCount++
			return err
		})
		
		assert.NotNil(t, result)
		assert.Equal(t, 2, callCount) // 初始尝试 + 1次重试
		
		var catmitErr *CatmitError
		assert.True(t, As(result, &catmitErr))
		assert.Contains(t, catmitErr.Message, "2 次重试后失败")
	})
	
	t.Run("context cancelled", func(t *testing.T) {
		handler := &DefaultHandler{
			MaxRetries:    3,
			RetryInterval: time.Second,
			Verbose:       false,
		}
		err := NewRetryable(ErrTypeNetwork, "network error")
		
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消
		
		result := handler.HandleWithRetry(ctx, err, func() error {
			return err
		})
		
		assert.NotNil(t, result)
	})
	
	t.Run("nil operation", func(t *testing.T) {
		handler := NewHandler(false)
		err := NewRetryable(ErrTypeNetwork, "network error")
		
		result := handler.HandleWithRetry(context.Background(), err, nil)
		
		assert.NotNil(t, result)
	})
}

func TestDefaultHandler_inferErrorType(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		expectedMsg  string
		hassSuggestion bool
	}{
		{
			name:         "git repository error",
			err:          errors.New("fatal: not a git repository"),
			expectedType: ErrTypeGit,
			expectedMsg:  "不是 Git 仓库",
			hassSuggestion: true,
		},
		{
			name:         "no changes error",
			err:          errors.New("nothing to commit, working tree clean"),
			expectedType: ErrTypeGit,
			expectedMsg:  "没有需要提交的更改",
			hassSuggestion: true,
		},
		{
			name:         "timeout error",
			err:          errors.New("context deadline exceeded"),
			expectedType: ErrTypeTimeout,
			expectedMsg:  "操作超时",
			hassSuggestion: true,
		},
		{
			name:         "network error",
			err:          errors.New("connection refused"),
			expectedType: ErrTypeNetwork,
			expectedMsg:  "网络错误",
			hassSuggestion: true,
		},
		{
			name:         "auth error",
			err:          errors.New("authentication failed"),
			expectedType: ErrTypeAuth,
			expectedMsg:  "认证失败",
			hassSuggestion: true,
		},
		{
			name:         "rate limit error",
			err:          errors.New("API rate limit exceeded"),
			expectedType: ErrTypeLLM,
			expectedMsg:  "API 速率限制",
			hassSuggestion: true,
		},
		{
			name:         "unknown error",
			err:          errors.New("something went wrong"),
			expectedType: ErrTypeUnknown,
			expectedMsg:  "something went wrong",
			hassSuggestion: false,
		},
	}
	
	handler := &DefaultHandler{}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.inferErrorType(tt.err)
			
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.Equal(t, tt.err, result.Cause)
			
			if tt.hassSuggestion {
				assert.NotEmpty(t, result.Suggestion)
			} else {
				assert.Empty(t, result.Suggestion)
			}
		})
	}
}

func TestDefaultHandler_getErrorIcon(t *testing.T) {
	tests := []struct {
		errType ErrorType
		icon    string
	}{
		{ErrTypeGit, "🔧"},
		{ErrTypeProvider, "🔗"},
		{ErrTypePR, "📝"},
		{ErrTypeConfig, "⚙️"},
		{ErrTypeNetwork, "🌐"},
		{ErrTypeAuth, "🔐"},
		{ErrTypeTimeout, "⏱️"},
		{ErrTypeValidation, "✅"},
		{ErrTypeLLM, "🤖"},
		{ErrTypeUnknown, "❌"},
		{ErrorType(999), "❌"}, // 未知类型
	}
	
	handler := &DefaultHandler{}
	
	for _, tt := range tests {
		t.Run(tt.errType.String(), func(t *testing.T) {
			icon := handler.getErrorIcon(tt.errType)
			assert.Equal(t, tt.icon, icon)
		})
	}
}

// 为 ErrorType 添加 String 方法以便测试输出
func (e ErrorType) String() string {
	names := []string{
		"Unknown", "Git", "Provider", "PR", "Config",
		"Network", "Auth", "Timeout", "Validation", "LLM",
	}
	if int(e) < len(names) {
		return names[e]
	}
	return "Invalid"
}