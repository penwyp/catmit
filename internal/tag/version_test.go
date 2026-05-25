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
		{name: "mixed conventional and free-text", messages: []string{"fix: bug", "random commit message", "feat: add login"}, want: BumpMinor},
		{name: "all free-text", messages: []string{"updated stuff", "made changes"}, want: BumpPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InferBump(tt.messages))
		})
	}
}

func TestInferBumpWithDetail(t *testing.T) {
	detail := InferBumpWithDetail([]string{"fix: bug", "feat(ui): add button"})
	assert.Equal(t, BumpMinor, detail.Bump)
	assert.Equal(t, 2, detail.TotalMessages)
	assert.Equal(t, 2, detail.ConventionalCount)
	assert.True(t, detail.HasFeature)
	assert.False(t, detail.HasBreaking)
	assert.True(t, detail.AllConventional())

	detail = InferBumpWithDetail([]string{"fix: bug", "random text", "feat!: drop api"})
	assert.Equal(t, BumpMajor, detail.Bump)
	assert.Equal(t, 3, detail.TotalMessages)
	assert.Equal(t, 2, detail.ConventionalCount)
	assert.True(t, detail.HasBreaking)
	assert.False(t, detail.AllConventional())

	detail = InferBumpWithDetail(nil)
	assert.Equal(t, BumpPatch, detail.Bump)
	assert.Equal(t, 0, detail.TotalMessages)
	assert.Equal(t, 0, detail.ConventionalCount)
	assert.False(t, detail.AllConventional())
}

func TestValidateConventionalCommit(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{name: "basic feat", message: "feat: add login", wantErr: false},
		{name: "feat with scope", message: "feat(auth): add login feature", wantErr: false},
		{name: "fix with bang", message: "fix!: drop legacy config", wantErr: false},
		{name: "feat with scope and bang", message: "feat(api)!: remove v1 endpoint", wantErr: false},
		{name: "multiline with body", message: "fix: resolve race condition\n\nUse mutex to guard shared state.", wantErr: false},
		{name: "chore", message: "chore: update dependencies", wantErr: false},
		{name: "docs", message: "docs: update README", wantErr: false},
		{name: "style", message: "style: format code", wantErr: false},
		{name: "test", message: "test: add unit tests", wantErr: false},
		{name: "perf", message: "perf: optimize query", wantErr: false},
		{name: "ci", message: "ci: add GitHub Actions", wantErr: false},
		{name: "build", message: "build: update go.mod", wantErr: false},
		{name: "revert", message: "revert: rollback feature", wantErr: false},
		{name: "empty", message: "", wantErr: true},
		{name: "no prefix", message: "just a random message", wantErr: true},
		{name: "invalid type", message: "foo: something", wantErr: true},
		{name: "no subject", message: "feat:", wantErr: true},
		{name: "no colon", message: "feat add login", wantErr: true},
		{name: "leading garbage", message: "Here is the commit:\nfeat: add login", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConventionalCommit(tt.message)
			if tt.wantErr {
				assert.Error(t, err, "expected error for message: %q", tt.message)
			} else {
				assert.NoError(t, err, "unexpected error for message: %q", tt.message)
			}
		})
	}
}

func TestTryRepairCommitMessage(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name:    "already valid",
			raw:     "feat: add login",
			want:    "feat: add login",
			wantErr: false,
		},
		{
			name:    "header on second line",
			raw:     "Here is the commit message:\nfeat: add login feature\n\nexplains why",
			want:    "feat: add login feature\n\nexplains why",
			wantErr: false,
		},
		{
			name:    "no conventional header",
			raw:     "just some random text\nno commit here",
			wantErr: true,
		},
		{
			name:    "bang syntax repair",
			raw:     "Got it!\nfeat!: drop legacy API\n\nremoves all v1 routes",
			want:    "feat!: drop legacy API\n\nremoves all v1 routes",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired, err := TryRepairCommitMessage(tt.raw)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, repaired)
			}
		})
	}
}
