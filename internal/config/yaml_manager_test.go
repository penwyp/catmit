package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewYAMLConfigManager(t *testing.T) {
	tests := []struct {
		name           string
		configPath     string
		expectedFormat Format
		expectError    bool
	}{
		{
			name:           "JSON file extension",
			configPath:     "/path/to/config.json",
			expectedFormat: FormatJSON,
		},
		{
			name:           "YAML file extension",
			configPath:     "/path/to/config.yaml",
			expectedFormat: FormatYAML,
		},
		{
			name:           "YML file extension",
			configPath:     "/path/to/config.yml",
			expectedFormat: FormatYAML,
		},
		{
			name:           "No extension defaults to YAML",
			configPath:     "/path/to/config",
			expectedFormat: FormatYAML,
		},
		{
			name:           "Unknown extension defaults to YAML",
			configPath:     "/path/to/config.txt",
			expectedFormat: FormatYAML,
		},
		{
			name:        "Empty path returns error",
			configPath:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewYAMLConfigManager(tt.configPath)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, manager)
				
				// Check format
				yamlMgr := manager.(*yamlConfigManager)
				assert.Equal(t, tt.expectedFormat, yamlMgr.format)
			}
		})
	}
}

func TestYAMLConfigManager_SaveAndLoad(t *testing.T) {
	tests := []struct {
		name       string
		format     Format
		extension  string
	}{
		{
			name:      "JSON format",
			format:    FormatJSON,
			extension: ".json",
		},
		{
			name:      "YAML format",
			format:    FormatYAML,
			extension: ".yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir, err := os.MkdirTemp("", "catmit-config-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			configPath := filepath.Join(tempDir, "config"+tt.extension)
			manager, err := NewYAMLConfigManager(configPath)
			require.NoError(t, err)

			// Test config
			testConfig := &Config{
				Version: "1.0.0",
				Remotes: map[string]RemoteConfig{
					"git.example.com": {
						Provider:     "gitlab",
						CLITool:      "glab",
						MinVersion:   "1.20.0",
						AuthCommand:  "glab auth login",
						CreatePRArgs: []string{"mr", "create", "--fill"},
					},
					"github.enterprise.com": {
						Provider:     "github",
						CLITool:      "gh",
						MinVersion:   "2.0.0",
						AuthCommand:  "gh auth login",
						CreatePRArgs: []string{"pr", "create", "--fill", "--draft"},
					},
				},
			}

			// Save
			err = manager.Save(testConfig)
			assert.NoError(t, err)

			// Verify file exists
			_, err = os.Stat(configPath)
			assert.NoError(t, err)

			// Load
			loadedConfig, err := manager.Load()
			assert.NoError(t, err)
			assert.Equal(t, testConfig, loadedConfig)
		})
	}
}

func TestYAMLConfigManager_FormatFallback(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Write JSON content to a .yaml file
	configPath := filepath.Join(tempDir, "config.yaml")
	jsonContent := `{
		"version": "1.0.0",
		"remotes": {
			"github.com": {
				"provider": "github",
				"cli_tool": "gh",
				"min_version": "2.0.0",
				"auth_command": "gh auth login",
				"create_pr_args": ["pr", "create", "--fill"]
			}
		}
	}`
	err = os.WriteFile(configPath, []byte(jsonContent), 0644)
	require.NoError(t, err)

	// Create manager expecting YAML
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Load should succeed with format fallback
	config, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "github", config.Remotes["github.com"].Provider)
}

func TestYAMLConfigManager_CreateDefaultConfig(t *testing.T) {
	tests := []struct {
		name      string
		extension string
		format    Format
	}{
		{
			name:      "JSON format",
			extension: ".json",
			format:    FormatJSON,
		},
		{
			name:      "YAML format",
			extension: ".yaml",
			format:    FormatYAML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir, err := os.MkdirTemp("", "catmit-config-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			configPath := filepath.Join(tempDir, "config"+tt.extension)
			manager, err := NewYAMLConfigManager(configPath)
			require.NoError(t, err)

			// Create default config
			err = manager.CreateDefaultConfig()
			assert.NoError(t, err)

			// Load and verify
			config, err := manager.Load()
			assert.NoError(t, err)
			assert.Equal(t, "1.0.0", config.Version)
			assert.Contains(t, config.Remotes, "github.com")
			assert.Contains(t, config.Remotes, "gitlab.com")
			assert.Contains(t, config.Remotes, "bitbucket.org")
			assert.Contains(t, config.Remotes, "gitea.com")

			// For YAML, check that file contains header
			if tt.format == FormatYAML {
				content, err := os.ReadFile(configPath)
				assert.NoError(t, err)
				assert.Contains(t, string(content), "# catmit provider configuration")
			}
		})
	}
}

func TestYAMLConfigManager_UpdateRemote(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Test updating when config doesn't exist
	err = manager.UpdateRemote("git.custom.com", RemoteConfig{
		Provider: "gitea",
		CLITool:  "tea",
	})
	assert.NoError(t, err)

	// Verify
	config, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "gitea", config.Remotes["git.custom.com"].Provider)
	assert.Equal(t, "tea", config.Remotes["git.custom.com"].CLITool)

	// Update existing remote
	err = manager.UpdateRemote("git.custom.com", RemoteConfig{
		Provider:    "gitlab",
		CLITool:     "glab",
		MinVersion:  "1.25.0",
		AuthCommand: "glab auth login",
	})
	assert.NoError(t, err)

	// Verify update
	config, err = manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "gitlab", config.Remotes["git.custom.com"].Provider)
	assert.Equal(t, "glab", config.Remotes["git.custom.com"].CLITool)
	assert.Equal(t, "1.25.0", config.Remotes["git.custom.com"].MinVersion)
}

func TestYAMLConfigManager_ConvertFormat(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Start with JSON
	configPath := filepath.Join(tempDir, "config.json")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Create test config
	testConfig := &Config{
		Version: "1.0.0",
		Remotes: map[string]RemoteConfig{
			"test.com": {
				Provider: "github",
				CLITool:  "gh",
			},
		},
	}
	err = manager.Save(testConfig)
	require.NoError(t, err)

	// Convert to YAML
	yamlMgr := manager.(*yamlConfigManager)
	err = yamlMgr.ConvertFormat(FormatYAML)
	assert.NoError(t, err)

	// Check new file exists
	yamlPath := filepath.Join(tempDir, "config.yaml")
	_, err = os.Stat(yamlPath)
	assert.NoError(t, err)

	// Load and verify content
	loadedConfig, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, testConfig.Version, loadedConfig.Version)
	assert.Equal(t, testConfig.Remotes["test.com"].Provider, loadedConfig.Remotes["test.com"].Provider)
	assert.Equal(t, testConfig.Remotes["test.com"].CLITool, loadedConfig.Remotes["test.com"].CLITool)
	
	// Verify format was updated
	assert.Equal(t, FormatYAML, yamlMgr.format)
	assert.Equal(t, yamlPath, yamlMgr.configPath)
}

func TestYAMLConfigManager_ConcurrentAccess(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Create initial config
	err = manager.CreateDefaultConfig()
	require.NoError(t, err)

	// Concurrent updates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			host := fmt.Sprintf("git%d.example.com", id)
			err := manager.UpdateRemote(host, RemoteConfig{
				Provider: "gitlab",
				CLITool:  "glab",
			})
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all updates were applied
	config, err := manager.Load()
	assert.NoError(t, err)
	for i := 0; i < 10; i++ {
		host := fmt.Sprintf("git%d.example.com", i)
		assert.Contains(t, config.Remotes, host)
	}
}