package template

import (
	"context"
	"os"
	"path/filepath"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/provider"
)

// DefaultManager is the default implementation of the template manager
type DefaultManager struct {
	loader    Loader
	parser    Parser
	processor Processor
	log       logger.Logger
}

// NewDefaultManager creates a new default manager
func NewDefaultManager(basePath string) *DefaultManager {
	fileLoader := NewFileLoader(basePath)
	cachedLoader := NewCachedLoader(fileLoader)

	return &DefaultManager{
		loader:    cachedLoader,
		parser:    NewMarkdownParser(),
		processor: NewTemplateProcessor(),
		log:       logger.NewDefault(),
	}
}

// LoadTemplate loads a template based on provider info
func (m *DefaultManager) LoadTemplate(ctx context.Context, info *provider.RemoteInfo) (*Template, error) {
	m.log.Debugf("Loading template for provider: %s", info.Provider)

	// Load the raw template
	tmpl, err := m.loader.Load(ctx, info.Provider)
	if err != nil {
		// If template not found, try the generic template
		if errors.Is(err, ErrTemplateNotFound) && info.Provider != "github" {
			m.log.Debugf("Provider-specific template not found, trying generic template")
			tmpl, err = m.loader.Load(ctx, "github")
		}

		if err != nil {
			return nil, err
		}
	}

	// Parse the template structure
	parsed, err := m.parser.Parse(tmpl.Content)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeValidation, "template parsing failed", err)
	}

	// Merge parsing results
	tmpl.Sections = parsed.Sections
	tmpl.Variables = parsed.Variables
	tmpl.Provider = info.Provider

	return tmpl, nil
}

// ProcessTemplate processes the template and fills in variables
func (m *DefaultManager) ProcessTemplate(ctx context.Context, tmpl *Template, data *TemplateData) (string, error) {
	m.log.Debugf("Processing template with data")

	// Process the template
	result, err := m.processor.Process(tmpl, data)
	if err != nil {
		return "", err
	}

	return result, nil
}

// ConfigurableManager is a configurable template manager
type ConfigurableManager struct {
	*DefaultManager
	config *ManagerConfig
}

// ManagerConfig is the configuration for the manager
type ManagerConfig struct {
	// TemplateDirs are additional template search directories
	TemplateDirs []string

	// DefaultProvider is the default provider type
	DefaultProvider string

	// StrictMode, if true, will error on missing required fields
	StrictMode bool

	// CustomFunctions are custom template functions
	CustomFunctions map[string]interface{}
}

// NewConfigurableManager creates a configurable manager
func NewConfigurableManager(basePath string, config *ManagerConfig) *ConfigurableManager {
	if config == nil {
		config = &ManagerConfig{
			DefaultProvider: "github",
			StrictMode:      false,
		}
	}

	return &ConfigurableManager{
		DefaultManager: NewDefaultManager(basePath),
		config:         config,
	}
}

// LoadTemplate loads a template (supports custom directories)
func (m *ConfigurableManager) LoadTemplate(ctx context.Context, info *provider.RemoteInfo) (*Template, error) {
	// First, try to load from custom directories
	for _, dir := range m.config.TemplateDirs {
		loader := NewFileLoader(dir)
		tmpl, err := loader.Load(ctx, info.Provider)
		if err == nil {
			// Parse the template
			parsed, err := m.parser.Parse(tmpl.Content)
			if err != nil {
				continue
			}
			tmpl.Sections = parsed.Sections
			tmpl.Variables = parsed.Variables
			tmpl.Provider = info.Provider
			return tmpl, nil
		}
	}

	// Use the default loading logic
	return m.DefaultManager.LoadTemplate(ctx, info)
}

// Helper functions

// FindRepositoryRoot finds the repository root directory
func FindRepositoryRoot() (string, error) {
	// Start from the current directory and look upwards for a .git directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the root of the filesystem
			break
		}
		dir = parent
	}

	return "", errors.New(errors.ErrTypeGit, "not in a git repository")
}

// CreateTemplateData creates template data from various sources
func CreateTemplateData(commitMsg string, branch string, changedFiles []string) *TemplateData {
	data := &TemplateData{
		CommitMessage: commitMsg,
		Branch:        branch,
		ChangedFiles:  changedFiles,
		FilesCount:    len(changedFiles),
		FileStats:     make(map[string]*FileStat),
	}

	// Initialize file statistics
	for _, file := range changedFiles {
		data.FileStats[file] = &FileStat{
			Path: file,
		}
	}

	return data
}

// EnrichTemplateData enriches template data with additional information
func EnrichTemplateData(data *TemplateData, info *provider.RemoteInfo) {
	if info != nil {
		data.RepoOwner = info.Owner
		data.RepoName = info.Repo
		data.Remote = "origin" // Default value, can be obtained elsewhere
	}

	// Set the default base branch if not set
	if data.BaseBranch == "" {
		data.BaseBranch = "main"
	}
}
