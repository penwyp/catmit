package config

// Config 配置文件结构
type Config struct {
	Version string                   `json:"version"`
	Remotes map[string]RemoteConfig `json:"remotes"`
}

// RemoteConfig 远程仓库配置
type RemoteConfig struct {
	Provider     string   `json:"provider"`      // github, gitlab, gitea等
	CLITool      string   `json:"cli_tool"`      // gh, glab, tea等
	MinVersion   string   `json:"min_version"`   // CLI工具最低版本要求
	AuthCommand  string   `json:"auth_command"`  // 认证命令
	CreatePRArgs []string `json:"create_pr_args"` // 创建PR的参数
}

// Manager 配置管理器接口
type Manager interface {
	// Load 加载配置文件
	Load() (*Config, error)
	
	// Save 保存配置文件（原子操作）
	Save(config *Config) error
	
	// CreateDefaultConfig 创建默认配置
	CreateDefaultConfig() error
	
	// UpdateRemote 更新指定remote的配置
	UpdateRemote(host string, config RemoteConfig) error
}