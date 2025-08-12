package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/penwyp/catmit/internal/errors"
)

// httpProber implements HTTP probing functionality
type httpProber struct {
	client     *http.Client
	maxRetries int
	timeout    time.Duration
}

// ProberOption is a configuration option for httpProber
type ProberOption func(*httpProber)


// NewHTTPProber creates a new HTTP prober
func NewHTTPProber(opts ...ProberOption) HTTPProber {
	p := &httpProber{
		maxRetries: 3,
		timeout:    3 * time.Second,
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client = &http.Client{
		Timeout: p.timeout,
	}

	return p
}

// ProbeGitea probes the Gitea API
func (p *httpProber) ProbeGitea(ctx context.Context, baseURL string) ProbeResult {
	url := baseURL + "/api/v1/version"

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration
			backoff := calculateBackoff(attempt - 1)
			select {
			case <-ctx.Done():
				return ProbeResult{
					IsGitea: false,
					Error:   ctx.Err(),
				}
			case <-time.After(backoff):
				// Continue to retry
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return ProbeResult{
				IsGitea: false,
				Error:   err,
			}
		}

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			// Network error, continue to retry
			continue
		}
		defer resp.Body.Close()

		// Server error (5xx), continue to retry
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			lastErr = errors.Newf(errors.ErrTypeExternal, "server error: %d", resp.StatusCode)
			continue
		}

		// Other non-200 status codes, do not retry
		if resp.StatusCode != http.StatusOK {
			return ProbeResult{
				IsGitea: false,
			}
		}

		// Parse response
		var versionInfo struct {
			Version string `json:"version"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
			// JSON parsing error, not Gitea
			return ProbeResult{
				IsGitea: false,
			}
		}

		// Successfully detected Gitea
		return ProbeResult{
			IsGitea: true,
			Version: versionInfo.Version,
		}
	}

	// All retries failed
	if lastErr != nil {
		return ProbeResult{
			IsGitea: false,
			Error:   lastErr,
		}
	}

	return ProbeResult{
		IsGitea: false,
		Error:   ErrProbeTimeout,
	}
}

// calculateBackoff calculates exponential backoff duration
func calculateBackoff(attempt int) time.Duration {
	base := time.Second
	maxBackoff := 4 * time.Second

	backoff := base * (1 << attempt) // 2^attempt
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return backoff
}
