# Provider Detection Flow Analysis

## Current Implementation Overview

The PR creation flow in catmit currently uses a provider detection mechanism to identify which Git hosting service (GitHub, GitLab, Gitea, etc.) is being used. This analysis documents the current flow and identifies integration points for config-based provider mapping.

## Current Provider Detection Flow

### 1. Entry Point: PR Creation (`internal/pr/creator.go`)

```go
// Create method in Creator struct (line 76-191)
func (c *Creator) Create(ctx context.Context, options CreateOptions) (string, error) {
    // Step 1: Get remote URL (line 83-86)
    remoteURL, err := c.git.GetRemoteURL(ctx, options.Remote)
    
    // Step 2: Detect provider (line 88-92)
    remoteInfo, err := c.providerDetector.DetectFromRemote(ctx, remoteURL)
    
    // Step 3: Check if provider is supported (line 94-97)
    if remoteInfo.Provider == "unknown" {
        return "", fmt.Errorf("unsupported provider: %s", remoteInfo.Provider)
    }
    
    // Step 4: Detect CLI status (line 99-103)
    cliStatus, err := c.cliDetector.DetectCLI(ctx, remoteInfo.Provider)
    
    // Step 5-8: Validate CLI installation, authentication, version...
    // Step 9: Build and execute PR command
}
```

### 2. Provider Detection Implementation (`cmd/root.go`)

The current implementation uses a `defaultProviderDetector` struct:

```go
type defaultProviderDetector struct{}

func (d *defaultProviderDetector) DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error) {
    // Step 1: Parse the Git remote URL
    info, err := provider.ParseGitRemoteURL(remoteURL)
    
    // Step 2: Detect provider from hostname
    detectProviderFromHost(&info)
    
    // Step 3: If unknown, try HTTP probe for Gitea
    if info.Provider == "unknown" || info.Provider == "" {
        prober := provider.NewHTTPProber()
        probeResult := prober.ProbeGitea(ctx, info.GetHTTPURL())
        if probeResult.IsGitea {
            info.Provider = "gitea"
        }
    }
    
    return info, nil
}
```

### 3. Host-based Detection (`cmd/root.go`)

```go
func detectProviderFromHost(info *provider.RemoteInfo) {
    host := strings.ToLower(info.Host)
    
    switch {
    case strings.Contains(host, "github.com"):
        info.Provider = "github"
    case strings.Contains(host, "gitlab.com"):
        info.Provider = "gitlab"
    case strings.Contains(host, "gitea"):
        info.Provider = "gitea"
    default:
        info.Provider = "unknown"
    }
}
```

### 4. Data Structures

#### RemoteInfo (`internal/provider/types.go`)
```go
type RemoteInfo struct {
    Provider string // github, gitlab, gitea, bitbucket, unknown
    Host     string // hostname, e.g., github.com
    Port     int    // port number, 0 for default
    Owner    string // repository owner or organization
    Repo     string // repository name
    Protocol string // https, ssh
}
```

## Config Integration Points

### 1. Existing Config Structure (`internal/config/types.go`)

The config package already defines structures for provider mapping:

```go
type Config struct {
    Version string                   `json:"version"`
    Remotes map[string]RemoteConfig `json:"remotes"`  // key is hostname
}

type RemoteConfig struct {
    Provider     string   `json:"provider"`       // github, gitlab, gitea, etc.
    CLITool      string   `json:"cli_tool"`       // gh, glab, tea, etc.
    MinVersion   string   `json:"min_version"`    // CLI tool minimum version
    AuthCommand  string   `json:"auth_command"`   // authentication command
    CreatePRArgs []string `json:"create_pr_args"` // PR creation arguments
}
```

### 2. Config Manager (`internal/config/manager.go`)

The config manager provides methods to:
- Load configuration from file
- Save configuration atomically
- Create default configuration
- Update specific remote configurations

## Integration Requirements

### 1. Enhanced Provider Detector

The provider detector needs to be enhanced to:

1. **Check config first**: Before hostname pattern matching, check if there's a configured mapping for the host
2. **Fall back to current detection**: If no config mapping exists, use the current detection logic
3. **Save detected mappings**: Optionally save newly detected providers to config for future use

### 2. Integration Flow

```
PR Creation Request
    ↓
Get Remote URL
    ↓
Parse URL → Extract Host
    ↓
Check Config Mapping ←─── Load Config File
    ↓ (if found)         (from ~/.config/catmit/providers.yaml)
    ↓
[Config Found?]
    Yes → Use configured provider
    No  → Current Detection Logic
           ├─ Pattern matching
           ├─ HTTP probe
           └─ Return result
    ↓
Use provider info for CLI detection
    ↓
Create PR
```

### 3. Code Changes Needed

#### A. Update defaultProviderDetector

```go
type defaultProviderDetector struct{
    configManager config.Manager  // Add config manager
}

func (d *defaultProviderDetector) DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error) {
    // Parse URL first
    info, err := provider.ParseGitRemoteURL(remoteURL)
    if err != nil {
        return provider.RemoteInfo{}, err
    }
    
    // NEW: Check config mapping first
    if d.configManager != nil {
        cfg, err := d.configManager.Load()
        if err == nil && cfg.Remotes != nil {
            if remoteConfig, exists := cfg.Remotes[info.Host]; exists {
                info.Provider = remoteConfig.Provider
                return info, nil
            }
        }
    }
    
    // Continue with existing detection logic...
    detectProviderFromHost(&info)
    
    // HTTP probe for unknown providers...
    // ...existing code...
}
```

#### B. Update Creator initialization

The `cmd/root.go` needs to initialize the config manager and pass it to the provider detector:

```go
// In cmd/root.go or where Creator is initialized
configPath := filepath.Join(os.UserConfigDir(), "catmit", "providers.yaml")
configMgr, _ := config.NewConfigManager(configPath)

providerDetector := &defaultProviderDetector{
    configManager: configMgr,
}
```

### 4. Benefits of Config-based Mapping

1. **Support for self-hosted instances**: Users can map their custom GitLab/Gitea instances
2. **Override detection**: Users can force a specific provider for edge cases
3. **Faster detection**: Skip HTTP probing for known hosts
4. **Custom CLI tools**: Support alternative CLI tools for the same provider

### 5. Example Config Usage

```yaml
# ~/.config/catmit/providers.yaml
version: "1.0.0"
remotes:
  github.company.com:
    provider: github
    cli_tool: gh
    min_version: "2.0.0"
  git.internal.net:
    provider: gitea
    cli_tool: tea
    min_version: "0.8.0"
  gitlab.myteam.io:
    provider: gitlab
    cli_tool: glab
    min_version: "1.20.0"
```

## Summary

The current provider detection flow is functional but limited to hardcoded patterns. The config package is already implemented and ready to use. The main integration work involves:

1. Adding config manager to the provider detector
2. Checking config mappings before falling back to pattern matching
3. Ensuring proper initialization of config manager in the command setup
4. Adding appropriate error handling and logging

This enhancement will make the PR feature more flexible and support a wider range of Git hosting scenarios, especially for enterprise users with self-hosted instances.