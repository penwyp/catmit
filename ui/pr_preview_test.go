package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPRPreviewModel_View(t *testing.T) {
	tests := []struct {
		name        string
		data        PRPreviewData
		showDetails bool
		contains    []string
		notContains []string
	}{
		{
			name: "basic PR preview",
			data: PRPreviewData{
				Title:    "feat: add new feature",
				Body:     "This is a description",
				Base:     "main",
				Head:     "feature-branch",
				Remote:   "origin",
				Provider: "github",
				IsDraft:  false,
				FileChanges: []FileChange{
					{Path: "file1.go", Additions: 10, Deletions: 5, ChangeType: "modified"},
				},
			},
			contains: []string{
				"Pull Request Preview",
				"Provider:",
				"github",
				"Remote:",
				"origin",
				"From:",
				"feature-branch",
				"To:",
				"main",
				"Title:",
				"feat: add new feature",
				"Description:",
				"This is a description",
				"Changes:",
				"1 files changed",
				"file1.go",
			},
		},
		{
			name: "draft PR",
			data: PRPreviewData{
				Title:    "WIP: work in progress",
				Base:     "main",
				Head:     "wip-branch",
				Remote:   "origin",
				Provider: "github",
				IsDraft:  true,
			},
			contains: []string{
				"Status:",
				"Draft",
			},
		},
		{
			name: "PR with long description",
			data: PRPreviewData{
				Title: "feat: add feature",
				Body: strings.Join([]string{
					"Line 1",
					"Line 2",
					"Line 3",
					"Line 4",
					"Line 5",
					"Line 6",
					"Line 7",
					"Line 8",
				}, "\n"),
				Base:     "main",
				Head:     "feature",
				Remote:   "origin",
				Provider: "github",
			},
			showDetails: false,
			contains: []string{
				"Line 1",
				"Line 2",
				"Line 3",
				"Line 4",
				"Line 5",
				"... 3 more lines ...",
				"[D] Show details",
			},
			notContains: []string{
				"Line 6",
				"Line 7",
				"Line 8",
			},
		},
		{
			name: "PR with long description - details shown",
			data: PRPreviewData{
				Title: "feat: add feature",
				Body: strings.Join([]string{
					"Line 1",
					"Line 2",
					"Line 3",
					"Line 4",
					"Line 5",
					"Line 6",
					"Line 7",
					"Line 8",
				}, "\n"),
				Base:     "main",
				Head:     "feature",
				Remote:   "origin",
				Provider: "github",
			},
			showDetails: true,
			contains: []string{
				"Line 1",
				"Line 2",
				"Line 3",
				"Line 4",
				"Line 5",
				"Line 6",
				"Line 7",
				"Line 8",
				"[D] Hide details",
			},
		},
		{
			name: "PR with many file changes",
			data: PRPreviewData{
				Title:    "refactor: major refactoring",
				Base:     "main",
				Head:     "refactor",
				Remote:   "origin",
				Provider: "gitlab",
				FileChanges: []FileChange{
					{Path: "file1.go", Additions: 10, Deletions: 5, ChangeType: "modified"},
					{Path: "file2.go", Additions: 20, Deletions: 0, ChangeType: "added"},
					{Path: "file3.go", Additions: 0, Deletions: 15, ChangeType: "deleted"},
					{Path: "file4.go", Additions: 5, Deletions: 5, ChangeType: "modified"},
					{Path: "file5.go", Additions: 3, Deletions: 2, ChangeType: "modified"},
				},
			},
			showDetails: false,
			contains: []string{
				"5 files changed",
				"+38",
				"-27",
				"file1.go",
				"file2.go",
				"file3.go",
				"... 2 more files ...",
			},
			notContains: []string{
				"file4.go",
				"file5.go",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewPRPreviewModel(tt.data, DefaultStyles(), 80)
			if tt.showDetails {
				model.ToggleDetails()
			}

			view := model.View()

			for _, expected := range tt.contains {
				assert.Contains(t, view, expected, "View should contain %q", expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, view, notExpected, "View should not contain %q", notExpected)
			}
		})
	}
}

func TestPRPreviewModel_ToggleDetails(t *testing.T) {
	data := PRPreviewData{
		Title: "test",
		Body: strings.Join([]string{
			"Line 1", "Line 2", "Line 3", "Line 4", "Line 5",
			"Line 6", "Line 7", "Line 8",
		}, "\n"),
		Base:     "main",
		Head:     "test",
		Remote:   "origin",
		Provider: "github",
	}

	model := NewPRPreviewModel(data, DefaultStyles(), 80)

	// Initially, details should be hidden
	view1 := model.View()
	assert.Contains(t, view1, "... 3 more lines ...")
	assert.NotContains(t, view1, "Line 8")

	// Toggle to show details
	model.ToggleDetails()
	view2 := model.View()
	assert.NotContains(t, view2, "... 3 more lines ...")
	assert.Contains(t, view2, "Line 8")

	// Toggle back to hide details
	model.ToggleDetails()
	view3 := model.View()
	assert.Contains(t, view3, "... 3 more lines ...")
	assert.NotContains(t, view3, "Line 8")
}

func TestGetChangeIcon(t *testing.T) {
	model := &PRPreviewModel{styles: DefaultStyles()}

	tests := []struct {
		changeType string
		expected   string
	}{
		{"added", "+"},
		{"deleted", "-"},
		{"modified", "●"},
		{"unknown", "○"},
		{"", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.changeType, func(t *testing.T) {
			icon := model.getChangeIcon(tt.changeType)
			assert.Equal(t, tt.expected, icon)
		})
	}
}