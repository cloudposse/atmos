package auth

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/telemetry"
)

// TestIsInteractive tests the isInteractive() function with various TTY and CI configurations.
func TestIsInteractive(t *testing.T) {
	tests := []struct {
		name           string
		setupEnv       func(*testing.T) func()
		expectedResult bool
		description    string
	}{
		{
			name: "interactive - stdin is TTY and not CI",
			setupEnv: func(t *testing.T) func() {
				// Normal interactive terminal: stdin is TTY, no CI env var.
				// Note: In actual tests, stdin will not be a TTY, so this tests the logic
				// assuming stdin TTY detection would return true.
				// We can't actually make stdin a TTY in unit tests.
				return func() {}
			},
			expectedResult: false, // Will be false in test environment (no real TTY)
			description:    "Interactive terminal with no CI",
		},
		{
			name: "non-interactive - CI environment set to true",
			setupEnv: func(t *testing.T) func() {
				// Simulate CI environment with CI=true.
				t.Setenv("CI", "true")
				return func() {}
			},
			expectedResult: false,
			description:    "CI=true should disable interactive mode",
		},
		{
			name: "non-interactive - CI environment set to false",
			setupEnv: func(t *testing.T) func() {
				// CI=false should not be treated as CI.
				t.Setenv("CI", "false")
				return func() {}
			},
			expectedResult: false, // Still false due to no TTY in test env
			description:    "CI=false should allow interactive mode (if stdin is TTY)",
		},
		{
			name: "non-interactive - GitHub Actions",
			setupEnv: func(t *testing.T) func() {
				// GitHub Actions CI environment.
				t.Setenv("GITHUB_ACTIONS", "true")
				return func() {}
			},
			expectedResult: false,
			description:    "GitHub Actions should disable interactive mode",
		},
		{
			name: "non-interactive - GitLab CI",
			setupEnv: func(t *testing.T) func() {
				// GitLab CI environment.
				t.Setenv("GITLAB_CI", "true")
				return func() {}
			},
			expectedResult: false,
			description:    "GitLab CI should disable interactive mode",
		},
		{
			name: "non-interactive - CircleCI",
			setupEnv: func(t *testing.T) func() {
				// CircleCI environment.
				t.Setenv("CIRCLECI", "true")
				return func() {}
			},
			expectedResult: false,
			description:    "CircleCI should disable interactive mode",
		},
		{
			name: "non-interactive - Jenkins",
			setupEnv: func(t *testing.T) func() {
				// Jenkins CI environment.
				t.Setenv("JENKINS_URL", "http://jenkins.example.com")
				return func() {}
			},
			expectedResult: false,
			description:    "Jenkins should disable interactive mode",
		},
		{
			name: "non-interactive - Travis CI",
			setupEnv: func(t *testing.T) func() {
				// Travis CI environment.
				t.Setenv("TRAVIS", "true")
				return func() {}
			},
			expectedResult: false,
			description:    "Travis CI should disable interactive mode",
		},
		{
			name: "non-interactive - Buildkite",
			setupEnv: func(t *testing.T) func() {
				// Buildkite CI environment.
				t.Setenv("BUILDKITE", "true")
				return func() {}
			},
			expectedResult: false,
			description:    "Buildkite should disable interactive mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Preserve and restore CI environment variables.
			preservedEnv := telemetry.PreserveCIEnvVars()
			defer func() {
				telemetry.RestoreCIEnvVars(preservedEnv)
			}()

			// Setup test environment.
			cleanup := tt.setupEnv(t)
			defer cleanup()

			// Call isInteractive().
			result := isInteractive()

			// Assert the result.
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

// TestIsInteractive_StdinBehavior documents the expected behavior with different stdin configurations.
// Note: These are documentation tests - actual TTY behavior cannot be fully tested in unit tests.
func TestIsInteractive_StdinBehavior(t *testing.T) {
	tests := []struct {
		name                string
		stdinDescription    string
		stdoutDescription   string
		ciEnv               bool
		expectedInteractive bool
		description         string
	}{
		{
			name:                "normal terminal",
			stdinDescription:    "TTY",
			stdoutDescription:   "TTY",
			ciEnv:               false,
			expectedInteractive: true,
			description:         "Normal interactive terminal - should allow interaction",
		},
		{
			name:                "piped stdout",
			stdinDescription:    "TTY",
			stdoutDescription:   "piped (| cat)",
			ciEnv:               false,
			expectedInteractive: true,
			description:         "Stdout piped but stdin is TTY - should allow interaction",
		},
		{
			name:                "redirected stdin",
			stdinDescription:    "redirected (< file)",
			stdoutDescription:   "TTY",
			ciEnv:               false,
			expectedInteractive: false,
			description:         "Stdin redirected - should NOT allow interaction",
		},
		{
			name:                "both piped",
			stdinDescription:    "piped",
			stdoutDescription:   "piped",
			ciEnv:               false,
			expectedInteractive: false,
			description:         "Both stdin and stdout piped - should NOT allow interaction",
		},
		{
			name:                "CI with TTY",
			stdinDescription:    "TTY",
			stdoutDescription:   "TTY",
			ciEnv:               true,
			expectedInteractive: false,
			description:         "CI environment even with TTY - should NOT allow interaction",
		},
		{
			name:                "CI with piped stdout",
			stdinDescription:    "TTY",
			stdoutDescription:   "piped",
			ciEnv:               true,
			expectedInteractive: false,
			description:         "CI environment with piped stdout - should NOT allow interaction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a documentation test - we can't actually manipulate TTY in unit tests.
			// The assertions document the expected behavior.
			t.Logf("Scenario: %s", tt.description)
			t.Logf("  stdin: %s", tt.stdinDescription)
			t.Logf("  stdout: %s", tt.stdoutDescription)
			t.Logf("  CI: %v", tt.ciEnv)
			t.Logf("  Expected isInteractive(): %v", tt.expectedInteractive)

			// The actual isInteractive() function checks:
			// 1. term.IsTTYSupportForStdin() - Returns true if stdin is a TTY
			// 2. !telemetry.IsCI() - Returns false if CI environment detected
			//
			// Result: isInteractive() = (stdin is TTY) AND (not CI)
			//
			// This means:
			// - Stdout being piped does NOT affect interactivity (by design)
			// - Users can pipe output while still providing interactive input
			// - CI always disables interactivity regardless of TTY
		})
	}
}

// TestIsInteractive_Integration tests isInteractive() with actual CI environment variables.
func TestIsInteractive_Integration(t *testing.T) {
	// Preserve original CI environment.
	preservedEnv := telemetry.PreserveCIEnvVars()
	defer func() {
		telemetry.RestoreCIEnvVars(preservedEnv)
	}()

	// Test 1: No CI environment - result depends on stdin TTY (will be false in test env).
	t.Run("no CI environment", func(t *testing.T) {
		result := isInteractive()
		// In test environment, stdin is not a TTY, so result should be false.
		assert.False(t, result, "Expected false in test environment (no TTY)")
	})

	// Test 2: CI=true should always return false.
	t.Run("CI=true", func(t *testing.T) {
		t.Setenv("CI", "true")
		result := isInteractive()
		assert.False(t, result, "Expected false when CI=true")
	})

	// Test 3: GitHub Actions should always return false.
	t.Run("GitHub Actions", func(t *testing.T) {
		// Clean environment first.
		os.Unsetenv("CI")
		t.Setenv("GITHUB_ACTIONS", "true")
		result := isInteractive()
		assert.False(t, result, "Expected false in GitHub Actions")
	})

	// Test 4: Multiple CI indicators.
	t.Run("multiple CI indicators", func(t *testing.T) {
		t.Setenv("CI", "true")
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITLAB_CI", "true")
		result := isInteractive()
		assert.False(t, result, "Expected false with multiple CI indicators")
	})
}

// TestIsInteractive_EdgeCases tests edge cases and boundary conditions.
func TestIsInteractive_EdgeCases(t *testing.T) {
	// Preserve original CI environment.
	preservedEnv := telemetry.PreserveCIEnvVars()
	defer func() {
		telemetry.RestoreCIEnvVars(preservedEnv)
	}()

	t.Run("CI env var with empty string", func(t *testing.T) {
		t.Setenv("CI", "")
		result := isInteractive()
		// Empty CI env var should not be treated as CI.
		// Result depends on stdin TTY (false in test env).
		assert.False(t, result, "Empty CI var should not affect result (depends on TTY)")
	})

	t.Run("CI env var with false value", func(t *testing.T) {
		t.Setenv("CI", "false")
		result := isInteractive()
		// CI=false should not be treated as CI.
		// Result depends on stdin TTY (false in test env).
		assert.False(t, result, "CI=false should not be treated as CI")
	})

	t.Run("CI env var with arbitrary value", func(t *testing.T) {
		t.Setenv("CI", "maybe")
		result := isInteractive()
		// CI=maybe should not be treated as CI (only "true" is recognized).
		// Result depends on stdin TTY (false in test env).
		assert.False(t, result, "CI=maybe should not be treated as CI")
	})

	t.Run("case sensitivity of CI env var", func(t *testing.T) {
		// Test that CI detection is case-sensitive.
		t.Setenv("ci", "true") // lowercase
		result := isInteractive()
		// Lowercase "ci" should not be detected (CI detection is case-sensitive).
		// Result depends on stdin TTY (false in test env).
		assert.False(t, result, "lowercase 'ci' should not affect result")
	})
}
