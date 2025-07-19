package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/penwyp/catmit/internal/errors"
	"gopkg.in/yaml.v3"
)

// Format represents the configuration file format
type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// yamlConfigManager supports both JSON and YAML configuration files
type yamlConfigManager struct {
	configPath string
	format     Format
	mu         sync.Mutex
}

// NewYAMLConfigManager creates a config manager that supports both JSON and YAML
func NewYAMLConfigManager(configPath string) (Manager, error) {
	if configPath == "" {
		return nil, errors.New(errors.ErrTypeConfig, "config path cannot be empty")
	}

	// Determine format based on extension
	ext := strings.ToLower(filepath.Ext(configPath))
	var format Format
	switch ext {
	case ".json":
		format = FormatJSON
	case ".yaml", ".yml":
		format = FormatYAML
	default:
		// Default to YAML for new files
		format = FormatYAML
	}

	return &yamlConfigManager{
		configPath: configPath,
		format:     format,
	}, nil
}

// Load loads the configuration file in either JSON or YAML format
func (m *yamlConfigManager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err // Return the raw error for IsNotExist checks
		}
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to read config file", err)
	}

	var config Config
	
	// Try to unmarshal based on format
	switch m.format {
	case FormatJSON:
		if err := json.Unmarshal(data, &config); err != nil {
			// Try YAML as fallback
			if yamlErr := yaml.Unmarshal(data, &config); yamlErr == nil {
				// Note: We don't update format here because we're still inside the Lock
				// and the format is not exposed outside the struct
				return &config, nil
			}
			return nil, errors.Wrap(errors.ErrTypeConfig, "failed to parse config as JSON", err)
		}
	case FormatYAML:
		if err := yaml.Unmarshal(data, &config); err != nil {
			// Try JSON as fallback
			if jsonErr := json.Unmarshal(data, &config); jsonErr == nil {
				// Note: We don't update format here because we're still inside the Lock
				// and the format is not exposed outside the struct
				return &config, nil
			}
			return nil, errors.Wrap(errors.ErrTypeConfig, "failed to parse config as YAML", err)
		}
	}

	return &config, nil
}

// Save saves the configuration file in the appropriate format
func (m *yamlConfigManager) Save(config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var data []byte
	var err error

	// Marshal based on format
	switch m.format {
	case FormatJSON:
		data, err = json.MarshalIndent(config, "", "  ")
	case FormatYAML:
		data, err = yaml.Marshal(config)
	default:
		return errors.Newf(errors.ErrTypeConfig, "unknown format: %s", m.format)
	}

	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to marshal config", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// Atomic write: write to temp file then rename
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

// CreateDefaultConfig creates a default configuration file
func (m *yamlConfigManager) CreateDefaultConfig() error {
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
			"bitbucket.org": {
				Provider:     "bitbucket",
				CLITool:      "bb",
				MinVersion:   "1.0.0",
				AuthCommand:  "",
				CreatePRArgs: []string{},
			},
			"gitea.com": {
				Provider:     "gitea",
				CLITool:      "tea",
				MinVersion:   "0.9.0",
				AuthCommand:  "tea login add",
				CreatePRArgs: []string{"pr", "create"},
			},
		},
	}

	// Add comment header for YAML files
	if m.format == FormatYAML {
		if err := m.saveWithHeader(defaultConfig); err != nil {
			return err
		}
		return nil
	}

	return m.Save(defaultConfig)
}

// saveWithHeader saves YAML with a comment header
func (m *yamlConfigManager) saveWithHeader(config *Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to marshal config", err)
	}

	// Add header comment
	header := `# catmit provider configuration
# This file maps Git remote hosts to their provider types and CLI tools
# You can add custom mappings for your self-hosted Git services

`
	fullData := []byte(header + string(data))

	// Ensure directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// Atomic write
	tmpFile := m.configPath + ".tmp"
	if err := os.WriteFile(tmpFile, fullData, 0644); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to write temp config file", err)
	}

	if err := os.Rename(tmpFile, m.configPath); err != nil {
		os.Remove(tmpFile)
		return errors.Wrap(errors.ErrTypeConfig, "failed to save config file", err)
	}

	return nil
}

// UpdateRemote updates a specific remote configuration
func (m *yamlConfigManager) UpdateRemote(host string, remoteConfig RemoteConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var config *Config
	
	// Try to load existing config (without lock since we already have it)
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		// If config file doesn't exist, create new config
		if os.IsNotExist(err) {
			config = &Config{
				Version: "1.0.0",
				Remotes: make(map[string]RemoteConfig),
			}
		} else {
			return errors.Wrap(errors.ErrTypeConfig, "failed to read config file", err)
		}
	} else {
		// Parse the existing config
		config = &Config{}
		switch m.format {
		case FormatJSON:
			if err := json.Unmarshal(data, config); err != nil {
				// Try YAML as fallback
				if yamlErr := yaml.Unmarshal(data, config); yamlErr == nil {
					// Successfully parsed as YAML
				} else {
					return errors.Wrap(errors.ErrTypeConfig, "failed to parse config", err)
				}
			}
		case FormatYAML:
			if err := yaml.Unmarshal(data, config); err != nil {
				// Try JSON as fallback
				if jsonErr := json.Unmarshal(data, config); jsonErr == nil {
					// Successfully parsed as JSON
				} else {
					return errors.Wrap(errors.ErrTypeConfig, "failed to parse config", err)
				}
			}
		}
	}

	// Update remote config
	if config.Remotes == nil {
		config.Remotes = make(map[string]RemoteConfig)
	}
	config.Remotes[host] = remoteConfig

	// Save config (without calling Save which would lock again)
	var saveData []byte
	var saveErr error

	// Marshal based on format
	switch m.format {
	case FormatJSON:
		saveData, saveErr = json.MarshalIndent(config, "", "  ")
	case FormatYAML:
		saveData, saveErr = yaml.Marshal(config)
	default:
		return errors.Newf(errors.ErrTypeConfig, "unknown format: %s", m.format)
	}

	if saveErr != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to marshal config", saveErr)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// Atomic write: write to temp file then rename
	tmpFile := m.configPath + ".tmp"
	if err := os.WriteFile(tmpFile, saveData, 0644); err != nil {
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

// ConvertFormat converts the configuration file to a different format
func (m *yamlConfigManager) ConvertFormat(newFormat Format) error {
	// Load current config
	config, err := m.Load()
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to load config", err)
	}

	// Update format
	oldFormat := m.format
	m.format = newFormat

	// Update file path extension
	dir := filepath.Dir(m.configPath)
	base := strings.TrimSuffix(filepath.Base(m.configPath), filepath.Ext(m.configPath))
	
	switch newFormat {
	case FormatJSON:
		m.configPath = filepath.Join(dir, base+".json")
	case FormatYAML:
		m.configPath = filepath.Join(dir, base+".yaml")
	}

	// Save in new format
	if err := m.Save(config); err != nil {
		// Restore old format on error
		m.format = oldFormat
		return errors.Wrap(errors.ErrTypeConfig, "failed to save in new format", err)
	}

	return nil
}