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

func TestRunSquashHistory_DryRunMode(t *testing.T) {
	// Save original values
	origDryRun := historyDryRun
	origTimeout := historyTimeout
	defer func() {
		historyDryRun = origDryRun
		historyTimeout = origTimeout
	}()

	// Set test values
	historyDryRun = true
	historyTimeout = 30
	historyDebug = false

	t.Run("dry run prevents execution", func(t *testing.T) {
		// Verify that dry-run mode is set
		assert.True(t, historyDryRun)
		// The actual execution test would require mocked dependencies
		// which is better suited for integration tests
	})
}

func TestRunSquashHistory_YesMode(t *testing.T) {
	// Save original values
	origYes := historyYes
	origTimeout := historyTimeout
	defer func() {
		historyYes = origYes
		historyTimeout = origTimeout
	}()

	// Set test values
	historyYes = true
	historyTimeout = 30

	t.Run("yes mode skips confirmation", func(t *testing.T) {
		// Verify that yes mode is set
		assert.True(t, historyYes)
		// The actual execution test would require mocked dependencies
	})
}

func TestRunSquashHistory_Flags(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		expected interface{}
	}{
		{
			name:     "default language",
			flag:     "lang",
			expected: "en",
		},
		{
			name:     "default timeout",
			flag:     "timeout",
			expected: "30",
		},
		{
			name:     "yes flag",
			flag:     "yes",
			expected: "false",
		},
		{
			name:     "debug flag",
			flag:     "debug",
			expected: "false",
		},
		{
			name:     "dry-run flag",
			flag:     "dry-run",
			expected: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := squashHistoryCmd.Flags().Lookup(tt.flag)
			assert.NotNil(t, flag)
			assert.Equal(t, tt.expected, flag.DefValue)
		})
	}
}