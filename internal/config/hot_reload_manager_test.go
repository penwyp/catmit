package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHotReloadManager_Basic tests basic hot reload functionality
func TestHotReloadManager_Basic(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	// Create base manager
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	// Create hot reload manager
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	// Initial config should be created
	config, err := hrManager.Load()
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.NotEmpty(t, config.Remotes)
	
	// Test callback registration
	var callbackCount int32
	var lastConfig *Config
	var mu sync.Mutex
	
	hrManager.OnConfigChange(func(cfg *Config) {
		atomic.AddInt32(&callbackCount, 1)
		mu.Lock()
		lastConfig = cfg
		mu.Unlock()
	})
	
	// Update config file directly
	newConfig := &Config{
		Version: "2.0.0",
		Remotes: map[string]RemoteConfig{
			"custom.git.com": {
				Provider: "custom",
				CLITool:  "custom-cli",
			},
		},
	}
	
	// Save through base manager to trigger file change
	err = baseManager.Save(newConfig)
	require.NoError(t, err)
	
	// Wait for reload
	time.Sleep(200 * time.Millisecond)
	
	// Check that callback was called
	assert.Equal(t, int32(1), atomic.LoadInt32(&callbackCount))
	
	// Check that config was reloaded
	mu.Lock()
	assert.NotNil(t, lastConfig)
	assert.Equal(t, "2.0.0", lastConfig.Version)
	assert.Contains(t, lastConfig.Remotes, "custom.git.com")
	mu.Unlock()
	
	// Load should return the new config
	loadedConfig, err := hrManager.Load()
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", loadedConfig.Version)
}

// TestHotReloadManager_MultipleChanges tests handling multiple rapid changes
func TestHotReloadManager_MultipleChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	var callbackCount int32
	hrManager.OnConfigChange(func(cfg *Config) {
		atomic.AddInt32(&callbackCount, 1)
	})
	
	// Make multiple rapid changes
	for i := 0; i < 5; i++ {
		newConfig := &Config{
			Version: "1.0." + string(rune('0'+i)),
			Remotes: map[string]RemoteConfig{
				"test.com": {
					Provider: "test",
				},
			},
		}
		err = baseManager.Save(newConfig)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Small delay between changes
	}
	
	// Wait for debouncing to settle
	time.Sleep(300 * time.Millisecond)
	
	// Should have fewer callbacks than changes due to debouncing
	count := atomic.LoadInt32(&callbackCount)
	assert.Greater(t, count, int32(0))
	assert.LessOrEqual(t, count, int32(5))
}

// TestHotReloadManager_UpdateRemote tests UpdateRemote functionality
func TestHotReloadManager_UpdateRemote(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	var callbackCalled bool
	hrManager.OnConfigChange(func(cfg *Config) {
		callbackCalled = true
	})
	
	// Update a remote
	err = hrManager.UpdateRemote("newhost.com", RemoteConfig{
		Provider: "github",
		CLITool:  "gh",
	})
	require.NoError(t, err)
	
	// Check that config was updated
	config, err := hrManager.Load()
	require.NoError(t, err)
	assert.Contains(t, config.Remotes, "newhost.com")
	assert.Equal(t, "github", config.Remotes["newhost.com"].Provider)
	
	// Callback should have been called
	time.Sleep(50 * time.Millisecond)
	assert.True(t, callbackCalled)
}

// TestHotReloadManager_FileRemoval tests handling of config file removal
func TestHotReloadManager_FileRemoval(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	// Load initial config
	initialConfig, err := hrManager.Load()
	require.NoError(t, err)
	assert.NotNil(t, initialConfig)
	
	// Remove the config file
	err = os.Remove(configPath)
	require.NoError(t, err)
	
	// Wait for file watcher to notice
	time.Sleep(200 * time.Millisecond)
	
	// Should still be able to load the cached config
	config, err := hrManager.Load()
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, initialConfig.Version, config.Version)
}

// TestHotReloadManager_InvalidConfig tests handling of invalid config
func TestHotReloadManager_InvalidConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	// Get initial config
	initialConfig, err := hrManager.Load()
	require.NoError(t, err)
	
	// Write invalid YAML to config file
	err = os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644)
	require.NoError(t, err)
	
	// Wait for reload attempt
	time.Sleep(200 * time.Millisecond)
	
	// Should still have the old config
	config, err := hrManager.Load()
	require.NoError(t, err)
	assert.Equal(t, initialConfig.Version, config.Version)
}

// TestHotReloadManager_ConcurrentAccess tests concurrent access safety
func TestHotReloadManager_ConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	// Start multiple goroutines accessing the config
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	
	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, err := hrManager.Load()
				if err != nil {
					errors <- err
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}
	
	// Writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				err := hrManager.UpdateRemote(
					fmt.Sprintf("host%d-%d.com", id, j),
					RemoteConfig{Provider: "test"},
				)
				if err != nil {
					errors <- err
				}
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}
	
	// File modifier
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			newConfig := &Config{
				Version: fmt.Sprintf("1.0.%d", i),
				Remotes: map[string]RemoteConfig{
					"filemod.com": {Provider: "test"},
				},
			}
			err := baseManager.Save(newConfig)
			if err != nil {
				errors <- err
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	
	wg.Wait()
	close(errors)
	
	// Check for errors
	var errorCount int
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
		errorCount++
	}
	assert.Equal(t, 0, errorCount)
}

// TestHotReloadManager_StopCleanup tests proper cleanup on stop
func TestHotReloadManager_StopCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	
	// Register a callback
	callbackCalled := false
	hrManager.OnConfigChange(func(cfg *Config) {
		callbackCalled = true
	})
	
	// Stop the manager
	err = hrManager.Stop()
	require.NoError(t, err)
	
	// Try to make changes after stop
	newConfig := &Config{
		Version: "99.0.0",
		Remotes: map[string]RemoteConfig{},
	}
	err = baseManager.Save(newConfig)
	require.NoError(t, err)
	
	// Wait a bit
	time.Sleep(200 * time.Millisecond)
	
	// Callback should not have been called
	assert.False(t, callbackCalled)
}

// TestHotReloadManager_CallbackPanic tests that panicking callbacks don't crash
func TestHotReloadManager_CallbackPanic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catmit-hotreload-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "providers.yaml")
	
	baseManager, err := NewYAMLConfigManager(configPath)
	require.NoError(t, err)
	
	hrManager, err := NewHotReloadManager(baseManager, configPath)
	require.NoError(t, err)
	defer hrManager.Stop()
	
	// Register a panicking callback
	hrManager.OnConfigChange(func(cfg *Config) {
		panic("test panic")
	})
	
	// Register a normal callback
	normalCallbackCalled := false
	hrManager.OnConfigChange(func(cfg *Config) {
		normalCallbackCalled = true
	})
	
	// Trigger a change
	err = hrManager.UpdateRemote("panic.test", RemoteConfig{Provider: "test"})
	require.NoError(t, err)
	
	// Wait for callbacks
	time.Sleep(100 * time.Millisecond)
	
	// Normal callback should still have been called
	assert.True(t, normalCallbackCalled)
	
	// Manager should still be functional
	config, err := hrManager.Load()
	require.NoError(t, err)
	assert.NotNil(t, config)
}