package git

import "context"

// Remote Git远程仓库信息
type Remote struct {
	Name     string // 远程仓库名称，如 origin
	FetchURL string // 拉取URL
	PushURL  string // 推送URL
}

// Runner Git命令执行器接口
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (string, error)
}

// RemoteManager Git远程仓库管理器
type RemoteManager interface {
	// GetRemotes 获取所有远程仓库
	GetRemotes(ctx context.Context) ([]Remote, error)
	
	// SelectRemote 根据优先级选择远程仓库
	SelectRemote(remotes []Remote, preferredName string) (*Remote, error)
	
	// GetCurrentBranch 获取当前分支名
	GetCurrentBranch(ctx context.Context) (string, error)
	
	// HasUpstreamBranch 检查分支是否有上游分支
	HasUpstreamBranch(ctx context.Context, branch string) bool
}