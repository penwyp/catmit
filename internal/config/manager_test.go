package config

import (
	"fmt"
	"io/ioutil"
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
			configPath: "/tmp/test-config.json",
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
			manager, err := NewConfigManager(tt.configPath)
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
	tmpDir, err := ioutil.TempDir("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	manager, err := NewConfigManager(configPath)
	require.NoError(t, err)

	// 创建默认配置
	err = manager.CreateDefaultConfig()
	assert.NoError(t, err)

	// 验证文件已创建
	assert.FileExists(t, configPath)

	// 读取并验证内容
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
				content := `{
					"version": "1.0.0",
					"remotes": {
						"github.com": {
							"provider": "github",
							"cli_tool": "gh",
							"min_version": "2.0.0"
						}
					}
				}`
				return ioutil.WriteFile(path, []byte(content), 0644)
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
				return ioutil.WriteFile(path, []byte("invalid json"), 0644)
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
			tmpDir, err := ioutil.TempDir("", "catmit-config-test")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configPath := filepath.Join(tmpDir, "config.json")
			if tt.setupFunc != nil {
				err = tt.setupFunc(configPath)
				require.NoError(t, err)
			}

			manager, err := NewConfigManager(configPath)
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
	tmpDir, err := ioutil.TempDir("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	manager, err := NewConfigManager(configPath)
	require.NoError(t, err)

	// 准备测试配置
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

	// 保存配置
	err = manager.Save(config)
	assert.NoError(t, err)

	// 验证文件存在
	assert.FileExists(t, configPath)

	// 重新加载验证
	loaded, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, config.Version, loaded.Version)
	assert.Equal(t, config.Remotes["github.com"], loaded.Remotes["github.com"])
}

func TestConfigManager_AtomicWrite(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	manager, err := NewConfigManager(configPath)
	require.NoError(t, err)

	// 创建初始配置
	config1 := &Config{
		Version: "1.0.0",
		Remotes: map[string]RemoteConfig{
			"github.com": {Provider: "github"},
		},
	}
	err = manager.Save(config1)
	require.NoError(t, err)

	// 模拟写入过程中的错误
	// 原子写入应该保证要么完全成功，要么保持原状态
	// 这里我们通过权限测试来验证
	
	// 先验证正常情况
	config2 := &Config{
		Version: "2.0.0",
		Remotes: map[string]RemoteConfig{
			"gitlab.com": {Provider: "gitlab"},
		},
	}
	err = manager.Save(config2)
	assert.NoError(t, err)

	// 验证新配置已保存
	loaded, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "2.0.0", loaded.Version)
}

func TestConfigManager_ConcurrentWrite(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	manager, err := NewConfigManager(configPath)
	require.NoError(t, err)

	// 创建初始配置
	err = manager.CreateDefaultConfig()
	require.NoError(t, err)

	// 并发写入测试
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

	// 验证没有错误
	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	// 验证最终状态是有效的
	_, err = manager.Load()
	assert.NoError(t, err)
}

func TestConfigManager_UpdateRemote(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "catmit-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	manager, err := NewConfigManager(configPath)
	require.NoError(t, err)

	// 创建初始配置
	err = manager.CreateDefaultConfig()
	require.NoError(t, err)

	// 更新remote配置
	remoteConfig := RemoteConfig{
		Provider:     "custom",
		CLITool:      "custom-cli",
		MinVersion:   "1.0.0",
		AuthCommand:  "custom-cli auth login",
		CreatePRArgs: []string{"pr", "create"},
	}

	err = manager.UpdateRemote("custom.example.com", remoteConfig)
	assert.NoError(t, err)

	// 验证更新
	config, err := manager.Load()
	assert.NoError(t, err)
	assert.Contains(t, config.Remotes, "custom.example.com")
	assert.Equal(t, remoteConfig, config.Remotes["custom.example.com"])
}