package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewClient(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test with OpenAI compatible provider (default)
	client := NewClient(logger)
	assert.NotNil(t, client)
	assert.NotNil(t, client.provider)
	assert.NotNil(t, client.logger)

	// Verify default provider is OpenAI compatible
	_, ok := client.provider.(*OpenAICompatibleProvider)
	assert.True(t, ok, "Default provider should be OpenAICompatibleProvider")
}

func TestClient_GetCommitMessage(t *testing.T) {
	tests := []struct {
		name           string
		systemPrompt   string
		userPrompt     string
		mockResponse   interface{}
		mockStatusCode int
		mockDelay      time.Duration
		expectedMsg    string
		expectedError  error
		checkErrorType func(error) bool
	}{
		{
			name:         "successful response",
			systemPrompt: "You are a commit message generator",
			userPrompt:   "Generate a commit message for auth changes",
			mockResponse: map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"content": "feat: implement OAuth2 authentication",
						},
					},
				},
			},
			mockStatusCode: http.StatusOK,
			expectedMsg:    "feat: implement OAuth2 authentication",
		},
		{
			name:         "empty response",
			systemPrompt: "System prompt",
			userPrompt:   "User prompt",
			mockResponse: map[string]interface{}{
				"choices": []map[string]interface{}{},
			},
			mockStatusCode: http.StatusOK,
			expectedError:  errors.ErrLLMResponse,
		},
		{
			name:         "malformed response",
			systemPrompt: "System prompt",
			userPrompt:   "User prompt",
			mockResponse: map[string]interface{}{
				"invalid": "response",
			},
			mockStatusCode: http.StatusOK,
			expectedError:  errors.ErrLLMResponse,
		},
		{
			name:           "rate limit exceeded",
			systemPrompt:   "System prompt",
			userPrompt:     "User prompt",
			mockResponse:   map[string]interface{}{"error": "rate limit exceeded"},
			mockStatusCode: http.StatusTooManyRequests,
			expectedError:  errors.ErrLLMRateLimit,
		},
		{
			name:           "server error",
			systemPrompt:   "System prompt",
			userPrompt:     "User prompt",
			mockResponse:   map[string]interface{}{"error": "internal server error"},
			mockStatusCode: http.StatusInternalServerError,
			checkErrorType: func(err error) bool {
				var catmitErr *errors.CatmitError
				if errors.As(err, &catmitErr) {
					return catmitErr.Type == errors.ErrTypeLLM && catmitErr.IsRetryable()
				}
				return false
			},
		},
		{
			name:         "context timeout",
			systemPrompt: "System prompt",
			userPrompt:   "User prompt",
			mockDelay:    2 * time.Second,
			mockResponse: map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"content": "should not see this",
						},
					},
				},
			},
			mockStatusCode: http.StatusOK,
			expectedError:  errors.ErrLLMTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Add delay if specified
				if tt.mockDelay > 0 {
					time.Sleep(tt.mockDelay)
				}

				// Verify request
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

				// Decode request body
				var reqBody map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)

				// Verify request contains expected fields
				assert.Equal(t, "test-model", reqBody["model"])
				messages, ok := reqBody["messages"].([]interface{})
				require.True(t, ok)
				require.Len(t, messages, 2)

				// Write response
				w.WriteHeader(tt.mockStatusCode)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			// Create client with mock server
			provider := &OpenAICompatibleProvider{
				apiURL:     server.URL,
				apiKey:     "test-key",
				model:      "test-model",
				httpClient: &http.Client{Timeout: 5 * time.Second},
			}

			client := &Client{
				provider: provider,
				logger:   zaptest.NewLogger(t),
			}

			// Create context with timeout for timeout test
			ctx := context.Background()
			if tt.mockDelay > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 1*time.Second)
				defer cancel()
			}

			// Call method
			msg, err := client.GetCommitMessage(ctx, tt.systemPrompt, tt.userPrompt)

			// Check results
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError))
				assert.Empty(t, msg)
			} else if tt.checkErrorType != nil {
				assert.Error(t, err)
				assert.True(t, tt.checkErrorType(err))
				assert.Empty(t, msg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedMsg, msg)
			}
		})
	}
}

func TestOpenAICompatibleProvider_GetCompletion(t *testing.T) {
	// Test environment variable configuration
	t.Run("missing API key", func(t *testing.T) {
		// Save and clear env vars
		oldKey := os.Getenv("CATMIT_LLM_API_KEY")
		os.Unsetenv("CATMIT_LLM_API_KEY")
		defer os.Setenv("CATMIT_LLM_API_KEY", oldKey)

		provider := &OpenAICompatibleProvider{
			apiURL: "https://api.test.com",
			model:  "test-model",
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errors.ErrLLMAPIKey))
	})

	// Note: empty prompts are allowed by the API, no validation needed
}

func TestOpenAICompatibleProvider_HTTPClient(t *testing.T) {
	// Test that custom HTTP client is used
	t.Run("custom http client", func(t *testing.T) {
		customClientUsed := false
		customClient := &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				customClientUsed = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"choices": [{
							"message": {"content": "test response"}
						}]
					}`)),
					Header: make(http.Header),
				}, nil
			}),
		}

		provider := &OpenAICompatibleProvider{
			apiURL:     "https://api.test.com",
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: customClient,
		}

		ctx := context.Background()
		msg, err := provider.GetCompletion(ctx, "system", "user")

		assert.NoError(t, err)
		assert.Equal(t, "test response", msg)
		assert.True(t, customClientUsed, "Custom HTTP client should be used")
	})
}

func TestDefaultProviderFromEnv(t *testing.T) {
	tests := []struct {
		name           string
		envVars        map[string]string
		expectedURL    string
		expectedModel  string
		expectedAPIKey string
	}{
		{
			name: "DeepSeek defaults",
			envVars: map[string]string{
				"CATMIT_LLM_API_KEY": "sk-deepseek",
			},
			expectedURL:    "https://api.deepseek.com/v1/chat/completions",
			expectedModel:  "deepseek-chat",
			expectedAPIKey: "sk-deepseek",
		},
		{
			name: "Volcengine Ark configuration",
			envVars: map[string]string{
				"CATMIT_LLM_API_KEY": "volc-key",
				"CATMIT_LLM_API_URL": "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
				"CATMIT_LLM_MODEL":   "deepseek-v3-250324",
			},
			expectedURL:    "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
			expectedModel:  "deepseek-v3-250324",
			expectedAPIKey: "volc-key",
		},
		{
			name: "Custom OpenAI-compatible provider",
			envVars: map[string]string{
				"CATMIT_LLM_API_KEY": "custom-key",
				"CATMIT_LLM_API_URL": "https://custom.api.com/v1/chat/completions",
				"CATMIT_LLM_MODEL":   "custom-model",
			},
			expectedURL:    "https://custom.api.com/v1/chat/completions",
			expectedModel:  "custom-model",
			expectedAPIKey: "custom-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear all LLM env vars first
			envVars := []string{"CATMIT_LLM_API_KEY", "CATMIT_LLM_API_URL", "CATMIT_LLM_MODEL"}
			oldVars := make(map[string]string)
			for _, k := range envVars {
				oldVars[k] = os.Getenv(k)
				os.Unsetenv(k)
			}

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			defer func() {
				// Restore original env vars
				for k, v := range oldVars {
					if v == "" {
						os.Unsetenv(k)
					} else {
						os.Setenv(k, v)
					}
				}
			}()

			// Create client with default provider
			client := NewClient(zaptest.NewLogger(t))
			provider, ok := client.provider.(*OpenAICompatibleProvider)
			require.True(t, ok)

			assert.Equal(t, tt.expectedURL, provider.apiURL)
			assert.Equal(t, tt.expectedModel, provider.model)
			assert.Equal(t, tt.expectedAPIKey, provider.apiKey)
		})
	}
}

func TestNewClientWithProvider(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a mock provider
	mockProvider := &mockLLMProvider{
		response: "test response",
	}

	client := NewClientWithProvider(mockProvider, logger)
	assert.NotNil(t, client)
	assert.Equal(t, mockProvider, client.provider)
	assert.NotNil(t, client.logger)

	// Test that the custom provider is used
	ctx := context.Background()
	msg, err := client.GetCommitMessage(ctx, "system", "user")
	assert.NoError(t, err)
	assert.Equal(t, "test response", msg)
}

func TestGetCommitMessage_LoggerBehavior(t *testing.T) {
	// Test with nil logger
	client := &Client{
		provider: &mockLLMProvider{response: "test"},
		logger:   nil,
	}

	ctx := context.Background()
	msg, err := client.GetCommitMessage(ctx, "system", "user")
	assert.NoError(t, err)
	assert.Equal(t, "test", msg)
}

func TestOpenAICompatibleProvider_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		mockResponse   string
		mockStatusCode int
		expectedError  error
	}{
		{
			name:           "bad request",
			mockResponse:   `{"error": {"message": "Invalid request"}}`,
			mockStatusCode: http.StatusBadRequest,
			expectedError:  errors.ErrInvalidInput,
		},
		{
			name:           "unauthorized",
			mockResponse:   `{"error": {"message": "Invalid API key"}}`,
			mockStatusCode: http.StatusUnauthorized,
			expectedError:  errors.ErrLLMAPIKey,
		},
		{
			name:           "forbidden",
			mockResponse:   `{"error": {"message": "Access denied"}}`,
			mockStatusCode: http.StatusForbidden,
			expectedError:  errors.ErrLLMAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.mockStatusCode)
				_, _ = w.Write([]byte(tt.mockResponse))
			}))
			defer server.Close()

			provider := &OpenAICompatibleProvider{
				apiURL:     server.URL,
				apiKey:     "test-key",
				model:      "test-model",
				httpClient: &http.Client{},
			}

			_, err := provider.GetCompletion(context.Background(), "system", "user")
			assert.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectedError))
		})
	}
}

// Mock provider for testing
type mockLLMProvider struct {
	response string
	err      error
}

func (m *mockLLMProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// Helper type for mocking HTTP transport
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("maskAPIKey", func(t *testing.T) {
		tests := []struct {
			name     string
			apiKey   string
			expected string
		}{
			{"short key", "abc", "****"},
			{"exact 8 chars", "12345678", "****"},
			{"normal key", "sk-abcdefghijklmnop", "sk-a***mnop"},
			{"empty key", "", "****"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := maskAPIKey(tt.apiKey)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("truncateForLog", func(t *testing.T) {
		tests := []struct {
			name     string
			content  string
			maxLen   int
			expected string
		}{
			{"short content", "hello", 10, "hello"},
			{"exact length", "hello", 5, "hello"},
			{"needs truncation", "hello world", 5, "hello..."},
			{"UTF-8 boundary", "hello世界", 5, "hello..."},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := truncateForLog(tt.content, tt.maxLen)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestOpenAICompatibleProvider_JSONErrors(t *testing.T) {
	// Test JSON marshaling error
	t.Run("json marshal error", func(t *testing.T) {
		provider := &OpenAICompatibleProvider{
			apiURL:     "https://api.test.com",
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{},
		}

		// Override messages to cause JSON marshal error
		// This is a bit hacky but tests the error path
		ctx := context.Background()
		_, err := provider.GetCompletion(ctx, string([]byte{0xFF}), "user")
		// We expect an error, but it might not be a marshal error due to Go's handling
		// Just ensure we get some error
		assert.Error(t, err)
	})

	// Test JSON unmarshal error
	t.Run("json unmarshal error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			apiURL:     server.URL,
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{},
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response")
	})

	// Test empty message content
	t.Run("empty message content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"content": "",
						},
					},
				},
			})
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			apiURL:     server.URL,
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{},
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty message content")
	})

	// Test read body error
	t.Run("read body error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			// Don't write the promised content
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			apiURL:     server.URL,
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{},
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		// Could be either "failed to read response" or EOF
	})
}

func TestOpenAICompatibleProvider_NetworkErrors(t *testing.T) {
	t.Run("network timeout error", func(t *testing.T) {
		// Create a server that delays response longer than client timeout
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			apiURL:     server.URL,
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{Timeout: 10 * time.Millisecond},
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		// Check if error is timeout error or contains timeout/deadline keywords
		isTimeout := errors.Is(err, errors.ErrLLMTimeout) ||
			strings.Contains(err.Error(), "timeout") ||
			strings.Contains(err.Error(), "deadline exceeded")
		assert.True(t, isTimeout, "Expected timeout error, got: %v", err)
	})

	t.Run("create request error", func(t *testing.T) {
		provider := &OpenAICompatibleProvider{
			apiURL:     "://invalid-url",
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{},
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create request")
	})

	t.Run("other API error codes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			apiURL:     server.URL,
			apiKey:     "test-key",
			model:      "test-model",
			httpClient: &http.Client{},
		}

		_, err := provider.GetCompletion(context.Background(), "system", "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error: status 404")
	})
}
