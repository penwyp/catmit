package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/provider"
)

// Minimum version requirements for CLI tools
var minVersionRequirements = map[string]string{
	"github": "2.0.0",
	"gitea":  "0.8.0",
}

// GitRunner Git命令执行器接口
type GitRunner interface {
	GetRemoteURL(ctx context.Context, remote string) (string, error)
	GetCurrentBranch(ctx context.Context) (string, error)
	GetCommitMessage(ctx context.Context, ref string) (string, error)
	GetDefaultBranch(ctx context.Context, remote string) (string, error)
}

// ProviderDetector Provider检测器接口
type ProviderDetector interface {
	DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error)
}

// CLIDetector CLI检测器接口
type CLIDetector interface {
	DetectCLI(ctx context.Context, provider string) (cli.CLIStatus, error)
	CheckMinVersion(current, minimum string) (bool, error)
}

// CommandBuilderInterface 命令构建器接口
type CommandBuilderInterface interface {
	BuildCommand(provider string, options PROptions) (string, []string, error)
	ParseGitHubPROutput(output string) (string, error)
	ParseGiteaPROutput(output string) (string, error)
}

// CommandRunner 命令执行器接口
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Creator PR创建器
type Creator struct {
	git              GitRunner
	providerDetector ProviderDetector
	cliDetector      CLIDetector
	commandBuilder   CommandBuilderInterface
	commandRunner    CommandRunner
}

// NewCreator 创建新的PR创建器
func NewCreator(
	git GitRunner,
	providerDetector ProviderDetector,
	cliDetector CLIDetector,
	commandBuilder CommandBuilderInterface,
	commandRunner CommandRunner,
) *Creator {
	return &Creator{
		git:              git,
		providerDetector: providerDetector,
		cliDetector:      cliDetector,
		commandBuilder:   commandBuilder,
		commandRunner:    commandRunner,
	}
}

// Create 创建PR
func (c *Creator) Create(ctx context.Context, options CreateOptions) (string, error) {
	// 设置默认值
	if options.Remote == "" {
		options.Remote = "origin"
	}

	// 获取remote URL
	remoteURL, err := c.git.GetRemoteURL(ctx, options.Remote)
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	// 检测provider
	remoteInfo, err := c.providerDetector.DetectFromRemote(ctx, remoteURL)
	if err != nil {
		return "", fmt.Errorf("failed to detect provider: %w", err)
	}

	// 检查是否支持的provider
	if remoteInfo.Provider == "unknown" {
		return "", fmt.Errorf("unsupported provider: %s", remoteInfo.Provider)
	}

	// 检测CLI状态
	cliStatus, err := c.cliDetector.DetectCLI(ctx, remoteInfo.Provider)
	if err != nil {
		return "", fmt.Errorf("failed to detect CLI: %w", err)
	}

	// 检查CLI是否安装
	if !cliStatus.Installed {
		return "", fmt.Errorf("%s is not installed", cliStatus.Name)
	}

	// 检查是否认证
	if !cliStatus.Authenticated {
		return "", fmt.Errorf("%s is not authenticated", cliStatus.Name)
	}

	// 检查版本要求
	if minVersion, ok := minVersionRequirements[remoteInfo.Provider]; ok {
		meetsRequirement, err := c.cliDetector.CheckMinVersion(cliStatus.Version, minVersion)
		if err != nil {
			return "", fmt.Errorf("failed to check version: %w", err)
		}
		if !meetsRequirement {
			return "", fmt.Errorf("%s version %s is below minimum required version %s", 
				cliStatus.Name, cliStatus.Version, minVersion)
		}
	}

	// 获取当前分支（如果需要）
	var headBranch string
	if options.HeadBranch == "" && remoteInfo.Provider == "gitea" {
		headBranch, err = c.git.GetCurrentBranch(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get current branch: %w", err)
		}
		options.HeadBranch = headBranch
	}

	// 构建PR选项
	prOptions := PROptions{
		Title:      options.Title,
		Body:       options.Body,
		BaseBranch: options.BaseBranch,
		HeadBranch: options.HeadBranch,
		Draft:      options.Draft,
		Labels:     options.Labels,
		Assignees:  options.Assignees,
		Reviewers:  options.Reviewers,
		Fill:       options.Fill,
	}

	// 构建命令
	cmd, args, err := c.commandBuilder.BuildCommand(remoteInfo.Provider, prOptions)
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}

	// 执行命令
	output, err := c.commandRunner.Run(ctx, cmd, args...)
	outputStr := string(output)

	// 解析输出获取PR URL
	var prURL string
	var parseErr error

	switch remoteInfo.Provider {
	case "github":
		prURL, parseErr = c.commandBuilder.ParseGitHubPROutput(outputStr)
	case "gitea":
		prURL, parseErr = c.commandBuilder.ParseGiteaPROutput(outputStr)
	}

	// 如果解析成功，返回URL（即使命令执行失败，如PR已存在的情况）
	if parseErr == nil && prURL != "" {
		return prURL, nil
	}

	// 如果命令执行失败且没有解析到URL，返回错误
	if err != nil {
		// 检查是否是PR已存在的情况
		if strings.Contains(outputStr, "already exists") {
			return "", fmt.Errorf("PR already exists but failed to parse URL: %s", outputStr)
		}
		return "", fmt.Errorf("failed to create PR: %w\nOutput: %s", err, outputStr)
	}

	// 如果命令成功但解析失败
	if parseErr != nil {
		return "", fmt.Errorf("failed to parse PR URL from output: %w\nOutput: %s", parseErr, outputStr)
	}

	return prURL, nil
}