package main

import (
	"os"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func TestColorConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected termenv.Profile
	}{
		{
			name: "GitHub Actions environment",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
				"CI":             "true",
			},
			expected: termenv.ANSI,
		},
		{
			name: "GitHub Actions with TERM=xterm-256color",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
				"TERM":           "xterm-256color",
			},
			expected: termenv.ANSI256,
		},
		{
			name: "Local development with truecolor terminal",
			envVars: map[string]string{
				"TERM":      "xterm-256color",
				"COLORTERM": "truecolor",
			},
			expected: termenv.TrueColor,
		},
		{
			name: "CI without explicit support",
			envVars: map[string]string{
				"CI": "true",
			},
			expected: termenv.ANSI,
		},
		{
			name:     "No color support",
			envVars:  map[string]string{},
			expected: termenv.Ascii,
		},
		{
			name: "Force color via FORCE_COLOR",
			envVars: map[string]string{
				"FORCE_COLOR": "1",
			},
			expected: termenv.ANSI,
		},
		{
			name: "Disable color via NO_COLOR",
			envVars: map[string]string{
				"NO_COLOR":  "1",
				"COLORTERM": "truecolor",
			},
			expected: termenv.Ascii,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original environment
			originalEnv := make(map[string]string)
			for key := range tt.envVars {
				originalEnv[key] = os.Getenv(key)
			}
			// Also store some standard vars that might affect detection
			for _, key := range []string{"GITHUB_ACTIONS", "CI", "TERM", "COLORTERM", "FORCE_COLOR", "NO_COLOR"} {
				if _, exists := originalEnv[key]; !exists {
					originalEnv[key] = os.Getenv(key)
				}
			}

			// Clean environment first
			for key := range originalEnv {
				os.Unsetenv(key)
			}

			// Set test environment
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Test the color profile detection
			profile := detectColorProfile()
			assert.Equal(t, tt.expected, profile, "Color profile should match expected")

			// Restore original environment
			for key := range originalEnv {
				os.Unsetenv(key)
			}
			for key, value := range originalEnv {
				if value != "" {
					os.Setenv(key, value)
				}
			}
		})
	}
}

func TestColorOutput(t *testing.T) {
	tests := []struct {
		name           string
		envVars        map[string]string
		expectColorSeq bool
		description    string
	}{
		{
			name: "GitHub Actions should render colors",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expectColorSeq: true,
			description:    "GitHub Actions supports ANSI colors",
		},
		{
			name: "CI environment should render colors",
			envVars: map[string]string{
				"CI": "true",
			},
			expectColorSeq: true,
			description: "CI environments typically support ANSI colors",
		},
		{
			name: "NO_COLOR should disable colors",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
				"NO_COLOR":       "1",
			},
			expectColorSeq: false,
			description:    "NO_COLOR env var should disable colors",
		},
		{
			name: "FORCE_COLOR should enable colors",
			envVars: map[string]string{
				"FORCE_COLOR": "1",
			},
			expectColorSeq: true,
			description:    "FORCE_COLOR should enable colors even in minimal environments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store and clean environment
			originalEnv := make(map[string]string)
			envVarsToCheck := []string{"GITHUB_ACTIONS", "CI", "TERM", "COLORTERM", "FORCE_COLOR", "NO_COLOR"}
			
			for _, key := range envVarsToCheck {
				originalEnv[key] = os.Getenv(key)
				os.Unsetenv(key)
			}

			// Set test environment
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Configure color profile
			configureColors()

			// Test color rendering
			coloredText := passStyle.Render("✔ TEST")
			
			if tt.expectColorSeq {
				// Should contain ANSI escape sequences
				assert.Contains(t, coloredText, "\033[", "Expected ANSI color codes in output: %s", tt.description)
				assert.NotEqual(t, "✔ TEST", coloredText, "Text should be styled with colors")
			} else {
				// Should not contain ANSI escape sequences
				assert.NotContains(t, coloredText, "\033[", "Should not contain ANSI color codes: %s", tt.description)
				assert.Equal(t, "✔ TEST", coloredText, "Text should not be styled when colors disabled")
			}

			// Restore environment
			for _, key := range envVarsToCheck {
				os.Unsetenv(key)
				if originalEnv[key] != "" {
					os.Setenv(key, originalEnv[key])
				}
			}
		})
	}
}

func TestGitHubActionsDetection(t *testing.T) {
	tests := []struct {
		name      string
		envVars   map[string]string
		expectGHA bool
	}{
		{
			name: "GITHUB_ACTIONS set to true",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expectGHA: true,
		},
		{
			name: "GITHUB_ACTIONS set to any value",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "1",
			},
			expectGHA: true,
		},
		{
			name:      "No GitHub Actions env var",
			envVars:   map[string]string{},
			expectGHA: false,
		},
		{
			name: "Other CI environment",
			envVars: map[string]string{
				"CI": "true",
			},
			expectGHA: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original environment
			originalGHA := os.Getenv("GITHUB_ACTIONS")
			originalCI := os.Getenv("CI")
			
			// Clean environment
			os.Unsetenv("GITHUB_ACTIONS")
			os.Unsetenv("CI")

			// Set test environment
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Test detection
			isGHA := isGitHubActions()
			assert.Equal(t, tt.expectGHA, isGHA, "GitHub Actions detection should match expected")

			// Restore environment
			os.Unsetenv("GITHUB_ACTIONS")
			os.Unsetenv("CI")
			if originalGHA != "" {
				os.Setenv("GITHUB_ACTIONS", originalGHA)
			}
			if originalCI != "" {
				os.Setenv("CI", originalCI)
			}
		})
	}
}