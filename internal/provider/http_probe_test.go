package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProbeHTTP(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		maxRetries      int
		expectedResult  ProbeResult
		expectedRetries int
		timeout         time.Duration
	}{
		{
			name: "Gitea API detected on first try",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/v1/version" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"version":"1.21.0"}`))
					}
				}))
			},
			maxRetries: 3,
			expectedResult: ProbeResult{
				IsGitea: true,
				Version: "1.21.0",
			},
			expectedRetries: 0,
			timeout:         5 * time.Second,
		},
		{
			name: "Non-Gitea server",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			maxRetries: 3,
			expectedResult: ProbeResult{
				IsGitea: false,
			},
			expectedRetries: 0,
			timeout:         5 * time.Second,
		},
		{
			name: "Gitea API with invalid JSON",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/v1/version" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`invalid json`))
					}
				}))
			},
			maxRetries: 3,
			expectedResult: ProbeResult{
				IsGitea: false,
			},
			expectedRetries: 0,
			timeout:         5 * time.Second,
		},
		{
			name: "Retry on server error",
			setupServer: func() *httptest.Server {
				attempts := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts++
					if attempts < 3 {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"version":"1.21.0"}`))
					}
				}))
			},
			maxRetries: 3,
			expectedResult: ProbeResult{
				IsGitea: true,
				Version: "1.21.0",
			},
			expectedRetries: 2,
			timeout:         10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			prober := NewHTTPProber()
			result := prober.ProbeGitea(ctx, server.URL)

			assert.Equal(t, tt.expectedResult.IsGitea, result.IsGitea)
			assert.Equal(t, tt.expectedResult.Version, result.Version)
		})
	}
}

func TestProbeHTTP_Timeout(t *testing.T) {
	// Create a server that never responds (simulates a hanging server)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block forever
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	prober := NewHTTPProber()
	result := prober.ProbeGitea(ctx, server.URL)

	assert.False(t, result.IsGitea)
	assert.NotNil(t, result.Error)
	assert.True(t, errors.Is(result.Error, context.DeadlineExceeded) ||
		errors.Is(result.Error, ErrProbeTimeout))
}

func TestProbeHTTP_NetworkError(t *testing.T) {
	// Use an invalid address to simulate a network error
	prober := NewHTTPProber()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := prober.ProbeGitea(ctx, "http://localhost:99999")

	assert.False(t, result.IsGitea)
	assert.NotNil(t, result.Error)

	// Check if the error is a network error or timeout error
	// The error might be wrapped, and might be either connection refused or deadline exceeded
	errStr := result.Error.Error()
	isNetworkError := strings.Contains(errStr, "connection refused") || 
	                  strings.Contains(errStr, "context deadline exceeded") ||
	                  strings.Contains(errStr, "no such host")
	assert.True(t, isNetworkError, "Expected network error (connection refused or timeout), got: %v", result.Error)
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 4 * time.Second}, // max 4 seconds
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			backoff := calculateBackoff(tt.attempt)
			assert.Equal(t, tt.expected, backoff)
		})
	}
}
