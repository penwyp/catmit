package provider

import (
	"context"
	"strings"

	"github.com/penwyp/catmit/internal/config"
)

// ConfigDetector is a provider detector that checks config mappings before falling back to other detection methods
type ConfigDetector struct {
	configManager config.Manager
	httpProber    HTTPProber
}

// NewConfigDetector creates a new config-aware provider detector
func NewConfigDetector(configManager config.Manager) *ConfigDetector {
	return &ConfigDetector{
		configManager: configManager,
		httpProber:    NewHTTPProber(),
	}
}

// DetectFromRemote detects provider from remote URL with config priority
func (d *ConfigDetector) DetectFromRemote(ctx context.Context, remoteURL string) (RemoteInfo, error) {
	// Parse the URL first
	info, err := ParseGitRemoteURL(remoteURL)
	if err != nil {
		return RemoteInfo{}, err
	}

	// 1. Check config mappings first (highest priority)
	if d.configManager != nil {
		cfg, err := d.configManager.Load()
		if err == nil && cfg != nil && cfg.Remotes != nil {
			// Check if we have a config for this host
			if remoteCfg, exists := cfg.Remotes[info.Host]; exists && remoteCfg.Provider != "" {
				info.Provider = remoteCfg.Provider
				return info, nil
			}
		}
	}

	// 2. Pattern matching for well-known hosts
	d.detectProviderFromHost(&info)
	if info.Provider != "unknown" && info.Provider != "" {
		return info, nil
	}

	// 3. HTTP probing for unknown hosts
	if d.httpProber != nil {
		probeResult := d.httpProber.ProbeGitea(ctx, info.GetHTTPURL())
		if probeResult.IsGitea {
			info.Provider = "gitea"
			
			// Optionally save this discovery to config for future use
			if d.configManager != nil {
				d.saveDiscoveredProvider(info.Host, "gitea")
			}
			
			return info, nil
		}
	}

	// 4. Return with "unknown" provider
	return info, nil
}

// detectProviderFromHost detects provider based on hostname patterns
func (d *ConfigDetector) detectProviderFromHost(info *RemoteInfo) {
	host := strings.ToLower(info.Host)
	
	// Check for exact domain matches first
	switch host {
	case "github.com", "www.github.com":
		info.Provider = "github"
		return
	case "gitlab.com", "www.gitlab.com":
		info.Provider = "gitlab"
		return
	case "bitbucket.org", "www.bitbucket.org":
		info.Provider = "bitbucket"
		return
	}
	
	// Then check for common patterns
	switch {
	case strings.Contains(host, "github"):
		// GitHub Enterprise often uses patterns like github.company.com
		info.Provider = "github"
	case strings.Contains(host, "gitlab"):
		// Self-hosted GitLab often contains "gitlab" in the hostname
		info.Provider = "gitlab"
	case strings.Contains(host, "bitbucket"):
		// Bitbucket Server/Data Center
		info.Provider = "bitbucket"
	case strings.Contains(host, "gitea"):
		// Common Gitea subdomain patterns
		info.Provider = "gitea"
	case strings.Contains(host, "gogs"):
		// Gogs (similar to Gitea)
		info.Provider = "gogs"
	default:
		info.Provider = "unknown"
	}
}

// saveDiscoveredProvider saves a newly discovered provider to config
func (d *ConfigDetector) saveDiscoveredProvider(host, provider string) {
	// Update the remote config directly
	// UpdateRemote will handle creating the config if it doesn't exist
	err := d.configManager.UpdateRemote(host, config.RemoteConfig{
		Provider: provider,
		CLITool:  getDefaultCLITool(provider),
	})
	
	// Ignore save errors - this is just an optimization
	_ = err
}

// getDefaultCLITool returns the default CLI tool for a provider
func getDefaultCLITool(provider string) string {
	switch provider {
	case "github":
		return "gh"
	case "gitlab":
		return "glab"
	case "gitea":
		return "tea"
	case "bitbucket":
		return "bb" // Bitbucket CLI tool (if available)
	case "gogs":
		return "" // No official CLI tool for Gogs
	default:
		return ""
	}
}