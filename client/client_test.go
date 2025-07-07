package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_GetCommitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		apiKey               string
		timeout              time.Duration
		handler              http.HandlerFunc
		expectedMsg          string
		expectedErrSubstring string
	}{
		{
			name:    "successful_completion",
			apiKey:  "valid_key",
			timeout: 2 * time.Second,
			handler: func(w http.ResponseWriter, r *http.Request) {
				// 验证鉴权头是否带入（简单检查，不影响功能）。
				require.Equal(t, "Bearer valid_key", r.Header.Get("Authorization"))
				
				// 验证请求体结构（旧版本使用单个 user 消息）
				body, _ := io.ReadAll(r.Body)
				var req chatRequest
				json.Unmarshal(body, &req)
				require.Len(t, req.Messages, 1)
				require.Equal(t, "user", req.Messages[0].Role)
				
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: test commit"}}]}`))
			},
			expectedMsg:          "feat: test commit",
			expectedErrSubstring: "",
		},
		{
			name:    "api_key_error",
			apiKey:  "invalid_key",
			timeout: 2 * time.Second,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error": "Invalid API Key"}`))
			},
			expectedMsg:          "",
			expectedErrSubstring: "API error: status 401",
		},
		{
			name:    "request_timeout",
			apiKey:  "valid_key",
			timeout: 2 * time.Second, // 不再使用此超时，但保留用于向后兼容
			handler: func(w http.ResponseWriter, r *http.Request) {
				// 故意延迟以触发超时
				time.Sleep(20 * time.Millisecond)
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: should timeout"}}]}`))
			},
			expectedMsg:          "",
			expectedErrSubstring: "context deadline exceeded",
		},
	}

	for _, tc := range tests {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(tc.handler)
			defer server.Close()

			c := NewClient(server.URL, tc.apiKey, tc.timeout, nil)

			// 对于超时测试，使用带超时的 context
			ctx := context.Background()
			if tc.name == "request_timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}

			msg, err := c.GetCommitMessageLegacy(ctx, "test prompt")

			if tc.expectedErrSubstring == "" {
				require.NoError(t, err)
				require.Equal(t, tc.expectedMsg, msg)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErrSubstring)
			}
		})
	}
}

// 新增测试 - 测试新的系统+用户消息结构
func Test_GetCommitMessage_SystemUserPrompts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		apiKey               string
		timeout              time.Duration
		handler              http.HandlerFunc
		expectedMsg          string
		expectedErrSubstring string
	}{
		{
			name:    "successful_system_user_messages",
			apiKey:  "valid_key",
			timeout: 2 * time.Second,
			handler: func(w http.ResponseWriter, r *http.Request) {
				// 验证鉴权头
				require.Equal(t, "Bearer valid_key", r.Header.Get("Authorization"))
				
				// 验证请求体结构（新版本使用 system + user 消息）
				body, _ := io.ReadAll(r.Body)
				var req chatRequest
				json.Unmarshal(body, &req)
				
				// 验证消息结构
				require.Len(t, req.Messages, 2)
				require.Equal(t, "system", req.Messages[0].Role)
				require.Equal(t, "user", req.Messages[1].Role)
				
				// 验证系统消息包含关键词
				require.Contains(t, req.Messages[0].Content, "expert software engineer")
				require.Contains(t, req.Messages[0].Content, "Conventional Commits")
				
				// 验证用户消息包含上下文数据
				require.Contains(t, req.Messages[1].Content, "test-branch")
				require.Contains(t, req.Messages[1].Content, "test.go")
				
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: add new feature"}}]}`))
			},
			expectedMsg:          "feat: add new feature",
			expectedErrSubstring: "",
		},
		{
			name:    "api_error_with_system_user",
			apiKey:  "invalid_key",
			timeout: 2 * time.Second,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error": "Invalid request"}`))
			},
			expectedMsg:          "",
			expectedErrSubstring: "API error: status 400",
		},
	}

	for _, tc := range tests {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(tc.handler)
			defer server.Close()

			c := NewClient(server.URL, tc.apiKey, tc.timeout, nil)

			// 使用新的系统+用户消息方法
			systemPrompt := "You are an expert software engineer and a master of writing concise, high-quality Git commit messages. You adhere strictly to the Conventional Commits specification."
			userPrompt := "Branch: test-branch\nChanged files: test.go\n\nGit diff:\n```diff\n+fmt.Println(\"hello\")\n```"
			
			msg, err := c.GetCommitMessage(context.Background(), systemPrompt, userPrompt)

			if tc.expectedErrSubstring == "" {
				require.NoError(t, err)
				require.Equal(t, tc.expectedMsg, msg)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErrSubstring)
			}
		})
	}
}

// 验证向后兼容性测试
func Test_GetCommitMessageLegacy_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		// 验证旧版本使用单个 user 消息
		body, _ := io.ReadAll(r.Body)
		var req chatRequest
		json.Unmarshal(body, &req)
		
		require.Len(t, req.Messages, 1)
		require.Equal(t, "user", req.Messages[0].Role)
		require.Equal(t, "legacy test prompt", req.Messages[0].Content)
		
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: legacy test"}}]}`))
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	c := NewClient(server.URL, "test_key", 2*time.Second, nil)
	msg, err := c.GetCommitMessageLegacy(context.Background(), "legacy test prompt")

	require.NoError(t, err)
	require.Equal(t, "feat: legacy test", msg)
}
