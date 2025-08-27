package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		expected string
		cleanup  func()
	}{
		{
			name:     "default version value",
			setup:    func() {},
			expected: "test",
			cleanup:  func() {},
		},
		{
			name: "modified version value",
			setup: func() {
				Version = "1.2.3"
			},
			expected: "1.2.3",
			cleanup: func() {
				Version = "test"
			},
		},
		{
			name: "version with build metadata",
			setup: func() {
				Version = "v2.0.0-alpha+build123"
			},
			expected: "v2.0.0-alpha+build123",
			cleanup: func() {
				Version = "test"
			},
		},
		{
			name: "empty version string",
			setup: func() {
				Version = ""
			},
			expected: "",
			cleanup: func() {
				Version = "test"
			},
		},
		{
			name: "semantic version format",
			setup: func() {
				Version = "v1.0.0"
			},
			expected: "v1.0.0",
			cleanup: func() {
				Version = "test"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer tt.cleanup()
			
			assert.Equal(t, tt.expected, Version)
		})
	}
}

func TestVersionImmutability(t *testing.T) {
	// Save original version.
	originalVersion := Version
	defer func() {
		Version = originalVersion
	}()
	
	// Test that version can be changed (as it would be during build).
	Version = "build-version"
	assert.Equal(t, "build-version", Version)
	
	// Test that version persists across function calls.
	checkVersion := func() string {
		return Version
	}
	assert.Equal(t, "build-version", checkVersion())
}

func TestVersionConcurrency(t *testing.T) {
	// Save original version.
	originalVersion := Version
	defer func() {
		Version = originalVersion
	}()
	
	// Test concurrent reads don't cause issues.
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func() {
			_ = Version
			done <- true
		}()
	}
	
	// Wait for all goroutines to complete.
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Version should remain unchanged.
	assert.Equal(t, originalVersion, Version)
}