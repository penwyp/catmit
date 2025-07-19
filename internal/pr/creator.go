package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/penwyp/catmit/internal/template"
)

// ErrPRAlreadyExists is returned when a PR already exists for the branch
// This wraps the framework error with additional URL information
type ErrPRAlreadyExists struct {
	URL string
	err error
}

func (e *ErrPRAlreadyExists) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return "pull request already exists: " + e.URL
}

func (e *ErrPRAlreadyExists) Unwrap() error {
	return e.err
}

// Minimum version requirements for CLI tools
var minVersionRequirements = map[string]string{
	"github": "2.0.0",
	"gitea":  "0.8.0",
	"gitlab": "1.0.0",
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
	ParseGitLabMROutput(output string) (string, error)
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
	templateManager  template.Manager // 可选的模板管理器
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

// WithTemplateManager 设置模板管理器
func (c *Creator) WithTemplateManager(tm template.Manager) *Creator {
	c.templateManager = tm
	return c
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
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get remote URL", err)
	}

	// 检测provider
	remoteInfo, err := c.providerDetector.DetectFromRemote(ctx, remoteURL)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeProvider, "failed to detect provider", err)
	}

	// 检查是否支持的provider
	if remoteInfo.Provider == "unknown" {
		return "", errors.ErrProviderNotSupported
	}

	// 检测CLI状态
	cliStatus, err := c.cliDetector.DetectCLI(ctx, remoteInfo.Provider)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypePR, "failed to detect CLI", err)
	}

	// 检查CLI是否安装
	if !cliStatus.Installed {
		return "", errors.ErrCLINotInstalled.WithSuggestion(fmt.Sprintf("请安装 %s CLI 工具", cliStatus.Name))
	}

	// 检查是否认证
	if !cliStatus.Authenticated {
		return "", errors.ErrCLINotAuthed.WithSuggestion(fmt.Sprintf("请运行 %s auth login 进行认证", cliStatus.Name))
	}

	// 检查版本要求
	if minVersion, ok := minVersionRequirements[remoteInfo.Provider]; ok {
		meetsRequirement, err := c.cliDetector.CheckMinVersion(cliStatus.Version, minVersion)
		if err != nil {
			return "", errors.Wrap(errors.ErrTypePR, "failed to check version", err)
		}
		if !meetsRequirement {
			return "", errors.New(errors.ErrTypePR, fmt.Sprintf("%s version %s is below minimum required version %s", 
				cliStatus.Name, cliStatus.Version, minVersion)).WithSuggestion(fmt.Sprintf("请升级 %s 到 %s 或更高版本", cliStatus.Name, minVersion))
		}
	}

	// 获取基础分支（如果未指定）
	if options.BaseBranch == "" {
		defaultBranch, err := c.git.GetDefaultBranch(ctx, options.Remote)
		if err != nil {
			// 如果获取失败，使用常见的默认值
			options.BaseBranch = "main"
		} else {
			options.BaseBranch = defaultBranch
		}
	}

	// 获取当前分支（如果需要）
	var headBranch string
	if options.HeadBranch == "" {
		headBranch, err = c.git.GetCurrentBranch(ctx)
		if err != nil {
			return "", errors.Wrap(errors.ErrTypeGit, "failed to get current branch", err)
		}
		if remoteInfo.Provider == "gitea" {
			options.HeadBranch = headBranch
		}
	} else {
		headBranch = options.HeadBranch
	}

	// 处理模板（如果启用）
	if options.UseTemplate && c.templateManager != nil {
		// 尝试加载模板
		tmpl, err := c.templateManager.LoadTemplate(ctx, &remoteInfo)
		if err == nil {
			// 如果成功加载模板，准备模板数据
			templateData := options.TemplateData
			if templateData == nil {
				// 如果没有提供模板数据，创建基础数据
				templateData = &template.TemplateData{
					Branch:     headBranch,
					BaseBranch: options.BaseBranch,
					Remote:     options.Remote,
					RepoOwner:  remoteInfo.Owner,
					RepoName:   remoteInfo.Repo,
				}
				
				// 如果有标题和描述，使用它们
				if options.Title != "" {
					templateData.CommitTitle = options.Title
				}
				if options.Body != "" {
					templateData.CommitMessage = options.Body
					templateData.CommitBody = options.Body
				}
			}
			
			// 处理模板
			processedBody, err := c.templateManager.ProcessTemplate(ctx, tmpl, templateData)
			if err == nil {
				// 成功处理，使用模板生成的内容
				options.Body = processedBody
				// 如果模板中包含标题，可能需要从中提取
				// 但通常标题是单独的字段
			}
			// 如果模板处理失败，继续使用原始内容
		}
		// 如果模板加载失败，继续使用原始内容
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
		return "", errors.Wrap(errors.ErrTypePR, "failed to build command", err)
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
	case "gitlab":
		prURL, parseErr = c.commandBuilder.ParseGitLabMROutput(outputStr)
	}

	// 如果命令执行失败
	if err != nil {
		// 检查是否是PR已存在的情况
		if strings.Contains(outputStr, "already exists") {
			// 如果解析到了URL，返回特定的错误
			if parseErr == nil && prURL != "" {
				return "", &ErrPRAlreadyExists{URL: prURL, err: errors.ErrPRAlreadyExists}
			}
			return "", errors.Wrapf(errors.ErrTypePR, "PR already exists but failed to parse URL", errors.New(errors.ErrTypePR, outputStr))
		}
		return "", errors.Wrapf(errors.ErrTypePR, "failed to create PR\nOutput: %s", err, outputStr)
	}

	// 如果命令成功且解析到URL
	if parseErr == nil && prURL != "" {
		return prURL, nil
	}

	// 如果命令成功但解析失败
	if parseErr != nil {
		return "", errors.Wrap(errors.ErrTypePR, fmt.Sprintf("failed to parse PR URL from output\nOutput: %s", outputStr), parseErr)
	}

	return prURL, nil
}