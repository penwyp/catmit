package pr

import (
	"fmt"
	"regexp"
	"strings"
)

// CommandBuilder PR命令构建器
type CommandBuilder struct{}

// NewCommandBuilder 创建新的命令构建器
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

// BuildCommand 根据provider构建相应的PR命令
func (b *CommandBuilder) BuildCommand(provider string, options PROptions) (string, []string, error) {
	switch provider {
	case "github":
		return b.BuildGitHubPRCommand(options)
	case "gitea":
		return b.BuildGiteaPRCommand(options)
	default:
		return "", nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// BuildGitHubPRCommand 构建GitHub CLI的PR创建命令
func (b *CommandBuilder) BuildGitHubPRCommand(options PROptions) (string, []string, error) {
	// 验证必需字段
	if options.BaseBranch == "" {
		return "", nil, fmt.Errorf("base branch is required")
	}

	args := []string{"pr", "create"}

	// 如果使用fill选项，其他选项可能不需要
	if options.Fill {
		args = append(args, "--fill")
		args = append(args, "--base", options.BaseBranch)
		if options.Draft {
			args = append(args, "--draft=true")
		} else {
			args = append(args, "--draft=false")
		}
		return "gh", args, nil
	}

	// 标题和正文
	if options.Title != "" {
		args = append(args, "--title", options.Title)
	}
	if options.Body != "" {
		args = append(args, "--body", options.Body)
	}

	// 基础分支
	args = append(args, "--base", options.BaseBranch)

	// 草稿状态
	if options.Draft {
		args = append(args, "--draft")
	}

	// 分配人
	if len(options.Assignees) > 0 {
		args = append(args, "--assignee", strings.Join(options.Assignees, ","))
	}

	// 标签
	if len(options.Labels) > 0 {
		args = append(args, "--label", strings.Join(options.Labels, ","))
	}

	// 审查人
	if len(options.Reviewers) > 0 {
		args = append(args, "--reviewer", strings.Join(options.Reviewers, ","))
	}

	return "gh", args, nil
}

// BuildGiteaPRCommand 构建tea CLI的PR创建命令
func (b *CommandBuilder) BuildGiteaPRCommand(options PROptions) (string, []string, error) {
	// 验证必需字段
	if options.BaseBranch == "" {
		return "", nil, fmt.Errorf("base branch is required")
	}
	if options.HeadBranch == "" {
		return "", nil, fmt.Errorf("head branch is required for Gitea")
	}

	args := []string{"pr", "create"}

	// 标题和描述（Gitea使用description而不是body）
	if options.Title != "" {
		args = append(args, "--title", options.Title)
	}
	if options.Body != "" {
		args = append(args, "--description", options.Body)
	}

	// 分支
	args = append(args, "--base", options.BaseBranch)
	args = append(args, "--head", options.HeadBranch)

	// 分配人（Gitea使用assignees）
	if len(options.Assignees) > 0 {
		args = append(args, "--assignees", strings.Join(options.Assignees, ","))
	}

	// 标签
	if len(options.Labels) > 0 {
		args = append(args, "--labels", strings.Join(options.Labels, ","))
	}

	// 里程碑
	if options.Milestone != "" {
		args = append(args, "--milestone", options.Milestone)
	}

	return "tea", args, nil
}

// ParseGitHubPROutput 解析GitHub CLI的输出获取PR URL
func (b *CommandBuilder) ParseGitHubPROutput(output string) (string, error) {
	// GitHub PR URL的正则表达式
	urlRegex := regexp.MustCompile(`https://github\.com/[\w-]+/[\w-]+/pull/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("no PR URL found in output")
}

// ParseGiteaPROutput 解析tea CLI的输出获取PR URL
func (b *CommandBuilder) ParseGiteaPROutput(output string) (string, error) {
	// Gitea PR URL的正则表达式（更通用，支持各种域名）
	urlRegex := regexp.MustCompile(`https?://[^\s]+/pulls?/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("no PR URL found in output")
}