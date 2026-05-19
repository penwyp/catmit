package tagging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	version, err := ParseVersion("v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "v", version.Prefix)
	assert.Equal(t, 1, version.Major)
	assert.Equal(t, 2, version.Minor)
	assert.Equal(t, 3, version.Patch)

	version, err = ParseVersion("1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "", version.Prefix)
	assert.Equal(t, "1.2.3", version.String())
}

func TestParseVersionRejectsInvalidVersions(t *testing.T) {
	invalid := []string{"", "v1.2", "v1.2.3.4", "release-1.2.3", "v01.2.3", "v1.02.3", "v1.2.03"}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			_, err := ParseVersion(value)
			assert.Error(t, err)
		})
	}
}

func TestLatestSemVerTag(t *testing.T) {
	latest, ok := LatestSemVerTag([]string{"not-semver", "v1.2.9", "v1.10.0", "v2.0.0", "v1.99.0"})
	require.True(t, ok)
	assert.Equal(t, "v2.0.0", latest.String())
}

func TestLatestSemVerTagKeepsLatestPrefix(t *testing.T) {
	latest, ok := LatestSemVerTag([]string{"1.9.0", "v2.0.0"})
	require.True(t, ok)

	next, err := latest.Next(BumpPatch)
	require.NoError(t, err)
	assert.Equal(t, "v2.0.1", next.String())
}

func TestVersionNext(t *testing.T) {
	base, err := ParseVersion("v1.2.3")
	require.NoError(t, err)

	tests := []struct {
		name string
		bump Bump
		want string
	}{
		{name: "patch", bump: BumpPatch, want: "v1.2.4"},
		{name: "minor", bump: BumpMinor, want: "v1.3.0"},
		{name: "major", bump: BumpMajor, want: "v2.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := base.Next(tt.bump)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestInferBump(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		want     Bump
	}{
		{name: "breaking footer", messages: []string{"feat: api\n\nBREAKING CHANGE: drop old field"}, want: BumpMajor},
		{name: "breaking bang", messages: []string{"fix!: change config"}, want: BumpMajor},
		{name: "feature", messages: []string{"fix: bug", "feat(ui): add release button"}, want: BumpMinor},
		{name: "patch", messages: []string{"docs: update readme", "fix: bug"}, want: BumpPatch},
		{name: "empty", messages: nil, want: BumpPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InferBump(tt.messages))
		})
	}
}
