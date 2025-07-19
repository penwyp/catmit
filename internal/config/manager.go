package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	
	"github.com/penwyp/catmit/internal/errors"
)

// configManager 配置文件管理器实现
type configManager struct {
	configPath string
	mu         sync.Mutex // 保护并发写入
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(configPath string) (Manager, error) {
	if configPath == "" {
		return nil, errors.New(errors.ErrTypeConfig, "config path cannot be empty")
	}

	return &configManager{
		configPath: configPath,
	}, nil
}

// Load 加载配置文件
func (m *configManager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := ioutil.ReadFile(m.configPath)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to read config file", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to parse config", err)
	}

	return &config, nil
}

// Save 保存配置文件（原子操作）
func (m *configManager) Save(config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 序列化配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to marshal config", err)
	}

	// 确保目录存在
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// 原子写入：先写入临时文件，然后重命名
	tmpFile := m.configPath + ".tmp"
	if err := ioutil.WriteFile(tmpFile, data, 0644); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to write temp config file", err)
	}

	// 原子重命名
	if err := os.Rename(tmpFile, m.configPath); err != nil {
		// 清理临时文件
		os.Remove(tmpFile)
		return errors.Wrap(errors.ErrTypeConfig, "failed to save config file", err)
	}

	return nil
}

// CreateDefaultConfig 创建默认配置
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

// UpdateRemote 更新指定remote的配置
func (m *configManager) UpdateRemote(host string, remoteConfig RemoteConfig) error {
	// 加载现有配置
	config, err := m.Load()
	if err != nil {
		// 如果配置文件不存在，创建新配置
		if os.IsNotExist(err) {
			config = &Config{
				Version: "1.0.0",
				Remotes: make(map[string]RemoteConfig),
			}
		} else {
			return err
		}
	}

	// 更新remote配置
	config.Remotes[host] = remoteConfig

	// 保存配置
	return m.Save(config)
}