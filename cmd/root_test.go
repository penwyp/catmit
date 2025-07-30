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