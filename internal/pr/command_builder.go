package pr

import (
	"fmt"
	"regexp"
	"strings"
	
	"github.com/penwyp/catmit/internal/errors"
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
	case "gitlab":
		return b.BuildGitLabMRCommand(options)
	default:
		return "", nil, errors.New(errors.ErrTypeProvider, fmt.Sprintf("unsupported provider: %s", provider)).WithSuggestion("当前支持 GitHub、GitLab、Gitea 和 Bitbucket")
	}
}

// BuildGitHubPRCommand 构建GitHub CLI的PR创建命令
func (b *CommandBuilder) BuildGitHubPRCommand(options PROptions) (string, []string, error) {
	// 验证必需字段
	if options.BaseBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "base branch is required").WithSuggestion("使用 --pr-base 参数指定基础分支")
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
		return "", nil, errors.New(errors.ErrTypeValidation, "base branch is required").WithSuggestion("使用 --pr-base 参数指定基础分支")
	}
	if options.HeadBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "head branch is required for Gitea")
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
	return "", errors.New(errors.ErrTypePR, "no PR URL found in output")
}

// ParseGiteaPROutput 解析tea CLI的输出获取PR URL
func (b *CommandBuilder) ParseGiteaPROutput(output string) (string, error) {
	// Gitea PR URL的正则表达式（更通用，支持各种域名）
	urlRegex := regexp.MustCompile(`https?://[^\s]+/pulls?/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", errors.New(errors.ErrTypePR, "no PR URL found in output")
}

// BuildGitLabMRCommand 构建GitLab CLI的MR创建命令
func (b *CommandBuilder) BuildGitLabMRCommand(options PROptions) (string, []string, error) {
	// 验证必需字段
	if options.BaseBranch == "" {
		return "", nil, errors.New(errors.ErrTypeValidation, "base branch is required").WithSuggestion("使用 --pr-base 参数指定基础分支")
	}

	args := []string{"mr", "create"}

	// 如果使用fill选项，只需提供基础分支
	if options.Fill {
		args = append(args, "--fill")
		args = append(args, "--target-branch", options.BaseBranch)
		if options.Draft {
			args = append(args, "--draft")
		}
		// Remove WIP prefix by default
		args = append(args, "--remove-source-branch=false")
		return "glab", args, nil
	}

	// 标题和描述
	if options.Title != "" {
		args = append(args, "--title", options.Title)
	}
	if options.Body != "" {
		args = append(args, "--description", options.Body)
	}

	// 目标分支
	args = append(args, "--target-branch", options.BaseBranch)

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
		// GitLab uses --reviewer for reviewers
		args = append(args, "--reviewer", strings.Join(options.Reviewers, ","))
	}

	// 里程碑
	if options.Milestone != "" {
		args = append(args, "--milestone", options.Milestone)
	}

	// Don't remove source branch by default
	args = append(args, "--remove-source-branch=false")

	return "glab", args, nil
}

// ParseGitLabMROutput 解析glab CLI的输出获取MR URL
func (b *CommandBuilder) ParseGitLabMROutput(output string) (string, error) {
	// GitLab MR URL的正则表达式
	urlRegex := regexp.MustCompile(`https?://[^\s]+/-/merge_requests/\d+`)
	matches := urlRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", errors.New(errors.ErrTypePR, "no MR URL found in output")
}