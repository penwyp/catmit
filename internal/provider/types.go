package provider

import (
	"context"
	"errors"
)

// RemoteInfo 包含解析后的Git remote信息
type RemoteInfo struct {
	Provider string // github, gitlab, gitea, bitbucket, unknown
	Host     string // 主机名，如 github.com
	Port     int    // 端口号，0表示默认端口
	Owner    string // 仓库所有者或组织
	Repo     string // 仓库名称
	Protocol string // https, ssh
}

// ProbeResult HTTP探测结果
type ProbeResult struct {
	IsGitea bool
	Version string
	Error   error
}

// Detector Provider检测器接口
type Detector interface {
	// DetectFromRemoteURL 从Git remote URL检测Provider类型
	DetectFromRemoteURL(url string) (*RemoteInfo, error)
	
	// ProbeHTTP 通过HTTP探测确认Provider类型
	ProbeHTTP(baseURL string) (*ProbeResult, error)
}

// HTTPProber HTTP探测器接口
type HTTPProber interface {
	ProbeGitea(ctx context.Context, baseURL string) ProbeResult
}

// 错误定义
var (
	ErrProbeTimeout = errors.New("probe timeout")
)