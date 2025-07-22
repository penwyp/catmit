package template

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
)

// PRTemplateManager manages PR template operations
type PRTemplateManager struct {
	templatePath string
	mu           sync.Mutex
	log          logger.Logger
}

// NewPRTemplateManager creates a new PR template manager
func NewPRTemplateManager() *PRTemplateManager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	
	templatePath := filepath.Join(home, ".config", "catmit", "pr-template.md")
	
	return &PRTemplateManager{
		templatePath: templatePath,
		log:          logger.NewDefault(),
	}
}

// EnsureTemplateExists checks if the PR template exists and creates it if not
func (m *PRTemplateManager) EnsureTemplateExists() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(m.templatePath); err == nil {
		m.log.Debugf("PR template already exists at: %s", m.templatePath)
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(m.templatePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// Write default template
	if err := os.WriteFile(m.templatePath, []byte(defaultPRTemplate), 0644); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create PR template", err)
	}

	m.log.Infof("Created default PR template at: %s", m.templatePath)
	return nil
}

// LoadTemplate loads the PR template content
func (m *PRTemplateManager) LoadTemplate(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure template exists
	if err := m.ensureTemplateExistsUnsafe(); err != nil {
		return "", err
	}

	// Read template content
	content, err := os.ReadFile(m.templatePath)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to read PR template", err)
	}

	return string(content), nil
}

// GetTemplatePath returns the path to the PR template file
func (m *PRTemplateManager) GetTemplatePath() string {
	return m.templatePath
}

// ensureTemplateExistsUnsafe is the internal version without lock
func (m *PRTemplateManager) ensureTemplateExistsUnsafe() error {
	// Check if file exists
	if _, err := os.Stat(m.templatePath); err == nil {
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(m.templatePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}

	// Write default template
	if err := os.WriteFile(m.templatePath, []byte(defaultPRTemplate), 0644); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create PR template", err)
	}

	m.log.Infof("Created default PR template at: %s", m.templatePath)
	return nil
}