package cli

import "context"

// Version 语义化版本结构
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string
	Build      string
}

// CLIStatus CLI工具状态信息
type CLIStatus struct {
	Name          string // CLI名称 (gh, tea等)
	Installed     bool   // 是否已安装
	Version       string // 版本号
	Authenticated bool   // 是否已认证
	Username      string // 认证的用户名
}

// CommandRunner 命令执行器接口
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}