package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagCommandFlags(t *testing.T) {
	flags := tagCmd.Flags()

	for _, name := range []string{
		"debug",
		"dry-run",
		"yes",
		"lang",
		"timeout",
		"remote",
		"bump",
		"tag",
		"initial",
		"stage-all",
		"seed",
	} {
		assert.NotNil(t, flags.Lookup(name), "expected --%s flag", name)
	}

	yesFlag := flags.Lookup("yes")
	require.NotNil(t, yesFlag)
	assert.Equal(t, "y", yesFlag.Shorthand)

	remoteFlag := flags.Lookup("remote")
	require.NotNil(t, remoteFlag)
	assert.Equal(t, "r", remoteFlag.Shorthand)
}

func TestTagCommandUsage(t *testing.T) {
	assert.Equal(t, "tag [SEED_TEXT]", tagCmd.Use)
	assert.Contains(t, tagCmd.Short, "semantic version tag")
	assert.Contains(t, tagCmd.Example, "catmit tag --yes")
	assert.Contains(t, tagCmd.Example, "catmit tag --dry-run")
}
