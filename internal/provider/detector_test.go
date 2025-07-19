package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected RemoteInfo
		wantErr  bool
	}{
		{
			name:  "GitHub HTTPS with .git",
			input: "https://github.com/owner/repo.git",
			expected: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:  "GitHub HTTPS without .git",
			input: "https://github.com/owner/repo",
			expected: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:  "GitHub SSH",
			input: "git@github.com:owner/repo.git",
			expected: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "ssh",
			},
		},
		{
			name:  "Gitea with custom port SSH",
			input: "ssh://git@gitea.company.com:2222/owner/repo.git",
			expected: RemoteInfo{
				Provider: "unknown", // 需要HTTP探测确认
				Host:     "gitea.company.com",
				Port:     2222,
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "ssh",
			},
		},
		{
			name:  "GitLab HTTPS",
			input: "https://gitlab.com/owner/repo.git",
			expected: RemoteInfo{
				Provider: "gitlab",
				Host:     "gitlab.com",
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:  "GitLab with subgroups",
			input: "https://gitlab.com/group/subgroup/repo.git",
			expected: RemoteInfo{
				Provider: "gitlab",
				Host:     "gitlab.com",
				Owner:    "group/subgroup",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:  "Self-hosted Git HTTPS",
			input: "https://git.internal.com/team/project.git",
			expected: RemoteInfo{
				Provider: "unknown",
				Host:     "git.internal.com",
				Owner:    "team",
				Repo:     "project",
				Protocol: "https",
			},
		},
		{
			name:  "HTTPS with port",
			input: "https://git.company.com:8443/owner/repo.git",
			expected: RemoteInfo{
				Provider: "unknown",
				Host:     "git.company.com",
				Port:     8443,
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:  "SSH with non-standard format",
			input: "ssh://git@github.com/owner/repo.git",
			expected: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "ssh",
			},
		},
		{
			name:    "Empty URL",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Invalid URL",
			input:   "not-a-url",
			wantErr: true,
		},
		{
			name:    "URL without path",
			input:   "https://github.com",
			wantErr: true,
		},
		{
			name:    "URL with only one path segment",
			input:   "https://github.com/owner",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseGitRemoteURL(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectProviderFromHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "GitHub",
			host:     "github.com",
			expected: "github",
		},
		{
			name:     "GitLab",
			host:     "gitlab.com",
			expected: "gitlab",
		},
		{
			name:     "Bitbucket",
			host:     "bitbucket.org",
			expected: "bitbucket",
		},
		{
			name:     "Unknown host",
			host:     "git.company.com",
			expected: "unknown",
		},
		{
			name:     "Gitea common pattern",
			host:     "gitea.company.com",
			expected: "unknown", // 需要HTTP探测
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectProviderFromHost(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoteInfo_GetHTTPURL(t *testing.T) {
	tests := []struct {
		name     string
		remote   RemoteInfo
		expected string
	}{
		{
			name: "HTTPS without port",
			remote: RemoteInfo{
				Host:     "github.com",
				Protocol: "https",
			},
			expected: "https://github.com",
		},
		{
			name: "HTTPS with port",
			remote: RemoteInfo{
				Host:     "git.company.com",
				Port:     8443,
				Protocol: "https",
			},
			expected: "https://git.company.com:8443",
		},
		{
			name: "SSH converts to HTTPS",
			remote: RemoteInfo{
				Host:     "github.com",
				Protocol: "ssh",
			},
			expected: "https://github.com",
		},
		{
			name: "SSH with port converts to HTTPS",
			remote: RemoteInfo{
				Host:     "gitea.company.com",
				Port:     2222,
				Protocol: "ssh",
			},
			expected: "https://gitea.company.com:2222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.remote.GetHTTPURL()
			assert.Equal(t, tt.expected, result)
		})
	}
}