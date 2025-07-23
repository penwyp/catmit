package squash

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockClient is a mock implementation of ClientInterface
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
			expected: "1. feat: 添加功能\n2. fix: 修复错误",
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

func TestCleanCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no cleaning needed",
			input:    "feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - consolidated",
			input:    "Here's the consolidated commit message: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - here is",
			input:    "Here is the consolidated commit message: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - the consolidated",
			input:    "The consolidated commit message is: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - consolidated only",
			input:    "Consolidated commit message: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - here's commit",
			input:    "Here's the commit message: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - here is commit",
			input:    "Here is the commit message: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove English prefix - the commit",
			input:    "The commit message is: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "remove Chinese prefix - 合并后的提交信息",
			input:    "合并后的提交信息：feat: 添加新功能",
			expected: "feat: 添加新功能",
		},
		{
			name:     "remove Chinese prefix - 这是合并后的提交信息",
			input:    "这是合并后的提交信息：feat: 添加新功能",
			expected: "feat: 添加新功能",
		},
		{
			name:     "remove Chinese prefix - 提交信息",
			input:    "提交信息：feat: 添加新功能",
			expected: "feat: 添加新功能",
		},
		{
			name:     "remove Chinese prefix - 以下是合并后的提交信息",
			input:    "以下是合并后的提交信息：feat: 添加新功能",
			expected: "feat: 添加新功能",
		},
		{
			name:     "remove Chinese prefix - 生成的提交信息",
			input:    "生成的提交信息：feat: 添加新功能",
			expected: "feat: 添加新功能",
		},
		{
			name:     "remove double quotes",
			input:    "\"feat: add new feature\"",
			expected: "feat: add new feature",
		},
		{
			name:     "remove single quotes",
			input:    "'feat: add new feature'",
			expected: "feat: add new feature",
		},
		{
			name:     "remove backticks",
			input:    "`feat: add new feature`",
			expected: "feat: add new feature",
		},
		{
			name:     "prefix and quotes combined",
			input:    "Here's the commit message: \"feat: add new feature\"",
			expected: "feat: add new feature",
		},
		{
			name:     "case insensitive prefix matching",
			input:    "HERE'S THE COMMIT MESSAGE: feat: add new feature",
			expected: "feat: add new feature",
		},
		{
			name:     "whitespace handling",
			input:    "  Here's the commit message:   feat: add new feature  ",
			expected: "feat: add new feature",
		},
		{
			name:     "short string - no quotes removal",
			input:    "ab",
			expected: "ab",
		},
		{
			name:     "mismatched quotes",
			input:    "\"feat: add new feature'",
			expected: "\"feat: add new feature'",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "prefix without colon",
			input:    "Here's the commit message feat: add new feature",
			expected: "Here's the commit message feat: add new feature",
		},
		{
			name:     "multiple prefixes - only first removed",
			input:    "Here's the commit message: The commit message is: feat: add new feature",
			expected: "The commit message is: feat: add new feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Access the unexported function through the squash type
			s := New(nil, "en")
			// We'll need to test this indirectly through the Generate method
			// or make the function exported for testing
			
			// For now, we test it through a mock that returns the input with prefix
			mockClient := new(MockClient)
			s = New(mockClient, "en")
			
			// Mock returns the input string
			mockClient.On("GenerateCommitMessage", mock.Anything, mock.Anything).
				Return(tt.input, nil)
			
			// Generate will clean the message
			result, err := s.Generate(context.Background(), []string{"commit 1", "commit 2"})
			
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
