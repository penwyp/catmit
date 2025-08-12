package template

import (
	"context"
	"github.com/penwyp/catmit/internal/provider"
)

// Template represents PR template information
type Template struct {
	Provider  string              // Provider type: github, gitlab, gitea, bitbucket
	Path      string              // Template file path
	Name      string              // Template name (for multi-template scenarios)
	Content   string              // Raw template content
	Sections  map[string]*Section // Parsed sections
	Variables []Variable          // Detected variables/placeholders
}

// Section represents a template section
type Section struct {
	Name     string // Section name, e.g., "Description", "Checklist", "Testing"
	Content  string // Section content
	Required bool   // Whether this section is required
	Level    int    // Heading level (1-6)
}

// Variable represents a template variable
type Variable struct {
	Name        string // Variable name, e.g., "CommitMessage", "Branch"
	Placeholder string // Original placeholder text, e.g., "{{.CommitMessage}}"
	Description string // Description extracted from template comments
	Required    bool   // Whether this variable is required
}

// TemplateData holds data for populating the template
type TemplateData struct {
	// Basic information
	CommitMessage string // Generated commit message
	CommitTitle   string // Commit message title line
	CommitBody    string // Commit message body
	Branch        string // Current branch name
	BaseBranch    string // Target branch name
	Remote        string // Remote repository name
	RepoOwner     string // Repository owner
	RepoName      string // Repository name

	// File change information
	ChangedFiles []string             // List of changed files
	FileStats    map[string]*FileStat // File statistics
	FilesCount   int                  // Total number of changed files
	AddedLines   int                  // Number of added lines
	DeletedLines int                  // Number of deleted lines

	// Commit history
	RecentCommits []string // Recent commit messages

	// Change summary
	ChangesSummary string // Overall description of changes

	// Additional metadata
	IssueNumber    string // Issue number extracted from branch name or commit message
	BreakingChange bool   // Whether this contains breaking changes
	TestsAdded     bool   // Whether tests were added
	DocsUpdated    bool   // Whether documentation was updated
}

// FileStat represents file change statistics
type FileStat struct {
	Path      string // File path
	Added     int    // Number of added lines
	Deleted   int    // Number of deleted lines
	IsNew     bool   // Whether this is a new file
	IsDeleted bool   // Whether this file was deleted
	IsRenamed bool   // Whether this file was renamed
	OldPath   string // Path before renaming
}

// Manager is the template manager interface
type Manager interface {
	// LoadTemplate loads a template based on provider info
	LoadTemplate(ctx context.Context, info *provider.RemoteInfo) (*Template, error)

	// ProcessTemplate processes the template and fills variables
	ProcessTemplate(ctx context.Context, tmpl *Template, data *TemplateData) (string, error)
}

// Loader is the template loader interface
type Loader interface {
	// Load loads the template for the specified provider
	Load(ctx context.Context, provider string) (*Template, error)

	// ListTemplates lists all available templates
	ListTemplates(ctx context.Context, provider string) ([]*Template, error)
}

// Parser is the template parser interface
type Parser interface {
	// Parse parses the template content
	Parse(content string) (*Template, error)

	// ExtractSections extracts template sections
	ExtractSections(content string) (map[string]*Section, error)

	// ExtractVariables extracts template variables
	ExtractVariables(content string) ([]Variable, error)
}

// Processor is the template processor interface
type Processor interface {
	// Process processes the template and replaces variables
	Process(tmpl *Template, data *TemplateData) (string, error)

	// ValidateRequired validates required fields
	ValidateRequired(tmpl *Template, data *TemplateData) error
}
