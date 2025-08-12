package ui

import ()

// PRPreviewData contains the data required for PR preview
type PRPreviewData struct {
	Title       string
	Body        string
	Base        string
	Head        string
	Remote      string
	Provider    string
	IsDraft     bool
	HasChanges  bool
	FileChanges []FileChange

	// Template related
	UsingTemplate    bool              // Whether a template is used
	TemplateName     string            // Template name
	RawLLMResponse   string            // Original LLM response before template processing
	TemplateContent  string            // The template content used
	TemplateVars     map[string]string // Template variables and their values
	ProcessingErrors []string          // Any errors during template processing
}

// FileChange represents file change information
type FileChange struct {
	Path       string
	Additions  int
	Deletions  int
	ChangeType string // "added", "modified", "deleted"
}










