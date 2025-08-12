package githistory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		branch         string
		backupExists   bool
		createError    error
		expectedBackup string
		expectError    bool
	}{
		{
			name:           "successful backup creation",
			branch:         "feature",
			backupExists:   false,
			createError:    nil,
			expectedBackup: "feature_bak",
			expectError:    false,
		},
		{
			name:           "backup already exists",
			branch:         "feature",
			backupExists:   true,
			createError:    nil,
			expectedBackup: "feature_bak_",
			expectError:    false,
		},
		{
			name:           "branch creation fails",
			branch:         "feature",
			backupExists:   false,
			createError:    errors.New("permission denied"),
			expectedBackup: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputs := []string{}
			errs := []error{}
			
			if tt.backupExists {
				// First call checks if backup exists (success means it exists)
				outputs = append(outputs, "ref exists")
				errs = append(errs, nil)
			} else {
				// First call checks if backup exists (error means it doesn't)
				outputs = append(outputs, "")
				errs = append(errs, errors.New("not found"))
			}
			
			// Second call creates the branch
			outputs = append(outputs, "")
			errs = append(errs, tt.createError)
			
			mr := &mockRunner{
				outputs: outputs,
				errors:  errs,
			}
			
			modifier := NewModifier(mr)
			backupName, err := modifier.BackupBranch(context.Background(), tt.branch)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.backupExists {
					// When backup exists, timestamp is added
					assert.Contains(t, backupName, tt.expectedBackup)
				} else {
					assert.Equal(t, tt.expectedBackup, backupName)
				}
			}
		})
	}
}

func TestRebaseInteractive(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{SHA: "abc123", ShortSHA: "abc123", Subject: "feat: first commit"},
		{SHA: "def456", ShortSHA: "def456", Subject: "fix: second commit"},
		{SHA: "ghi789", ShortSHA: "ghi789", Subject: "docs: third commit"},
	}

	tests := []struct {
		name        string
		base        string
		commits     []Commit
		newMessage  string
		outputs     []string
		errors      []error
		expectError bool
	}{
		{
			name:       "successful rebase",
			base:       "origin/main",
			commits:    commits,
			newMessage: "feat: consolidated commit message",
			outputs:    []string{"current-sha\n", "", ""},
			errors:     []error{nil, nil, nil},
			expectError: false,
		},
		{
			name:       "no commits to rebase",
			base:       "origin/main",
			commits:    []Commit{},
			newMessage: "feat: message",
			outputs:    []string{},
			errors:     []error{},
			expectError: true,
		},
		{
			name:       "reset fails",
			base:       "origin/main",
			commits:    commits,
			newMessage: "feat: message",
			outputs:    []string{"current-sha\n", ""},
			errors:     []error{nil, errors.New("reset failed")},
			expectError: true,
		},
		{
			name:       "commit fails with recovery",
			base:       "origin/main",
			commits:    commits,
			newMessage: "feat: message",
			outputs:    []string{"current-sha\n", "", "", ""},
			errors:     []error{nil, nil, errors.New("commit failed"), nil},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := &mockRunner{
				outputs: tt.outputs,
				errors:  tt.errors,
			}
			
			modifier := NewModifier(mr)
			err := modifier.RebaseInteractive(context.Background(), tt.base, tt.commits, tt.newMessage)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Verify the sequence of git commands
				require.GreaterOrEqual(t, len(mr.calls), 3)
				
				// Should get current HEAD
				assert.Equal(t, []string{"git", "rev-parse", "HEAD"}, mr.calls[0])
				
				// Should reset soft to base
				assert.Equal(t, []string{"git", "reset", "--soft", tt.base}, mr.calls[1])
				
				// Should commit with new message
				assert.Equal(t, []string{"git", "commit", "-m", tt.newMessage}, mr.calls[2])
			}
		})
	}
}

func TestResetHard(t *testing.T) {
	t.Parallel()

	mr := &mockRunner{
		outputs: []string{""},
		errors:  []error{nil},
	}
	
	modifier := NewModifier(mr)
	err := modifier.ResetHard(context.Background(), "abc123")
	
	assert.NoError(t, err)
	require.Len(t, mr.calls, 1)
	assert.Equal(t, []string{"git", "reset", "--hard", "abc123"}, mr.calls[0])
}

func TestAbortRebase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		err         error
		expectError bool
	}{
		{
			name:        "successful abort",
			output:      "",
			err:         nil,
			expectError: false,
		},
		{
			name:        "no rebase in progress",
			output:      "",
			err:         errors.New("No rebase in progress"),
			expectError: false, // This is not an error condition
		},
		{
			name:        "other error",
			output:      "",
			err:         errors.New("fatal: could not read log"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := &mockRunner{
				outputs: []string{tt.output},
				errors:  []error{tt.err},
			}
			
			modifier := NewModifier(mr)
			err := modifier.AbortRebase(context.Background())
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			require.Len(t, mr.calls, 1)
			assert.Equal(t, []string{"git", "rebase", "--abort"}, mr.calls[0])
		})
	}
}

func TestCherryPick(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		commits     []string
		err         error
		expectError bool
	}{
		{
			name:        "successful cherry-pick single commit",
			commits:     []string{"abc123"},
			err:         nil,
			expectError: false,
		},
		{
			name:        "successful cherry-pick multiple commits",
			commits:     []string{"abc123", "def456", "ghi789"},
			err:         nil,
			expectError: false,
		},
		{
			name:        "no commits provided",
			commits:     []string{},
			err:         nil,
			expectError: true,
		},
		{
			name:        "cherry-pick fails",
			commits:     []string{"abc123"},
			err:         errors.New("conflict"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := &mockRunner{
				outputs: []string{""},
				errors:  []error{tt.err},
			}
			
			modifier := NewModifier(mr)
			err := modifier.CherryPick(context.Background(), tt.commits)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.Len(t, mr.calls, 1)
				expectedCall := append([]string{"git", "cherry-pick"}, tt.commits...)
				assert.Equal(t, expectedCall, mr.calls[0])
			}
		})
	}
}