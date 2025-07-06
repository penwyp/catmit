package client

import (
	"context"
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
			timeout: 10 * time.Millisecond,
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

			c := NewClient(server.URL, tc.apiKey, tc.timeout)

			msg, err := c.GetCommitMessage(context.Background(), "test prompt")

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
