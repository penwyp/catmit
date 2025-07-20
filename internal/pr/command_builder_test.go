package pr

import (
	"testing"
	
	"github.com/stretchr/testify/assert"
)

func TestParseGiteaErrorForPRInfo(t *testing.T) {
	builder := NewCommandBuilder()
	
	tests := []struct {
		name        string
		errorOutput string
		remoteHost  string
		owner       string
		repo        string
		expectedURL string
		expectError bool
	}{
		{
			name:        "extract issue_id from tea error",
			errorOutput: "Error: could not create PR from feat-op to john.doe:master: pull request already exists for these targets [id: 842, issue_id: 80, head_repo_id: 193, base_repo_id: 193, head_branch: feat-op, base_branch: master]",
			remoteHost:  "git.example.com",
			owner:       "john.doe",
			repo:        "app-frontend",
			expectedURL: "https://git.example.com/john.doe/app-frontend/pulls/80",
			expectError: false,
		},
		{
			name:        "no issue_id in error",
			errorOutput: "Error: could not create PR from feat-op to john.doe:master: pull request already exists",
			remoteHost:  "git.example.com",
			owner:       "john.doe",
			repo:        "app-frontend",
			expectedURL: "",
			expectError: true,
		},
		{
			name:        "extract with spaces around issue_id",
			errorOutput: "Error: pull request already exists [issue_id:   42, other fields]",
			remoteHost:  "example.com",
			owner:       "user",
			repo:        "project",
			expectedURL: "https://example.com/user/project/pulls/42",
			expectError: false,
		},
	}
	
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, err := builder.ParseGiteaErrorForPRInfo(tc.errorOutput, tc.remoteHost, tc.owner, tc.repo)
			
			if tc.expectError {
				assert.Error(t, err)
				assert.Empty(t, url)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedURL, url)
			}
		})
	}
}