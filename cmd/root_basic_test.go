package cmd

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestGetVersionString tests the version string function
func TestGetVersionString(t *testing.T) {
	// Test with default version
	version = "dev"
	assert.Equal(t, "catmit version dev", GetVersionString())

	// Test with a specific version
	version = "1.0.0"
	assert.Equal(t, "catmit version 1.0.0", GetVersionString())
}

// TestIsPRRequested tests the PR flag detection
func TestIsPRRequested(t *testing.T) {
	tests := []struct {
		name         string
		flagPR       bool
		flagCreatePR bool
		expected     bool
	}{
		{
			name:         "neither flag set",
			flagPR:       false,
			flagCreatePR: false,
			expected:     false,
		},
		{
			name:         "only flagPR set",
			flagPR:       true,
			flagCreatePR: false,
			expected:     true,
		},
		{
			name:         "only flagCreatePR set",
			flagPR:       false,
			flagCreatePR: true,
			expected:     true,
		},
		{
			name:         "both flags set",
			flagPR:       true,
			flagCreatePR: true,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original values
			originalFlagPR := flagPR
			defer func() {
				flagPR = originalFlagPR
			}()

			flagPR = tt.flagPR

			assert.Equal(t, tt.expected, isPRRequested())
		})
	}
}
