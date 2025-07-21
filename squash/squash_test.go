package squash

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockClient 是 ClientInterface 的 mock 实现
type MockClient struct {
	mock.Mock
}

func (m *MockClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
}

func TestSquash_Generate(t *testing.T) {
	tests := []struct {
		name          string
		messages      []string
		lang          string
		mockResponse  string
		mockError     error
		expectedError string
	}{
		{
			name: "successful generation in English",
			messages: []string{
				"feat: add user authentication",
				"fix: resolve login error",
				"docs: update auth guide",
			},
			lang:         "en",
			mockResponse: "feat: implement complete authentication system\n\n- Add user authentication\n- Fix login errors\n- Update documentation",
			mockError:    nil,
		},
		{
			name: "successful generation in Chinese",
			messages: []string{
				"feat: 添加用户认证",
				"fix: 修复登录错误",
			},
			lang:         "zh",
			mockResponse: "feat: 实现完整的认证系统\n\n- 添加用户认证功能\n- 修复登录相关错误",
			mockError:    nil,
		},
		{
			name:          "error with less than 2 messages",
			messages:      []string{"feat: single commit"},
			lang:          "en",
			expectedError: "at least 2 commit messages are required",
		},
		{
			name: "error from LLM client",
			messages: []string{
				"feat: add feature",
				"fix: fix bug",
			},
			lang:          "en",
			mockError:     errors.New("API error"),
			expectedError: "failed to generate commit message: API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockClient)
			s := New(mockClient, tt.lang)

			if len(tt.messages) >= 2 && tt.mockError == nil {
				mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
					Return(tt.mockResponse, tt.mockError)
			} else if tt.mockError != nil {
				mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
					Return("", tt.mockError)
			}

			ctx := context.Background()
			result, err := s.Generate(ctx, tt.messages)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, strings.TrimSpace(tt.mockResponse), result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSquash_ValidateMessages(t *testing.T) {
	s := New(nil, "en")

	tests := []struct {
		name          string
		messages      []string
		expectedError string
	}{
		{
			name: "valid messages",
			messages: []string{
				"feat: add feature",
				"fix: fix bug",
				"docs: update docs",
			},
			expectedError: "",
		},
		{
			name:          "less than 2 messages",
			messages:      []string{"feat: single message"},
			expectedError: "at least 2 commit messages are required",
		},
		{
			name:          "empty message",
			messages:      []string{"feat: add feature", "   ", "fix: fix bug"},
			expectedError: "commit message 2 is empty",
		},
		{
			name: "messages too long",
			messages: []string{
				strings.Repeat("a", 6001),
				strings.Repeat("b", 6001),
			},
			expectedError: "total input is too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.ValidateMessages(tt.messages)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSquash_buildPrompt(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		messages []string
		expected string
	}{
		{
			name: "English prompt",
			lang: "en",
			messages: []string{
				"feat: add feature",
				"fix: fix bug",
			},
			expected: "Input commit messages:\n1. feat: add feature\n2. fix: fix bug",
		},
		{
			name: "Chinese prompt",
			lang: "zh",
			messages: []string{
				"feat: 添加功能",
				"fix: 修复错误",
			},
			expected: "输入的提交信息：\n1. feat: 添加功能\n2. fix: 修复错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(nil, tt.lang)
			prompt := s.buildPrompt(tt.messages)
			assert.Contains(t, prompt, tt.expected)
		})
	}
}