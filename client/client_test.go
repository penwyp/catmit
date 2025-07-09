package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_OpenAICompatibleProvider_GetCompletion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		apiKey               string
		model                string
		handler              http.HandlerFunc
		expectedMsg          string
		expectedErrSubstring string
	}{
		{
			name:   "successful_completion",
			apiKey: "valid_key",
			model:  "test-model",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// 验证鉴权头是否带入（简单检查，不影响功能）。
				require.Equal(t, "Bearer valid_key", r.Header.Get("Authorization"))
				
				// 验证请求体结构（使用 system + user 消息）
				body, _ := io.ReadAll(r.Body)
				var req chatRequest
				_ = json.Unmarshal(body, &req)
				require.Len(t, req.Messages, 2)
				require.Equal(t, "system", req.Messages[0].Role)
				require.Equal(t, "user", req.Messages[1].Role)
				require.Equal(t, "test-model", req.Model)
				
				w.Header().Set("Content-Type", "application/json")
				response := `{
					"id": "04ddb5eb-9727-4e59-af37-7cde0a2c9830",
					"object": "chat.completion",
					"created": 1751859666,
					"model": "test-model",
					"choices": [
						{
							"index": 0,
							"message": {
								"role": "assistant",
								"content": "feat: test commit"
							},
							"logprobs": null,
							"finish_reason": "stop"
						}
					],
					"usage": {
						"prompt_tokens": 45,
						"completion_tokens": 4,
						"total_tokens": 49
					},
					"system_fingerprint": "fp_8802369eaa_prod0623_fp8_kvcache"
				}`
				_, _ = w.Write([]byte(response))
			},
			expectedMsg:          "feat: test commit",
			expectedErrSubstring: "",
		},
		{
			name:   "api_key_error",
			apiKey: "invalid_key",
			model:  "test-model",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error": "Invalid API Key"}`))
			},
			expectedMsg:          "",
			expectedErrSubstring: "API error: status 401",
		},
		{
			name:   "request_timeout",
			apiKey: "valid_key",
			model:  "test-model",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// 故意延迟以触发超时
				time.Sleep(20 * time.Millisecond)
				response := `{
					"id": "timeout-test-id",
					"object": "chat.completion",
					"created": 1751859666,
					"model": "test-model",
					"choices": [
						{
							"index": 0,
							"message": {
								"role": "assistant",
								"content": "feat: should timeout"
							},
							"logprobs": null,
							"finish_reason": "stop"
						}
					],
					"usage": {
						"prompt_tokens": 20,
						"completion_tokens": 5,
						"total_tokens": 25
					},
					"system_fingerprint": "fp_8802369eaa_prod0623_fp8_kvcache"
				}`
				_, _ = w.Write([]byte(response))
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

			// 创建 Provider 实例
			provider := &OpenAICompatibleProvider{
				apiURL:     server.URL,
				apiKey:     tc.apiKey,
				model:      tc.model,
				httpClient: &http.Client{},
			}

			// 对于超时测试，使用带超时的 context
			ctx := context.Background()
			if tc.name == "request_timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}

			msg, err := provider.GetCompletion(ctx, "test system prompt", "test user prompt")

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

func Test_Client_GetCommitMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证鉴权头
		require.Equal(t, "Bearer test_key", r.Header.Get("Authorization"))
		
		// 验证请求体结构
		body, _ := io.ReadAll(r.Body)
		var req chatRequest
		_ = json.Unmarshal(body, &req)
		
		// 验证消息结构
		require.Len(t, req.Messages, 2)
		require.Equal(t, "system", req.Messages[0].Role)
		require.Equal(t, "user", req.Messages[1].Role)
		require.Equal(t, "test-model", req.Model)
		
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"id": "client-test-id",
			"object": "chat.completion",
			"created": 1751859666,
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "feat: add new feature"
					},
					"logprobs": null,
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 120,
				"completion_tokens": 8,
				"total_tokens": 128
			},
			"system_fingerprint": "fp_8802369eaa_prod0623_fp8_kvcache"
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	// 创建自定义 Provider
	provider := &OpenAICompatibleProvider{
		apiURL:     server.URL,
		apiKey:     "test_key",
		model:      "test-model",
		httpClient: &http.Client{},
	}

	client := NewClientWithProvider(provider, nil)
	
	msg, err := client.GetCommitMessage(context.Background(), 
		"You are an expert software engineer and a master of writing concise, high-quality Git commit messages.", 
		"Branch: test-branch\nChanged files: test.go\n\nGit diff:\n```diff\n+fmt.Println(\"hello\")\n```")

	require.NoError(t, err)
	require.Equal(t, "feat: add new feature", msg)
}

func Test_NewOpenAICompatibleProvider_EnvironmentVariables(t *testing.T) {
	t.Parallel()

	// 测试默认值
	t.Run("default_values", func(t *testing.T) {
		// 清理环境变量
		os.Unsetenv("CATMIT_LLM_API_URL")
		os.Unsetenv("CATMIT_LLM_API_KEY")
		os.Unsetenv("CATMIT_LLM_MODEL")
		
		provider := NewOpenAICompatibleProvider()
		
		require.Equal(t, "https://api.deepseek.com/v1/chat/completions", provider.apiURL)
		require.Equal(t, "", provider.apiKey)
		require.Equal(t, "deepseek-chat", provider.model)
	})

	// 测试自定义值
	t.Run("custom_values", func(t *testing.T) {
		os.Setenv("CATMIT_LLM_API_URL", "https://ark.cn-beijing.volces.com/api/v3/chat/completions")
		os.Setenv("CATMIT_LLM_API_KEY", "96aba69f-69b1-4a62-bce5-53bc1a721176")
		os.Setenv("CATMIT_LLM_MODEL", "deepseek-v3-250324")
		defer func() {
			os.Unsetenv("CATMIT_LLM_API_URL")
			os.Unsetenv("CATMIT_LLM_API_KEY")
			os.Unsetenv("CATMIT_LLM_MODEL")
		}()
		
		provider := NewOpenAICompatibleProvider()
		
		require.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/chat/completions", provider.apiURL)
		require.Equal(t, "96aba69f-69b1-4a62-bce5-53bc1a721176", provider.apiKey)
		require.Equal(t, "deepseek-v3-250324", provider.model)
	})
}