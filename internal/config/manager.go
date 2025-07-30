package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/penwyp/catmit/internal/errors"
)

// configManager implements the configuration file manager
type configManager struct {
	configPath string
	mu         sync.Mutex // protects concurrent writes
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) (Manager, error) {
	if configPath == "" {
		return nil, errors.New(errors.ErrTypeConfig, "config path cannot be empty")
	}

	return &configManager{
		configPath: configPath,
	}, nil
}

// Load loads the configuration file
func (m *configManager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to read config file", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to parse config", err)
	}

	return &config, nil
}

// Save saves the configuration file (atomic operation)
func (m *configManager) Save(config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize the configuration
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to marshal config", err)
	}

	// Ensure the directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// Atomic write: write to a temp file first, then rename
	tmpFile := m.configPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to write temp config file", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, m.configPath); err != nil {
		// Clean up temp file
		os.Remove(tmpFile)
		return errors.Wrap(errors.ErrTypeConfig, "failed to save config file", err)
	}

	return nil
}

// CreateDefaultConfig creates the default configuration
func (m *configManager) CreateDefaultConfig() error {
	defaultConfig := &Config{
		Version: "1.0.0",
		Remotes: map[string]RemoteConfig{
			"github.com": {
				Provider:     "github",
				CLITool:      "gh",
				MinVersion:   "2.0.0",
				AuthCommand:  "gh auth login",
				CreatePRArgs: []string{"pr", "create", "--fill"},
			},
			"gitlab.com": {
				Provider:     "gitlab",
				CLITool:      "glab",
				MinVersion:   "1.20.0",
				AuthCommand:  "glab auth login",
				CreatePRArgs: []string{"mr", "create", "--fill"},
			},
		},
	}

	return m.Save(defaultConfig)
}

// UpdateRemote updates the configuration for the specified remote
func (m *configManager) UpdateRemote(host string, remoteConfig RemoteConfig) error {
	// Load the existing configuration
	config, err := m.Load()
	if err != nil {
		// If the config file does not exist, create a new config
		if os.IsNotExist(err) {
			config = &Config{
				Version: "1.0.0",
				Remotes: make(map[string]RemoteConfig),
			}
		} else {
			return err
		}
	}

	// Update the remote configuration
	config.Remotes[host] = remoteConfig

	// Save the configuration
	return m.Save(config)
}
