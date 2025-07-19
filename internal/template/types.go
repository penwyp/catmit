package template

import (
	"context"
	"github.com/penwyp/catmit/internal/provider"
)

// Template PR模板信息
type Template struct {
	Provider  string              // 提供商类型: github, gitlab, gitea, bitbucket
	Path      string              // 模板文件路径
	Name      string              // 模板名称（用于多模板时）
	Content   string              // 原始模板内容
	Sections  map[string]*Section // 解析后的章节
	Variables []Variable          // 检测到的变量/占位符
}

// Section 模板章节
type Section struct {
	Name     string // 章节名称，如 "Description", "Checklist", "Testing"
	Content  string // 章节内容
	Required bool   // 是否为必填章节
	Level    int    // 标题级别 (1-6)
}

// Variable 模板变量
type Variable struct {
	Name        string // 变量名，如 "CommitMessage", "Branch"
	Placeholder string // 原始占位符文本，如 "{{.CommitMessage}}"
	Description string // 从模板注释中提取的描述
	Required    bool   // 是否必填
}

// TemplateData 用于填充模板的数据
type TemplateData struct {
	// 基础信息
	CommitMessage string   // 生成的提交消息
	CommitTitle   string   // 提交消息的标题行
	CommitBody    string   // 提交消息的正文部分
	Branch        string   // 当前分支名
	BaseBranch    string   // 目标分支名
	Remote        string   // 远程仓库名
	RepoOwner     string   // 仓库所有者
	RepoName      string   // 仓库名称
	
	// 文件变更信息
	ChangedFiles []string              // 变更的文件列表
	FileStats    map[string]*FileStat  // 文件统计信息
	FilesCount   int                   // 变更文件总数
	AddedLines   int                   // 新增行数
	DeletedLines int                   // 删除行数
	
	// 提交历史
	RecentCommits []string // 最近的提交消息
	
	// 变更摘要
	ChangesSummary string // 变更的整体描述
	
	// 额外元数据
	IssueNumber  string   // 从分支名或提交消息中提取的issue编号
	BreakingChange bool   // 是否包含破坏性变更
	TestsAdded   bool     // 是否添加了测试
	DocsUpdated  bool     // 是否更新了文档
}

// FileStat 文件变更统计
type FileStat struct {
	Path         string // 文件路径
	Added        int    // 新增行数
	Deleted      int    // 删除行数
	IsNew        bool   // 是否为新文件
	IsDeleted    bool   // 是否被删除
	IsRenamed    bool   // 是否被重命名
	OldPath      string // 重命名前的路径
}

// Manager 模板管理器接口
type Manager interface {
	// LoadTemplate 根据provider信息加载模板
	LoadTemplate(ctx context.Context, info *provider.RemoteInfo) (*Template, error)
	
	// ProcessTemplate 处理模板，填充变量
	ProcessTemplate(ctx context.Context, tmpl *Template, data *TemplateData) (string, error)
}

// Loader 模板加载器接口
type Loader interface {
	// Load 加载指定provider的模板
	Load(ctx context.Context, provider string) (*Template, error)
	
	// ListTemplates 列出所有可用模板
	ListTemplates(ctx context.Context, provider string) ([]*Template, error)
}

// Parser 模板解析器接口
type Parser interface {
	// Parse 解析模板内容
	Parse(content string) (*Template, error)
	
	// ExtractSections 提取模板章节
	ExtractSections(content string) (map[string]*Section, error)
	
	// ExtractVariables 提取模板变量
	ExtractVariables(content string) ([]Variable, error)
}

// Processor 模板处理器接口
type Processor interface {
	// Process 处理模板，替换变量
	Process(tmpl *Template, data *TemplateData) (string, error)
	
	// ValidateRequired 验证必填项
	ValidateRequired(tmpl *Template, data *TemplateData) error
}