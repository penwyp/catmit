package template

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRTemplateManager(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)
	
	t.Run("NewPRTemplateManager", func(t *testing.T) {
		manager := NewPRTemplateManager()
		assert.NotNil(t, manager)
		expectedPath := filepath.Join(tmpDir, ".config", "catmit", "pr-template.md")
		assert.Equal(t, expectedPath, manager.GetTemplatePath())
	})
	
	t.Run("EnsureTemplateExists_CreatesNewTemplate", func(t *testing.T) {
		manager := NewPRTemplateManager()
		
		// Ensure template doesn't exist initially
		_, err := os.Stat(manager.GetTemplatePath())
		assert.True(t, os.IsNotExist(err))
		
		// Call EnsureTemplateExists
		err = manager.EnsureTemplateExists()
		require.NoError(t, err)
		
		// Check that template was created
		info, err := os.Stat(manager.GetTemplatePath())
		require.NoError(t, err)
		assert.False(t, info.IsDir())
		
		// Read and verify content
		content, err := os.ReadFile(manager.GetTemplatePath())
		require.NoError(t, err)
		assert.Contains(t, string(content), "### 📝 Change Type")
		assert.Contains(t, string(content), "### 📌 Summary")
		assert.Contains(t, string(content), "### 🎯 Motivation")
	})
	
	t.Run("EnsureTemplateExists_DoesNotOverwrite", func(t *testing.T) {
		manager := NewPRTemplateManager()
		
		// Create directory
		dir := filepath.Dir(manager.GetTemplatePath())
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		
		// Write custom content
		customContent := "# Custom Template\nThis is custom content."
		err = os.WriteFile(manager.GetTemplatePath(), []byte(customContent), 0644)
		require.NoError(t, err)
		
		// Call EnsureTemplateExists
		err = manager.EnsureTemplateExists()
		require.NoError(t, err)
		
		// Verify content wasn't overwritten
		content, err := os.ReadFile(manager.GetTemplatePath())
		require.NoError(t, err)
		assert.Equal(t, customContent, string(content))
	})
	
	t.Run("LoadTemplate", func(t *testing.T) {
		// Create a fresh directory for this test
		tmpDir3 := t.TempDir()
		os.Setenv("HOME", tmpDir3)
		defer os.Setenv("HOME", tmpDir)
		
		manager := NewPRTemplateManager()
		
		// Ensure template exists
		err := manager.EnsureTemplateExists()
		require.NoError(t, err)
		
		// Load template
		ctx := context.Background()
		content, err := manager.LoadTemplate(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		assert.Contains(t, content, "### 📝 Change Type")
	})
	
	t.Run("LoadTemplate_CreatesIfNotExists", func(t *testing.T) {
		// Create new manager with fresh directory
		tmpDir2 := t.TempDir()
		os.Setenv("HOME", tmpDir2)
		defer os.Setenv("HOME", tmpDir)
		
		manager := NewPRTemplateManager()
		
		// Load template (should create it)
		ctx := context.Background()
		content, err := manager.LoadTemplate(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		
		// Verify file was created
		_, err = os.Stat(manager.GetTemplatePath())
		require.NoError(t, err)
	})
	
	t.Run("ConcurrentAccess", func(t *testing.T) {
		manager := NewPRTemplateManager()
		ctx := context.Background()
		
		// Run multiple goroutines accessing the template
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				err := manager.EnsureTemplateExists()
				assert.NoError(t, err)
				
				content, err := manager.LoadTemplate(ctx)
				assert.NoError(t, err)
				assert.NotEmpty(t, content)
				
				done <- true
			}()
		}
		
		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}