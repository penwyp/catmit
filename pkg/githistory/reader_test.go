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

	gitLogOutput := `abc123|abc123|feat: add new feature|This is the body|John Doe <john@example.com>|2024-01-01T10:00:00Z|John Doe <john@example.com>|2024-01-01T10:00:00Z|parent123
def456|def456|fix: bug fix|Fix description|Jane Smith <jane@example.com>|2024-01-01T11:00:00Z|Jane Smith <jane@example.com>|2024-01-01T11:00:00Z|abc123`

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
	
	// Test valid input with multiple commits
	input := `abc123|abc123|feat: feature|body text|Author <email>|2024-01-01T10:00:00Z|Committer <email>|2024-01-01T10:00:00Z|parent1 parent2
def456|def456|fix: bug||Author2 <email2>|2024-01-02T10:00:00Z|Committer2 <email2>|2024-01-02T10:00:00Z|`
	
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

func TestFormatCommitList(t *testing.T) {
	commits := []Commit{
		{ShortSHA: "abc123", Subject: "feat: add feature"},
		{ShortSHA: "def456", Subject: "fix: bug fix"},
		{ShortSHA: "ghi789", Subject: "docs: update readme"},
	}
	
	expected := `1. abc123 feat: add feature
2. def456 fix: bug fix
3. ghi789 docs: update readme`
	
	result := FormatCommitList(commits)
	assert.Equal(t, expected, result)
}