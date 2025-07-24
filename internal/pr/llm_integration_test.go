package pr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLLMClient is a mock implementation of llm.Interface
type MockLLMClient struct {
	mock.Mock
}

func (m *MockLLMClient) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := m.Called(ctx, systemPrompt, userPrompt)
	return args.String(0), args.Error(1)
}

func TestLLMGenerator_GeneratePRTitle(t *testing.T) {
	t.Run("GenerateFromCommits", func(t *testing.T) {
		mockLLM := new(MockLLMClient)
		generator := NewLLMGenerator(mockLLM)
		
		data := &PRAnalysisData{
			BranchName: "feature/user-auth",
			Commits: []CommitInfo{
				{SHA: "abc123", Message: "feat: add user registration endpoint"},
				{SHA: "def456", Message: "feat: add email validation"},
				{SHA: "ghi789", Message: "test: add unit tests for registration"},
			},
			DiffStats: "3 files changed, 150 insertions(+), 10 deletions(-)",
		}
		
		// Mock LLM response
		mockLLM.On("GetCommitMessage", mock.Anything, "", mock.Anything).Return("[Feature] Add user authentication system", nil)
		
		ctx := context.Background()
		title, err := generator.GeneratePRTitle(ctx, data)
		
		require.NoError(t, err)
		assert.Equal(t, "[Feature] Add user authentication system", title)
		
		// Verify the prompt included commit information
		mockLLM.AssertCalled(t, "GetCommitMessage", ctx, "", mock.MatchedBy(func(prompt string) bool {
			return assert.Contains(t, prompt, "feat: add user registration endpoint") &&
				assert.Contains(t, prompt, "3 files changed")
		}))
	})
	
	t.Run("GenerateFromBranchName", func(t *testing.T) {
		mockLLM := new(MockLLMClient)
		generator := NewLLMGenerator(mockLLM)
		
		data := &PRAnalysisData{
			BranchName: "fix/email-validation-bug",
			Commits:    []CommitInfo{}, // No commits
		}
		
		// Mock LLM response
		mockLLM.On("GetCommitMessage", mock.Anything, "", mock.Anything).Return("[Fix] Correct email validation logic", nil)
		
		ctx := context.Background()
		title, err := generator.GeneratePRTitle(ctx, data)
		
		require.NoError(t, err)
		assert.Equal(t, "[Fix] Correct email validation logic", title)
		
		// Verify the prompt included branch name
		mockLLM.AssertCalled(t, "GetCommitMessage", ctx, "", mock.MatchedBy(func(prompt string) bool {
			return assert.Contains(t, prompt, "fix/email-validation-bug")
		}))
	})
	
	t.Run("CleanTitle_AddsType", func(t *testing.T) {
		mockLLM := new(MockLLMClient)
		generator := NewLLMGenerator(mockLLM)
		
		data := &PRAnalysisData{
			BranchName: "update-readme",
		}
		
		// Mock LLM response without type prefix
		mockLLM.On("GetCommitMessage", mock.Anything, "", mock.Anything).Return("Update README with new instructions", nil)
		
		ctx := context.Background()
		title, err := generator.GeneratePRTitle(ctx, data)
		
		require.NoError(t, err)
		assert.Equal(t, "[Docs] Update README with new instructions", title)
	})
}

func TestLLMGenerator_GeneratePRBody(t *testing.T) {
	t.Run("FillTemplate", func(t *testing.T) {
		mockLLM := new(MockLLMClient)
		generator := NewLLMGenerator(mockLLM)
		
		template := `### 📝 Change Type
- [ ] Feature
- [ ] Bug Fix

### 📌 Summary
<!-- Description here -->`
		
		data := &PRAnalysisData{
			BranchName: "feature/user-auth",
			Commits: []CommitInfo{
				{Message: "feat: add user registration"},
			},
			ChangedFiles: []string{"auth/register.go", "auth/register_test.go"},
			HasTests:     true,
		}
		
		expectedBody := `### 📝 Change Type
- [x] Feature
- [ ] Bug Fix

### 📌 Summary
Add user registration functionality with comprehensive tests`
		
		// Mock LLM response
		mockLLM.On("GetCommitMessage", mock.Anything, "", mock.Anything).Return(expectedBody, nil)
		
		ctx := context.Background()
		body, err := generator.GeneratePRBody(ctx, template, data)
		
		require.NoError(t, err)
		assert.Contains(t, body, "[x] Feature")
		assert.Contains(t, body, "Add user registration")
		
		// Verify the prompt included necessary information
		mockLLM.AssertCalled(t, "GetCommitMessage", ctx, "", mock.MatchedBy(func(prompt string) bool {
			return assert.Contains(t, prompt, "auth/register.go") &&
				assert.Contains(t, prompt, "Test files were modified")
		}))
	})
	
	t.Run("CleanBody_RemovesWrapper", func(t *testing.T) {
		mockLLM := new(MockLLMClient)
		generator := NewLLMGenerator(mockLLM)
		
		template := "### Summary"
		data := &PRAnalysisData{BranchName: "test"}
		
		// Mock LLM response with wrapper text
		mockLLM.On("GetCommitMessage", mock.Anything, "", mock.Anything).Return(`Here is the filled template:

### Summary
This is the actual content

Based on the changes...`, nil)
		
		ctx := context.Background()
		body, err := generator.GeneratePRBody(ctx, template, data)
		
		require.NoError(t, err)
		assert.Equal(t, "### Summary\nThis is the actual content", body)
	})
}