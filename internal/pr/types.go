package pr

import "github.com/penwyp/catmit/internal/template"

// PROptions PR创建选项
type PROptions struct {
	// 通用字段
	Title      string   // PR标题
	Body       string   // PR描述/正文
	BaseBranch string   // 目标分支（必需）
	HeadBranch string   // 源分支（Gitea必需）
	Draft      bool     // 是否为草稿PR
	
	// 元数据
	Labels    []string // 标签
	Assignees []string // 分配人
	Reviewers []string // 审查人
	Milestone string   // 里程碑
	
	// 特殊选项
	Fill bool // GitHub的--fill选项，自动填充标题和描述
}

// CreateOptions PR创建的高级选项
type CreateOptions struct {
	Remote     string   // Git remote名称，默认为origin
	Title      string   // PR标题
	Body       string   // PR描述
	BaseBranch string   // 目标分支
	HeadBranch string   // 源分支（可选，自动检测）
	Draft      bool     // 是否为草稿
	Labels     []string // 标签
	Assignees  []string // 分配人
	Reviewers  []string // 审查人
	Fill       bool     // 使用--fill选项
	
	// 模板相关选项
	UseTemplate   bool                   // 是否使用模板
	TemplateData  *template.TemplateData // 模板数据（如果提供）
}