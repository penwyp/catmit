package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		version1 string
		version2 string
		expected int // -1: v1 < v2, 0: v1 == v2, 1: v1 > v2
	}{
		// 标准版本比较
		{
			name:     "Equal versions",
			version1: "2.0.0",
			version2: "2.0.0",
			expected: 0,
		},
		{
			name:     "Major version difference",
			version1: "2.0.0",
			version2: "1.0.0",
			expected: 1,
		},
		{
			name:     "Minor version difference",
			version1: "2.1.0",
			version2: "2.0.0",
			expected: 1,
		},
		{
			name:     "Patch version difference",
			version1: "2.0.1",
			version2: "2.0.0",
			expected: 1,
		},
		{
			name:     "Complex comparison",
			version1: "1.9.9",
			version2: "2.0.0",
			expected: -1,
		},
		// 预发布版本
		{
			name:     "Pre-release vs stable",
			version1: "2.0.0-beta.1",
			version2: "2.0.0",
			expected: -1,
		},
		{
			name:     "Pre-release comparison",
			version1: "2.0.0-beta.2",
			version2: "2.0.0-beta.1",
			expected: 1,
		},
		{
			name:     "Alpha vs Beta",
			version1: "2.0.0-alpha.1",
			version2: "2.0.0-beta.1",
			expected: -1,
		},
		// 构建元数据
		{
			name:     "Build metadata ignored",
			version1: "2.0.0+build123",
			version2: "2.0.0+build456",
			expected: 0,
		},
		{
			name:     "Build metadata with pre-release",
			version1: "2.0.0-beta.1+build123",
			version2: "2.0.0-beta.1+build456",
			expected: 0,
		},
		// 带v前缀
		{
			name:     "With v prefix",
			version1: "v2.0.0",
			version2: "v1.9.9",
			expected: 1,
		},
		{
			name:     "Mixed v prefix",
			version1: "v2.0.0",
			version2: "2.0.0",
			expected: 0,
		},
		// 边界情况
		{
			name:     "Missing patch version",
			version1: "2.0",
			version2: "2.0.0",
			expected: 0,
		},
		{
			name:     "Missing minor and patch",
			version1: "2",
			version2: "2.0.0",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.version1, tt.version2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  Version
		expectErr bool
	}{
		{
			name:  "Standard version",
			input: "2.4.1",
			expected: Version{
				Major: 2,
				Minor: 4,
				Patch: 1,
			},
		},
		{
			name:  "Version with v prefix",
			input: "v1.2.3",
			expected: Version{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
		},
		{
			name:  "Version with pre-release",
			input: "2.0.0-beta.1",
			expected: Version{
				Major:      2,
				Minor:      0,
				Patch:      0,
				PreRelease: "beta.1",
			},
		},
		{
			name:  "Version with build metadata",
			input: "2.0.0+build123",
			expected: Version{
				Major: 2,
				Minor: 0,
				Patch: 0,
				Build: "build123",
			},
		},
		{
			name:  "Complex version",
			input: "v2.0.0-rc.1+build.123",
			expected: Version{
				Major:      2,
				Minor:      0,
				Patch:      0,
				PreRelease: "rc.1",
				Build:      "build.123",
			},
		},
		{
			name:  "Missing patch",
			input: "2.1",
			expected: Version{
				Major: 2,
				Minor: 1,
				Patch: 0,
			},
		},
		{
			name:  "Major only",
			input: "3",
			expected: Version{
				Major: 3,
				Minor: 0,
				Patch: 0,
			},
		},
		{
			name:      "Invalid format",
			input:     "invalid",
			expectErr: true,
		},
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseVersion(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCheckMinVersion(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		minimum    string
		expectMeet bool
		expectErr  bool
	}{
		{
			name:       "Meets exact version",
			current:    "2.0.0",
			minimum:    "2.0.0",
			expectMeet: true,
		},
		{
			name:       "Exceeds minimum",
			current:    "2.1.0",
			minimum:    "2.0.0",
			expectMeet: true,
		},
		{
			name:       "Below minimum",
			current:    "1.9.9",
			minimum:    "2.0.0",
			expectMeet: false,
		},
		{
			name:       "Pre-release current",
			current:    "2.0.0-beta.1",
			minimum:    "2.0.0",
			expectMeet: false,
		},
		{
			name:       "Pre-release minimum",
			current:    "2.0.0",
			minimum:    "2.0.0-beta.1",
			expectMeet: true,
		},
		{
			name:      "Invalid current version",
			current:   "invalid",
			minimum:   "2.0.0",
			expectErr: true,
		},
		{
			name:      "Invalid minimum version",
			current:   "2.0.0",
			minimum:   "invalid",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meets, err := CheckMinVersion(tt.current, tt.minimum)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectMeet, meets)
			}
		})
	}
}