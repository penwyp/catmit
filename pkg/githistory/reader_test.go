package githistory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner implements Runner interface for testing
type mockRunner struct {
	calls    [][]string
	outputs  []string
	errors   []error
	callIdx  int
}

func (m *mockRunner) Run(ctx context.Context, command string, args ...string) (string, error) {
	m.calls = append(m.calls, append([]string{command}, args...))
	
	if m.callIdx >= len(m.outputs) {
		return "", errors.New("unexpected call")
	}
	
	output := m.outputs[m.callIdx]
	err := m.errors[m.callIdx]
	m.callIdx++
	
	return output, err
}

func TestFindMergeBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ref1        string
		ref2        string
		output      string
		err         error
		expectedSHA string
		expectError bool
	}{
		{
			name:        "successful merge base",
			ref1:        "main",
			ref2:        "feature",
			output:      "abc123def456\n",
			err:         nil,
			expectedSHA: "abc123def456",
			expectError: false,
		},
		{
			name:        "git command fails",
			ref1:        "main",
			ref2:        "feature",
			output:      "",
			err:         errors.New("fatal: Not a valid object name"),
			expectedSHA: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := &mockRunner{
				outputs: []string{tt.output},
				errors:  []error{tt.err},
			}
			
			reader := NewReader(mr)
			sha, err := reader.FindMergeBase(context.Background(), tt.ref1, tt.ref2)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSHA, sha)
			}
			
			// Verify the correct git command was called
			require.Len(t, mr.calls, 1)
			assert.Equal(t, []string{"git", "merge-base", tt.ref1, tt.ref2}, mr.calls[0])
		})
	}
}

func TestGetUnpushedCommits(t *testing.T) {
	t.Parallel()

	// Using ASCII control characters as separators
	// \x1f for field separator, \x1e for record separator
	gitLogOutput := "abc123\x1fabc123\x1ffeat: add new feature\x1fThis is the body\x1fJohn Doe <john@example.com>\x1f2024-01-01T10:00:00Z\x1fJohn Doe <john@example.com>\x1f2024-01-01T10:00:00Z\x1fparent123\x1e" +
		"def456\x1fdef456\x1ffix: bug fix\x1fFix description\x1fJane Smith <jane@example.com>\x1f2024-01-01T11:00:00Z\x1fJane Smith <jane@example.com>\x1f2024-01-01T11:00:00Z\x1fabc123\x1e"

	mr := &mockRunner{
		outputs: []string{gitLogOutput},
		errors:  []error{nil},
	}
	
	reader := NewReader(mr)
	commits, err := reader.GetUnpushedCommits(context.Background(), "origin/main", "HEAD")
	
	assert.NoError(t, err)
	assert.Len(t, commits, 2)
	
	// Check first commit
	assert.Equal(t, "abc123", commits[0].SHA)
	assert.Equal(t, "abc123", commits[0].ShortSHA)
	assert.Equal(t, "feat: add new feature", commits[0].Subject)
	assert.Equal(t, "This is the body", commits[0].Body)
	assert.Equal(t, "John Doe <john@example.com>", commits[0].Author)
	assert.Equal(t, []string{"parent123"}, commits[0].ParentSHAs)
	
	// Check second commit
	assert.Equal(t, "def456", commits[1].SHA)
	assert.Equal(t, "fix: bug fix", commits[1].Subject)
}

func TestGetUnpushedCommitsWithMultilineBody(t *testing.T) {
	t.Parallel()

	// Test with multi-line commit body (similar to the real-world example)
	gitLogOutput := "c5a3b7e2\x1fc5a3b7e2\x1ffeat(makefile): enhance lint task\x1fAdd support for running lint on specific packages via PKG variable.\nIntroduce a new lint-stage target to lint staged and unstaged files,\nimproving flexibility and efficiency in code quality checks.\x1fpenwyp <wu.pen@example.com>\x1f2025-01-08T21:55:14+08:00\x1fpenwyp <wu.pen@example.com>\x1f2025-01-08T21:55:14+08:00\x1feadb5a3d\x1e" +
		"eadb5a3d\x1feadb5a3d\x1ffeat(objectparser): implement unified prompt strategy\x1frefactor(conversion): consolidate prompt management into ConversionPromptStrategy\nfeat(trigger): add DetermineTriggerPromptNeeds method to PromptSelector\nrefactor(prompts): simplify ConversionPreparation struct by removing redundant fields\x1fpenwyp <wu.pen@example.com>\x1f2025-01-08T17:53:22+08:00\x1fpenwyp <wu.pen@example.com>\x1f2025-01-08T18:11:50+08:00\x1fcbcbb1ef\x1e"

	mr := &mockRunner{
		outputs: []string{gitLogOutput},
		errors:  []error{nil},
	}
	
	reader := NewReader(mr)
	commits, err := reader.GetUnpushedCommits(context.Background(), "main", "HEAD")
	
	assert.NoError(t, err)
	assert.Len(t, commits, 2)
	
	// Check first commit with multi-line body
	assert.Equal(t, "c5a3b7e2", commits[0].SHA)
	assert.Equal(t, "feat(makefile): enhance lint task", commits[0].Subject)
	assert.Contains(t, commits[0].Body, "Add support for running lint on specific packages")
	assert.Contains(t, commits[0].Body, "Introduce a new lint-stage target")
	assert.Contains(t, commits[0].Body, "improving flexibility and efficiency")
	
	// Check second commit with multi-line body
	assert.Equal(t, "eadb5a3d", commits[1].SHA)
	assert.Equal(t, "feat(objectparser): implement unified prompt strategy", commits[1].Subject)
	assert.Contains(t, commits[1].Body, "refactor(conversion)")
	assert.Contains(t, commits[1].Body, "feat(trigger)")
	assert.Contains(t, commits[1].Body, "refactor(prompts)")
}

func TestHasUncommittedChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		hasChanges  bool
	}{
		{
			name:       "no changes",
			output:     "",
			hasChanges: false,
		},
		{
			name:       "has staged changes",
			output:     "M  file.txt\n",
			hasChanges: true,
		},
		{
			name:       "has unstaged changes",
			output:     " M file.txt\n",
			hasChanges: true,
		},
		{
			name:       "has untracked files",
			output:     "?? newfile.txt\n",
			hasChanges: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := &mockRunner{
				outputs: []string{tt.output},
				errors:  []error{nil},
			}
			
			reader := NewReader(mr)
			hasChanges, err := reader.HasUncommittedChanges(context.Background())
			
			assert.NoError(t, err)
			assert.Equal(t, tt.hasChanges, hasChanges)
		})
	}
}

func TestGetCurrentBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		output       string
		err          error
		expectedBranch string
		expectError  bool
	}{
		{
			name:         "on branch main",
			output:       "main\n",
			err:          nil,
			expectedBranch: "main",
			expectError:  false,
		},
		{
			name:         "on feature branch",
			output:       "feature/add-rebase\n",
			err:          nil,
			expectedBranch: "feature/add-rebase",
			expectError:  false,
		},
		{
			name:         "detached HEAD",
			output:       "HEAD\n",
			err:          nil,
			expectedBranch: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := &mockRunner{
				outputs: []string{tt.output},
				errors:  []error{tt.err},
			}
			
			reader := NewReader(mr)
			branch, err := reader.GetCurrentBranch(context.Background())
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBranch, branch)
			}
		})
	}
}

func TestParseCommits(t *testing.T) {
	t.Parallel()

	r := &reader{}
	
	// Test empty input
	commits, err := r.parseCommits("")
	assert.NoError(t, err)
	assert.Empty(t, commits)
	
	// Test malformed input
	commits, err = r.parseCommits("invalid|line|not|enough|parts")
	assert.NoError(t, err)
	assert.Empty(t, commits)
	
	// Test valid input with multiple commits using correct separators
	input := "abc123\x1fabc123\x1ffeat: feature\x1fbody text\x1fAuthor <email>\x1f2024-01-01T10:00:00Z\x1fCommitter <email>\x1f2024-01-01T10:00:00Z\x1fparent1 parent2\x1e" +
		"def456\x1fdef456\x1ffix: bug\x1f\x1fAuthor2 <email2>\x1f2024-01-02T10:00:00Z\x1fCommitter2 <email2>\x1f2024-01-02T10:00:00Z\x1f"
	
	commits, err = r.parseCommits(input)
	assert.NoError(t, err)
	assert.Len(t, commits, 2)
	
	// Verify first commit
	assert.Equal(t, "abc123", commits[0].SHA)
	assert.Equal(t, "feat: feature", commits[0].Subject)
	assert.Equal(t, "body text", commits[0].Body)
	assert.Equal(t, []string{"parent1", "parent2"}, commits[0].ParentSHAs)
	
	// Verify second commit (no parents)
	assert.Equal(t, "def456", commits[1].SHA)
	assert.Empty(t, commits[1].Body)
	assert.Empty(t, commits[1].ParentSHAs)
}

