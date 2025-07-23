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
		name     string
		flagPR   bool
		expected bool
	}{
		{
			name:     "flag not set",
			flagPR:   false,
			expected: false,
		},
		{
			name:     "flag set",
			flagPR:   true,
			expected: true,
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
