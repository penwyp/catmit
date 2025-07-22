package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
)

// Predefined errors
var (
	ErrTemplateNotFound = errors.New(
		errors.ErrTypeConfig,
		"PR template not found",
	).WithSuggestion("Create a template file, e.g., .github/PULL_REQUEST_TEMPLATE.md")

	ErrTemplateReadError = errors.NewRetryable(
		errors.ErrTypeConfig,
		"Failed to read template file",
	)
)

// templatePaths defines the template search paths for each provider
var templatePaths = map[string][]string{
	"github": {
		".github/PULL_REQUEST_TEMPLATE.md",
		".github/pull_request_template.md",
		".github/PULL_REQUEST_TEMPLATE/*.md",
		".github/pull_request_template/*.md",
		"PULL_REQUEST_TEMPLATE.md",
		"pull_request_template.md",
	},
	"gitlab": {
		".gitlab/merge_request_templates/*.md",
		".gitlab/merge_request_templates/Default.md",
		".gitlab/merge_request_templates/default.md",
	},
	"gitea": {
		".gitea/PULL_REQUEST_TEMPLATE.md",
		".gitea/pull_request_template.md",
		".gitea/PULL_REQUEST_TEMPLATE/*.md",
		".gitea/pull_request_template/*.md",
		"PULL_REQUEST_TEMPLATE.md",
		"pull_request_template.md",
	},
	"bitbucket": {
		".bitbucket/PULLREQUEST_TEMPLATE.md",
		".bitbucket/pullrequest_template.md",
		"PULLREQUEST_TEMPLATE.md",
		"pullrequest_template.md",
	},
}

// FileLoader loads templates from the file system
type FileLoader struct {
	basePath string // repository root directory
	log      logger.Logger
}

// NewFileLoader creates a new file loader
func NewFileLoader(basePath string) *FileLoader {
	return &FileLoader{
		basePath: basePath,
		log:      logger.NewDefault(),
	}
}

// Load loads the template for the specified provider
func (l *FileLoader) Load(ctx context.Context, provider string) (*Template, error) {
	l.log.Debugf("Loading template for provider: %s", provider)

	// Get the search paths for the provider
	paths, ok := templatePaths[strings.ToLower(provider)]
	if !ok {
		// Unknown provider, try generic paths
		paths = templatePaths["github"]
	}

	// Iterate over search paths
	for _, pathPattern := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Handle wildcard paths
		if strings.Contains(pathPattern, "*") {
			templates, err := l.loadGlobTemplates(pathPattern)
			if err != nil {
				l.log.Debugf("Failed to load templates from %s: %v", pathPattern, err)
				continue
			}
			if len(templates) > 0 {
				// Return the first found template (can be optimized later to select default or let user choose)
				return templates[0], nil
			}
		} else {
			// Single file path
			tmpl, err := l.loadSingleTemplate(pathPattern)
			if err != nil {
				l.log.Debugf("Failed to load template from %s: %v", pathPattern, err)
				continue
			}
			return tmpl, nil
		}
	}

	return nil, ErrTemplateNotFound
}

// ListTemplates lists all available templates
func (l *FileLoader) ListTemplates(ctx context.Context, provider string) ([]*Template, error) {
	l.log.Debugf("Listing templates for provider: %s", provider)

	var allTemplates []*Template

	paths, ok := templatePaths[strings.ToLower(provider)]
	if !ok {
		paths = templatePaths["github"]
	}

	for _, pathPattern := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if strings.Contains(pathPattern, "*") {
			templates, err := l.loadGlobTemplates(pathPattern)
			if err != nil {
				continue
			}
			allTemplates = append(allTemplates, templates...)
		} else {
			tmpl, err := l.loadSingleTemplate(pathPattern)
			if err != nil {
				continue
			}
			allTemplates = append(allTemplates, tmpl)
		}
	}

	// Deduplicate: deduplicate based on file path (considering case-insensitive file systems)
	seen := make(map[string]bool)
	var uniqueTemplates []*Template
	for _, tmpl := range allTemplates {
		// Convert path to lowercase to handle case-insensitive file systems
		normalizedPath := strings.ToLower(tmpl.Path)
		if !seen[normalizedPath] {
			seen[normalizedPath] = true
			uniqueTemplates = append(uniqueTemplates, tmpl)
		}
	}

	return uniqueTemplates, nil
}

// loadSingleTemplate loads a single template file
func (l *FileLoader) loadSingleTemplate(relativePath string) (*Template, error) {
	fullPath := filepath.Join(l.basePath, relativePath)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, errors.Wrapf(errors.ErrTypeConfig, "template file not found: %s", err, fullPath)
	}

	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "Failed to read template file", err)
	}

	// Create template object
	tmpl := &Template{
		Path:    fullPath,
		Name:    extractTemplateName(relativePath),
		Content: string(content),
	}

	// Infer provider from path
	tmpl.Provider = inferProviderFromPath(relativePath)

	return tmpl, nil
}

// loadGlobTemplates loads template files matching a wildcard pattern
func (l *FileLoader) loadGlobTemplates(pattern string) ([]*Template, error) {
	fullPattern := filepath.Join(l.basePath, pattern)

	// Get matching files
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to match template files", err)
	}

	var templates []*Template

	for _, match := range matches {
		// Calculate relative path
		relativePath, err := filepath.Rel(l.basePath, match)
		if err != nil {
			continue
		}

		// Read file content
		content, err := os.ReadFile(match)
		if err != nil {
			l.log.Debugf("Failed to read template %s: %v", match, err)
			continue
		}

		tmpl := &Template{
			Path:     match,
			Name:     extractTemplateName(relativePath),
			Content:  string(content),
			Provider: inferProviderFromPath(relativePath),
		}

		templates = append(templates, tmpl)
	}

	return templates, nil
}

// extractTemplateName extracts the template name from the file path
func extractTemplateName(path string) string {
	// Get file name (without extension)
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Normalize name
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	// Title case each word
	words := strings.Fields(strings.ToLower(name))
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	name = strings.Join(words, " ")

	// If it's a default template, return "Default"
	if strings.ToLower(name) == "pull request template" ||
		strings.ToLower(name) == "merge request template" ||
		strings.ToLower(name) == "pullrequest template" {
		return "Default"
	}

	return name
}

// inferProviderFromPath infers the provider type from the path
func inferProviderFromPath(path string) string {
	path = strings.ToLower(path)

	switch {
	case strings.Contains(path, ".github"):
		return "github"
	case strings.Contains(path, ".gitlab"):
		return "gitlab"
	case strings.Contains(path, ".gitea"):
		return "gitea"
	case strings.Contains(path, ".bitbucket"):
		return "bitbucket"
	default:
		// Infer from file name
		if strings.Contains(path, "merge_request") {
			return "gitlab"
		}
		if strings.Contains(path, "pullrequest") {
			return "bitbucket"
		}
		// Default to GitHub
		return "github"
	}
}

// CachedLoader is a template loader with cache
type CachedLoader struct {
	loader Loader
	cache  map[string]*Template
	log    logger.Logger
}

// NewCachedLoader creates a new cached loader
func NewCachedLoader(loader Loader) *CachedLoader {
	return &CachedLoader{
		loader: loader,
		cache:  make(map[string]*Template),
		log:    logger.NewDefault(),
	}
}

// Load loads a template (prefer cache)
func (c *CachedLoader) Load(ctx context.Context, provider string) (*Template, error) {
	// Check cache
	cacheKey := fmt.Sprintf("default:%s", provider)
	if tmpl, ok := c.cache[cacheKey]; ok {
		c.log.Debugf("Loading template from cache for provider: %s", provider)
		return tmpl, nil
	}

	// Load from underlying loader
	tmpl, err := c.loader.Load(ctx, provider)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to load template from underlying loader", err)
	}

	// Store in cache
	c.cache[cacheKey] = tmpl
	return tmpl, nil
}

// ListTemplates lists all available templates
func (c *CachedLoader) ListTemplates(ctx context.Context, provider string) ([]*Template, error) {
	// List operation does not use cache, always fetch latest
	return c.loader.ListTemplates(ctx, provider)
}

// ClearCache clears the cache
func (c *CachedLoader) ClearCache() {
	c.cache = make(map[string]*Template)
}
