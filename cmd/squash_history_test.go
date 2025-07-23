package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSquashHistoryCommand(t *testing.T) {
	// Test that the squash-history command is properly initialized
	assert.NotNil(t, squashHistoryCmd)
	assert.Equal(t, "squash-history", squashHistoryCmd.Use)
	assert.Contains(t, squashHistoryCmd.Short, "Squash unpushed commits")
	assert.Contains(t, squashHistoryCmd.Long, "unpushed commits")
	assert.Contains(t, squashHistoryCmd.Long, "backup branch")

	// Test flags
	flags := squashHistoryCmd.Flags()
	assert.NotNil(t, flags.Lookup("yes"))
	assert.NotNil(t, flags.Lookup("lang"))
	assert.NotNil(t, flags.Lookup("timeout"))
	assert.NotNil(t, flags.Lookup("debug"))
	assert.NotNil(t, flags.Lookup("dry-run"))

	// Verify flag defaults
	langFlag := flags.Lookup("lang")
	assert.Equal(t, "en", langFlag.DefValue)

	timeoutFlag := flags.Lookup("timeout")
	assert.Equal(t, "30", timeoutFlag.DefValue)

	// Verify no append-mode flag
	assert.Nil(t, flags.Lookup("append-mode"))
	
	// Verify no rebase flag (as it's been moved to this command)
	assert.Nil(t, flags.Lookup("rebase"))
}

func TestSquashHistoryExamples(t *testing.T) {
	// Test that examples are provided
	assert.Contains(t, squashHistoryCmd.Example, "catmit squash-history")
	assert.Contains(t, squashHistoryCmd.Example, "--yes")
	assert.Contains(t, squashHistoryCmd.Example, "--dry-run")
	assert.Contains(t, squashHistoryCmd.Example, "--lang zh")
}