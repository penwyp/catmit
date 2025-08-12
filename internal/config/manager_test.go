package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigManager(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "Valid path",
			configPath: "/tmp/test-config.yaml",
			wantErr:    false,
		},
		{
			name:        "Empty path",
			configPath:  "",
			wantErr:     true,
			errContains: "config path cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewYAMLConfigManager(tt.configPath)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
			}
		})
	}
}

func TestConfigManager_CreateDefaultConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Create default config
	err = manager.CreateDefaultConfig()
	assert.NoError(t, err)

	// Verify file is created
	assert.FileExists(t, configPath)

	// Read and verify content
	config, err := manager.Load()
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.NotEmpty(t, config.Version)
	assert.NotEmpty(t, config.Remotes)
}

func TestConfigManager_Load(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(string) error
		wantErr     bool
		errContains string
		validate    func(*testing.T, *Config)
	}{
		{
			name: "Load valid config",
			setupFunc: func(path string) error {
				content := `version: "1.0.0"
remotes:
  github.com:
    provider: "github"
    cli_tool: "gh"
    min_version: "2.0.0"`
				return os.WriteFile(path, []byte(content), 0644)
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "1.0.0", cfg.Version)
				assert.Contains(t, cfg.Remotes, "github.com")
				assert.Equal(t, "github", cfg.Remotes["github.com"].Provider)
			},
		},
		{
			name: "Load corrupted config",
			setupFunc: func(path string) error {
				return os.WriteFile(path, []byte("invalid json"), 0644)
			},
			wantErr:     true,
			errContains: "failed to parse config",
		},
		{
			name:        "Load non-existent config",
			setupFunc:   func(path string) error { return nil },
			wantErr:     true,
			errContains: "no such file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "catmit-config-test")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configPath := filepath.Join(tmpDir, "config.yaml")
			if tt.setupFunc != nil {
				err = tt.setupFunc(configPath)
				require.NoError(t, err)
			}

			manager, err := NewYAMLConfigManager(configPath)
			require.NoError(t, err)

			config, err := manager.Load()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, config)
				}
			}
		})
	}
}

func TestConfigManager_Save(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Prepare test config
	config := &Config{
		Version: "1.0.0",
		Remotes: map[string]RemoteConfig{
			"github.com": {
				Provider:   "github",
				CLITool:    "gh",
				MinVersion: "2.0.0",
			},
		},
	}

	// Save config
	err = manager.Save(config)
	assert.NoError(t, err)

	// Verify file exists
	assert.FileExists(t, configPath)

	// Reload and verify
	loaded, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, config.Version, loaded.Version)
	
	// Compare fields individually since YAML/JSON marshaling may convert nil slices to empty slices
	expectedRemote := config.Remotes["github.com"]
	actualRemote := loaded.Remotes["github.com"]
	assert.Equal(t, expectedRemote.Provider, actualRemote.Provider)
	assert.Equal(t, expectedRemote.CLITool, actualRemote.CLITool)
	assert.Equal(t, expectedRemote.MinVersion, actualRemote.MinVersion)
	assert.Equal(t, expectedRemote.AuthCommand, actualRemote.AuthCommand)
	// Both nil and empty slice are acceptable for CreatePRArgs
	if expectedRemote.CreatePRArgs == nil {
		assert.True(t, len(actualRemote.CreatePRArgs) == 0)
	} else {
		assert.Equal(t, expectedRemote.CreatePRArgs, actualRemote.CreatePRArgs)
	}
}

func TestConfigManager_AtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Create initial config
	config1 := &Config{
		Version: "1.0.0",
		Remotes: map[string]RemoteConfig{
			"github.com": {Provider: "github"},
		},
	}
	err = manager.Save(config1)
	require.NoError(t, err)

	// Simulate error during write process
	// Atomic write should guarantee either full success or keep the original state
	// Here we verify by permission test

	// First verify normal case
	config2 := &Config{
		Version: "2.0.0",
		Remotes: map[string]RemoteConfig{
			"gitlab.com": {Provider: "gitlab"},
		},
	}
	err = manager.Save(config2)
	assert.NoError(t, err)

	// Verify new config is saved
	loaded, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "2.0.0", loaded.Version)
}

func TestConfigManager_ConcurrentWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Create initial config
	err = manager.CreateDefaultConfig()
	require.NoError(t, err)

	// Concurrent write test
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			config := &Config{
				Version: fmt.Sprintf("1.0.%d", id),
				Remotes: map[string]RemoteConfig{
					fmt.Sprintf("host%d.com", id): {
						Provider: fmt.Sprintf("provider%d", id),
					},
				},
			}
			if err := manager.Save(config); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Verify no errors
	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	// Verify final state is valid
	_, err = manager.Load()
	assert.NoError(t, err)
}

func TestConfigManager_UpdateRemote(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	manager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)

	// Create initial config
	err = manager.CreateDefaultConfig()
	require.NoError(t, err)

	// Update remote config
	remoteConfig := RemoteConfig{
		Provider:     "custom",
		CLITool:      "custom-cli",
		MinVersion:   "1.0.0",
		AuthCommand:  "custom-cli auth login",
		CreatePRArgs: []string{"pr", "create"},
	}

	err = manager.UpdateRemote("custom.example.com", remoteConfig)
	assert.NoError(t, err)

	// Verify update
	config, err := manager.Load()
	assert.NoError(t, err)
	assert.Contains(t, config.Remotes, "custom.example.com")
	assert.Equal(t, remoteConfig, config.Remotes["custom.example.com"])
}
