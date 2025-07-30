package config

// Config represents the configuration file structure
type Config struct {
	Version string                  `json:"version" yaml:"version"`
	Remotes map[string]RemoteConfig `json:"remotes" yaml:"remotes"`
}

// RemoteConfig represents the configuration for a remote repository
type RemoteConfig struct {
	Provider     string   `json:"provider" yaml:"provider"`             // github, gitlab, gitea, etc.
	CLITool      string   `json:"cli_tool" yaml:"cli_tool"`             // gh, glab, tea, etc.
	MinVersion   string   `json:"min_version" yaml:"min_version"`       // Minimum version required for the CLI tool
	AuthCommand  string   `json:"auth_command" yaml:"auth_command"`     // Authentication command
	CreatePRArgs []string `json:"create_pr_args" yaml:"create_pr_args"` // Arguments for creating a PR
}

// Manager is the interface for the configuration manager
type Manager interface {
	// Load loads the configuration file
	Load() (*Config, error)

	// Save saves the configuration file (atomic operation)
	Save(config *Config) error

	// CreateDefaultConfig creates the default configuration
	CreateDefaultConfig() error

	// UpdateRemote updates the configuration for the specified remote
	UpdateRemote(host string, config RemoteConfig) error
}
