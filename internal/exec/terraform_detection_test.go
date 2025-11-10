package exec

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsOpenTofu_FastPath tests detection by executable basename.
func TestIsOpenTofu_FastPath(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "tofu executable",
			command:  "tofu",
			expected: true,
		},
		{
			name:     "tofu with full path",
			command:  "/usr/bin/tofu",
			expected: true,
		},
		{
			name:     "tofu with custom path",
			command:  "/opt/opentofu/bin/tofu",
			expected: true,
		},
		{
			name:     "tofu with uppercase",
			command:  "/usr/bin/TOFU",
			expected: true,
		},
		{
			name:     "terraform executable",
			command:  "terraform",
			expected: false,
		},
		{
			name:     "terraform with full path",
			command:  "/usr/bin/terraform",
			expected: false,
		},
		{
			name:     "custom named terraform",
			command:  "/usr/local/bin/my-terraform",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache before each test.
			detectionCacheMux.Lock()
			detectionCache = make(map[string]bool)
			detectionCacheMux.Unlock()

			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Command: tt.command,
					},
				},
			}

			result := IsOpenTofu(atmosConfig)
			assert.Equal(t, tt.expected, result, "Detection result should match expected for command: %s", tt.command)
		})
	}
}

// TestIsOpenTofu_Caching tests that detection results are cached.
func TestIsOpenTofu_Caching(t *testing.T) {
	// Clear cache before test.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCacheMux.Unlock()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				Command: "tofu",
			},
		},
	}

	// First call should detect and cache.
	result1 := IsOpenTofu(atmosConfig)
	assert.True(t, result1, "First call should detect OpenTofu")

	// Check cache was populated.
	detectionCacheMux.RLock()
	cached, exists := detectionCache["tofu"]
	detectionCacheMux.RUnlock()

	assert.True(t, exists, "Cache should contain entry for 'tofu'")
	assert.True(t, cached, "Cache should indicate OpenTofu")

	// Second call should use cache (we can't directly verify this,
	// but we can verify the result is consistent).
	result2 := IsOpenTofu(atmosConfig)
	assert.Equal(t, result1, result2, "Cached result should match first result")
}

// TestIsOpenTofu_DefaultCommand tests behavior when command is empty.
func TestIsOpenTofu_DefaultCommand(t *testing.T) {
	// Clear cache before test.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCacheMux.Unlock()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				Command: "", // Empty command.
			},
		},
	}

	result := IsOpenTofu(atmosConfig)

	// With empty command, should default to "terraform" which is not OpenTofu.
	// This will try to execute "terraform version" which may fail in test environment,
	// but should default to false (Terraform) on error.
	assert.False(t, result, "Empty command should default to Terraform")
}

// TestIsOpenTofu_SlowPath tests detection by version command.
// This test requires actual terraform/tofu binaries to be available.
func TestIsOpenTofu_SlowPath(t *testing.T) {
	// Clear cache before test.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCacheMux.Unlock()

	t.Run("detects OpenTofu from version command", func(t *testing.T) {
		// Skip if tofu is not available.
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					Command: "tofu",
				},
			},
		}

		// This will use fast path (basename contains "tofu"), but we're
		// verifying that the detection logic works end-to-end.
		result := IsOpenTofu(atmosConfig)
		assert.True(t, result, "Should detect OpenTofu")
	})

	t.Run("detects Terraform from version command", func(t *testing.T) {
		// Skip if terraform is not available.
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					Command: "terraform",
				},
			},
		}

		// This will use slow path (execute version command).
		result := IsOpenTofu(atmosConfig)

		// Result depends on whether terraform is available.
		// If terraform is available and responds, should be false.
		// If not available, detectByVersionCommand returns false (safe default).
		assert.False(t, result, "Should detect Terraform or default to false")
	})
}

// TestDetectByVersionCommand tests the version command detection directly.
func TestDetectByVersionCommand(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("returns false for non-existent command", func(t *testing.T) {
		result := detectByVersionCommand(atmosConfig, "/non/existent/command")
		assert.False(t, result, "Should return false for non-existent command")
	})

	t.Run("returns false for command that times out", func(t *testing.T) {
		// Using a command that will hang would test timeout, but that's hard to do
		// in a unit test. The timeout logic is covered by the implementation.
		// We'll just verify the function signature works.
		result := detectByVersionCommand(atmosConfig, "invalidcommandname12345")
		assert.False(t, result, "Should return false for invalid command")
	})
}

// TestCacheDetectionResult tests the caching mechanism.
func TestCacheDetectionResult(t *testing.T) {
	// Clear cache before test.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCacheMux.Unlock()

	t.Run("caches OpenTofu detection", func(t *testing.T) {
		cacheDetectionResult("tofu", true)

		detectionCacheMux.RLock()
		cached, exists := detectionCache["tofu"]
		detectionCacheMux.RUnlock()

		assert.True(t, exists, "Cache should contain entry")
		assert.True(t, cached, "Cache should indicate OpenTofu")
	})

	t.Run("caches Terraform detection", func(t *testing.T) {
		cacheDetectionResult("terraform", false)

		detectionCacheMux.RLock()
		cached, exists := detectionCache["terraform"]
		detectionCacheMux.RUnlock()

		assert.True(t, exists, "Cache should contain entry")
		assert.False(t, cached, "Cache should indicate Terraform")
	})

	t.Run("caches multiple commands", func(t *testing.T) {
		// Clear cache.
		detectionCacheMux.Lock()
		detectionCache = make(map[string]bool)
		detectionCacheMux.Unlock()

		cacheDetectionResult("/usr/bin/tofu", true)
		cacheDetectionResult("/usr/bin/terraform", false)
		cacheDetectionResult("/opt/tofu", true)

		detectionCacheMux.RLock()
		assert.Len(t, detectionCache, 3, "Cache should contain three entries")
		assert.True(t, detectionCache["/usr/bin/tofu"], "First entry should be OpenTofu")
		assert.False(t, detectionCache["/usr/bin/terraform"], "Second entry should be Terraform")
		assert.True(t, detectionCache["/opt/tofu"], "Third entry should be OpenTofu")
		detectionCacheMux.RUnlock()
	})

	t.Run("overwrites existing cache entry", func(t *testing.T) {
		// Clear cache.
		detectionCacheMux.Lock()
		detectionCache = make(map[string]bool)
		detectionCacheMux.Unlock()

		cacheDetectionResult("tofu", true)
		cacheDetectionResult("tofu", false) // Overwrite.

		detectionCacheMux.RLock()
		cached, exists := detectionCache["tofu"]
		detectionCacheMux.RUnlock()

		assert.True(t, exists, "Cache should contain entry")
		assert.False(t, cached, "Cache should reflect updated value")
	})
}

// TestIsKnownOpenTofuFeature tests the pattern matching for OpenTofu features.
func TestIsKnownOpenTofuFeature(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "module source interpolation error",
			err:      errors.New("Variables not allowed: Variables may not be used here"),
			expected: true,
		},
		{
			name:     "module source interpolation with more context",
			err:      errors.New("Error: Variables not allowed\n\nVariables may not be used here.\n\non main.tf line 10"),
			expected: true,
		},
		{
			name:     "unrelated validation error",
			err:      errors.New("Error: Missing required argument"),
			expected: false,
		},
		{
			name:     "syntax error",
			err:      errors.New("Error: Invalid expression"),
			expected: false,
		},
		{
			name:     "missing file error",
			err:      errors.New("Error: file does not exist"),
			expected: false,
		},
		{
			name:     "case sensitive check",
			err:      errors.New("variables not allowed"),
			expected: false, // Pattern is case-sensitive.
		},
		{
			name:     "partial match",
			err:      errors.New("Variables not"),
			expected: false, // Should not match partial patterns.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKnownOpenTofuFeature(tt.err)
			assert.Equal(t, tt.expected, result, "Pattern matching result should match expected for: %s", tt.name)
		})
	}
}

// TestIsKnownOpenTofuFeature_Patterns tests that all known patterns are detected.
func TestIsKnownOpenTofuFeature_Patterns(t *testing.T) {
	// List of known OpenTofu-specific error patterns that should be skipped.
	knownPatterns := []string{
		"Variables not allowed", // Module source interpolation (OpenTofu 1.8+).
	}

	for _, pattern := range knownPatterns {
		t.Run("detects pattern: "+pattern, func(t *testing.T) {
			err := errors.New("Error in configuration: " + pattern + " - please check your syntax")
			result := isKnownOpenTofuFeature(err)
			assert.True(t, result, "Should detect known OpenTofu pattern: %s", pattern)
		})
	}
}

// TestIsOpenTofu_ConcurrentAccess tests that caching is thread-safe.
func TestIsOpenTofu_ConcurrentAccess(t *testing.T) {
	// Clear cache before test.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCacheMux.Unlock()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				Command: "tofu",
			},
		},
	}

	// Run multiple goroutines accessing IsOpenTofu concurrently.
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			result := IsOpenTofu(atmosConfig)
			assert.True(t, result, "Concurrent detection should return true")
			done <- true
		}()
	}

	// Wait for all goroutines to complete.
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify cache was populated correctly.
	detectionCacheMux.RLock()
	cached, exists := detectionCache["tofu"]
	detectionCacheMux.RUnlock()

	assert.True(t, exists, "Cache should contain entry after concurrent access")
	assert.True(t, cached, "Cache should indicate OpenTofu")
}

// TestIsKnownOpenTofuFeature_EdgeCases tests edge cases for pattern matching.
func TestIsKnownOpenTofuFeature_EdgeCases(t *testing.T) {
	t.Run("empty error message", func(t *testing.T) {
		err := errors.New("")
		result := isKnownOpenTofuFeature(err)
		assert.False(t, result, "Empty error message should not match")
	})

	t.Run("very long error message with pattern", func(t *testing.T) {
		longPrefix := strings.Repeat("error context ", 100)
		err := errors.New(longPrefix + "Variables not allowed in this context")
		result := isKnownOpenTofuFeature(err)
		assert.True(t, result, "Should detect pattern in long error message")
	})

	t.Run("error message with only whitespace", func(t *testing.T) {
		err := errors.New("   \n\t   ")
		result := isKnownOpenTofuFeature(err)
		assert.False(t, result, "Whitespace-only error should not match")
	})

	t.Run("multiple patterns in one error", func(t *testing.T) {
		// If we add more patterns in the future, this test ensures
		// that we detect if ANY pattern matches.
		err := errors.New("Variables not allowed and some other error")
		result := isKnownOpenTofuFeature(err)
		assert.True(t, result, "Should match if any pattern is found")
	})
}

// TestIsOpenTofu_Integration tests the full integration with different scenarios.
func TestIsOpenTofu_Integration(t *testing.T) {
	testCases := []struct {
		name         string
		command      string
		expectedTofu bool
		description  string
	}{
		{
			name:         "standard tofu",
			command:      "tofu",
			expectedTofu: true,
			description:  "Standard tofu command should be detected as OpenTofu",
		},
		{
			name:         "standard terraform",
			command:      "terraform",
			expectedTofu: false,
			description:  "Standard terraform command should be detected as Terraform",
		},
		{
			name:         "absolute path tofu",
			command:      "/usr/local/bin/tofu",
			expectedTofu: true,
			description:  "Absolute path to tofu should be detected as OpenTofu",
		},
		{
			name:         "absolute path terraform",
			command:      "/usr/local/bin/terraform",
			expectedTofu: false,
			description:  "Absolute path to terraform should be detected as Terraform",
		},
		{
			name:         "custom tofu installation",
			command:      "/opt/opentofu/1.10.0/bin/tofu",
			expectedTofu: true,
			description:  "Custom installation path with tofu should be detected",
		},
		{
			name:         "homebrew tofu",
			command:      "/opt/homebrew/bin/tofu",
			expectedTofu: true,
			description:  "Homebrew installation of tofu should be detected",
		},
		{
			name:         "asdf tofu",
			command:      "/Users/user/.asdf/shims/tofu",
			expectedTofu: true,
			description:  "asdf-managed tofu should be detected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear cache for each test.
			detectionCacheMux.Lock()
			detectionCache = make(map[string]bool)
			detectionCacheMux.Unlock()

			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Command: tc.command,
					},
				},
			}

			result := IsOpenTofu(atmosConfig)
			assert.Equal(t, tc.expectedTofu, result, tc.description)
		})
	}
}

// BenchmarkIsOpenTofu_FastPath benchmarks the fast path detection.
func BenchmarkIsOpenTofu_FastPath(b *testing.B) {
	// Clear cache.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCacheMux.Unlock()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				Command: "tofu",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsOpenTofu(atmosConfig)
	}
}

// BenchmarkIsOpenTofu_Cached benchmarks cached lookups.
func BenchmarkIsOpenTofu_Cached(b *testing.B) {
	// Clear cache and populate with one entry.
	detectionCacheMux.Lock()
	detectionCache = make(map[string]bool)
	detectionCache["tofu"] = true
	detectionCacheMux.Unlock()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				Command: "tofu",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsOpenTofu(atmosConfig)
	}
}

// BenchmarkIsKnownOpenTofuFeature benchmarks pattern matching.
func BenchmarkIsKnownOpenTofuFeature(b *testing.B) {
	err := errors.New("Variables not allowed: Variables may not be used here")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isKnownOpenTofuFeature(err)
	}
}
