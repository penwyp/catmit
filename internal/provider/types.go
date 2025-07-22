package provider

import (
	"context"
	"github.com/penwyp/catmit/internal/errors"
)

// RemoteInfo contains parsed information from a Git remote URL
type RemoteInfo struct {
	Provider string // Provider type: github, gitlab, gitea, bitbucket, unknown
	Host     string // Hostname, e.g., github.com
	Port     int    // Port number, 0 means default port
	Owner    string // Repository owner or organization
	Repo     string // Repository name
	Protocol string // Protocol: https, ssh
}

// ProbeResult represents the result of an HTTP probe
type ProbeResult struct {
	IsGitea bool
	Version string
	Error   error
}

// Detector is the interface for provider detection
type Detector interface {
	// DetectFromRemoteURL detects the provider type from a Git remote URL
	DetectFromRemoteURL(url string) (*RemoteInfo, error)

	// ProbeHTTP confirms the provider type via HTTP probing
	ProbeHTTP(baseURL string) (*ProbeResult, error)
}

// HTTPProber is the interface for HTTP probing
type HTTPProber interface {
	ProbeGitea(ctx context.Context, baseURL string) ProbeResult
}

// Error definitions
var (
	ErrProbeTimeout = errors.NewRetryable(errors.ErrTypeTimeout, "probe timeout")
)
