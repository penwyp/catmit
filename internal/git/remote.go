package git

import (
	"context"
	"fmt"
	"strings"
)

// remoteManager Git远程仓库管理器实现
type remoteManager struct {
	runner Runner
}

// NewRemoteManager 创建新的远程仓库管理器
func NewRemoteManager(runner Runner) RemoteManager {
	return &remoteManager{
		runner: runner,
	}
}

// GetRemotes 获取所有远程仓库
func (m *remoteManager) GetRemotes(ctx context.Context) ([]Remote, error) {
	output, err := m.runner.Run(ctx, "git", "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("failed to get remotes: %w", err)
	}

	// 解析git remote -v输出
	remotes := make(map[string]*Remote)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	for _, line := range lines {
		if line == "" {
			continue
		}

		// 格式: origin	https://github.com/owner/repo.git (fetch)
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		url := parts[1]
		typeStr := strings.Trim(parts[2], "()")

		if _, exists := remotes[name]; !exists {
			remotes[name] = &Remote{Name: name}
		}

		if typeStr == "fetch" {
			remotes[name].FetchURL = url
		} else if typeStr == "push" {
			remotes[name].PushURL = url
		}
	}

	// 转换为切片
	result := make([]Remote, 0, len(remotes))
	for _, remote := range remotes {
		result = append(result, *remote)
	}

	return result, nil
}

// SelectRemote 根据优先级选择远程仓库
func (m *remoteManager) SelectRemote(remotes []Remote, preferredName string) (*Remote, error) {
	if len(remotes) == 0 {
		return nil, fmt.Errorf("no remotes configured")
	}

	// 如果指定了远程仓库名
	if preferredName != "" {
		for _, remote := range remotes {
			if remote.Name == preferredName {
				return &remote, nil
			}
		}
		return nil, fmt.Errorf("remote '%s' not found", preferredName)
	}

	// 默认查找origin
	for _, remote := range remotes {
		if remote.Name == "origin" {
			return &remote, nil
		}
	}

	return nil, fmt.Errorf("no 'origin' remote found and no remote specified")
}

// GetCurrentBranch 获取当前分支名
func (m *remoteManager) GetCurrentBranch(ctx context.Context) (string, error) {
	output, err := m.runner.Run(ctx, "git", "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	branch := strings.TrimSpace(output)
	if branch == "" {
		return "", fmt.Errorf("not on any branch (detached HEAD)")
	}

	return branch, nil
}

// HasUpstreamBranch 检查分支是否有上游分支
func (m *remoteManager) HasUpstreamBranch(ctx context.Context, branch string) bool {
	_, err := m.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	return err == nil
}