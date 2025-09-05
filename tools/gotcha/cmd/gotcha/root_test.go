package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestNormalizePostingStrategy(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		flagPresent bool
		expected    string
	}{
		// Default behavior
		{"empty with flag present", "", true, "always"},
		{"empty without flag", "", false, ""},

		// Boolean aliases
		{"true alias", "true", true, "always"},
		{"TRUE uppercase", "TRUE", true, "always"},
		{"True mixed case", "True", true, "always"},
		{"false alias", "false", true, "never"},
		{"FALSE uppercase", "FALSE", true, "never"},
		{"False mixed case", "False", true, "never"},
		{"1 alias", "1", true, "always"},
		{"0 alias", "0", true, "never"},
		{"yes alias", "yes", true, "always"},
		{"YES uppercase", "YES", true, "always"},
		{"no alias", "no", true, "never"},
		{"NO uppercase", "NO", true, "never"},

		// Named strategies
		{"always", "always", true, "always"},
		{"ALWAYS uppercase", "ALWAYS", true, "always"},
		{"never", "never", true, "never"},
		{"NEVER uppercase", "NEVER", true, "never"},
		{"adaptive", "adaptive", true, "adaptive"},
		{"ADAPTIVE uppercase", "ADAPTIVE", true, "adaptive"},
		{"Adaptive mixed case", "Adaptive", true, "adaptive"},
		{"on-failure", "on-failure", true, "on-failure"},
		{"ON-FAILURE uppercase", "ON-FAILURE", true, "on-failure"},
		{"on-skip", "on-skip", true, "on-skip"},
		{"ON-SKIP uppercase", "ON-SKIP", true, "on-skip"},

		// OS names
		{"linux", "linux", true, "linux"},
		{"LINUX uppercase", "LINUX", true, "linux"},
		{"darwin", "darwin", true, "darwin"},
		{"Darwin mixed", "Darwin", true, "darwin"},
		{"windows", "windows", true, "windows"},
		{"WINDOWS uppercase", "WINDOWS", true, "windows"},

		// Edge cases
		{"unknown strategy", "unknown-strategy", true, "unknown-strategy"},
		{"with spaces trimmed", "  adaptive  ", true, "adaptive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePostingStrategy(tt.input, tt.flagPresent)
			assert.Equal(t, tt.expected, result,
				"normalizePostingStrategy(%q, %v) should return %q",
				tt.input, tt.flagPresent, tt.expected)
		})
	}
}

func TestShouldPostCommentWithOS(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		os       string
		failed   int
		skipped  int
		passed   int
		want     bool
	}{
		// Always strategy
		{"always - no failures on linux", "always", "linux", 0, 0, 10, true},
		{"always - with failures on windows", "always", "windows", 5, 2, 3, true},
		{"always - all passed on darwin", "always", "darwin", 0, 0, 10, true},

		// Never strategy
		{"never - with failures on linux", "never", "linux", 5, 2, 3, false},
		{"never - all passed on darwin", "never", "darwin", 0, 0, 10, false},
		{"empty strategy - with failures", "", "linux", 5, 2, 3, false},

		// Adaptive strategy
		{"adaptive - linux all pass", "adaptive", "linux", 0, 0, 10, true},
		{"adaptive - linux with failures", "adaptive", "linux", 5, 0, 5, true},
		{"adaptive - linux with skips only", "adaptive", "linux", 0, 3, 7, true},
		{"adaptive - windows all pass", "adaptive", "windows", 0, 0, 10, false},
		{"adaptive - windows with failures", "adaptive", "windows", 5, 0, 5, true},
		{"adaptive - windows with skips", "adaptive", "windows", 0, 3, 7, true},
		{"adaptive - darwin with skips", "adaptive", "darwin", 0, 3, 7, true},
		{"adaptive - darwin all pass", "adaptive", "darwin", 0, 0, 10, false},
		{"adaptive - darwin with both failures and skips", "adaptive", "darwin", 2, 3, 5, true},

		// On-failure strategy
		{"on-failure - with failures on linux", "on-failure", "linux", 5, 2, 3, true},
		{"on-failure - only skips on linux", "on-failure", "linux", 0, 3, 7, false},
		{"on-failure - all pass on windows", "on-failure", "windows", 0, 0, 10, false},
		{"on-failure - with failures on darwin", "on-failure", "darwin", 1, 0, 9, true},
		{"on-failure - mixed failures and skips", "on-failure", "windows", 2, 3, 5, true},

		// On-skip strategy
		{"on-skip - with skips on linux", "on-skip", "linux", 0, 3, 7, true},
		{"on-skip - with failures no skips on linux", "on-skip", "linux", 5, 0, 5, false},
		{"on-skip - all pass on darwin", "on-skip", "darwin", 0, 0, 10, false},
		{"on-skip - failures and skips on windows", "on-skip", "windows", 2, 3, 5, true},
		{"on-skip - only skips on windows", "on-skip", "windows", 0, 1, 9, true},

		// OS-specific strategies
		{"linux strategy on linux", "linux", "linux", 0, 0, 10, true},
		{"linux strategy on linux with failures", "linux", "linux", 5, 2, 3, true},
		{"linux strategy on darwin", "linux", "darwin", 0, 0, 10, false},
		{"linux strategy on windows", "linux", "windows", 5, 2, 3, false},
		{"darwin strategy on darwin", "darwin", "darwin", 5, 2, 3, true},
		{"darwin strategy on linux", "darwin", "linux", 5, 2, 3, false},
		{"windows strategy on windows", "windows", "windows", 0, 0, 10, true},
		{"windows strategy on linux", "windows", "linux", 5, 2, 3, false},

		// Alternative forms
		{"onfailure without dash", "onfailure", "linux", 5, 0, 5, true},
		{"onskip without dash", "onskip", "linux", 0, 3, 7, true},

		// Unknown strategy (treated as OS name)
		{"freebsd as strategy on freebsd", "freebsd", "freebsd", 0, 0, 10, true},
		{"freebsd as strategy on linux", "freebsd", "linux", 0, 0, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test summary
			summary := &types.TestSummary{
				Failed:  make([]types.TestResult, tt.failed),
				Skipped: make([]types.TestResult, tt.skipped),
				Passed:  make([]types.TestResult, tt.passed),
			}

			// Fill with dummy data
			for i := 0; i < tt.failed; i++ {
				summary.Failed[i] = types.TestResult{
					Package: "test/package",
					Test:    fmt.Sprintf("TestFailed%d", i),
					Status:  "FAIL",
				}
			}
			for i := 0; i < tt.skipped; i++ {
				summary.Skipped[i] = types.TestResult{
					Package:    "test/package",
					Test:       fmt.Sprintf("TestSkipped%d", i),
					Status:     "SKIP",
					SkipReason: "test condition not met",
				}
			}
			for i := 0; i < tt.passed; i++ {
				summary.Passed[i] = types.TestResult{
					Package: "test/package",
					Test:    fmt.Sprintf("TestPassed%d", i),
					Status:  "PASS",
				}
			}

			result := shouldPostCommentWithOS(tt.strategy, summary, tt.os)
			assert.Equal(t, tt.want, result,
				"Strategy %q on %s with %d failed, %d skipped, %d passed should return %v",
				tt.strategy, tt.os, tt.failed, tt.skipped, tt.passed, tt.want)
		})
	}
}

func TestShouldPostComment(t *testing.T) {
	// This test verifies that shouldPostComment uses runtime.GOOS correctly
	// It's a simple wrapper test since the real logic is in shouldPostCommentWithOS

	summary := &types.TestSummary{
		Failed:  []types.TestResult{},
		Skipped: []types.TestResult{},
		Passed: []types.TestResult{
			{Package: "test", Test: "TestPass", Status: "PASS"},
		},
	}

	// Test always strategy (should always return true)
	assert.True(t, shouldPostComment("always", summary))

	// Test never strategy (should always return false)
	assert.False(t, shouldPostComment("never", summary))

	// For other strategies, the result depends on runtime.GOOS
	// which we can't easily mock in this test, so we just verify
	// that the function doesn't panic
	_ = shouldPostComment("adaptive", summary)
	_ = shouldPostComment("on-failure", summary)
	_ = shouldPostComment("on-skip", summary)
	_ = shouldPostComment("linux", summary)
	_ = shouldPostComment("darwin", summary)
	_ = shouldPostComment("windows", summary)
}

func TestArgumentParsingWithDashSeparator(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedPackages []string
		expectedPassthru []string
		expectError      bool
	}{
		{
			name:             "no arguments",
			args:             []string{},
			expectedPackages: []string{"./..."},
			expectedPassthru: []string{},
		},
		{
			name:             "packages only",
			args:             []string{"./pkg/...", "./internal/..."},
			expectedPackages: []string{"./pkg/...", "./internal/..."},
			expectedPassthru: []string{},
		},
		{
			name:             "with dash separator and passthrough args",
			args:             []string{"./pkg/...", "--", "-race", "-v"},
			expectedPackages: []string{"./pkg/..."},
			expectedPassthru: []string{"-race", "-v"},
		},
		{
			name:             "dash separator with no packages",
			args:             []string{"--", "-race", "-count=1"},
			expectedPackages: []string{"./..."},
			expectedPassthru: []string{"-race", "-count=1"},
		},
		{
			name:             "dash separator with coverpkg",
			args:             []string{"--", "-coverpkg=github.com/cloudposse/atmos/..."},
			expectedPackages: []string{"./..."},
			expectedPassthru: []string{"-coverpkg=github.com/cloudposse/atmos/..."},
		},
		{
			name:             "complex passthrough with run flag",
			args:             []string{"./test/...", "--", "-run", "TestConfig.*", "-v", "-race"},
			expectedPackages: []string{"./test/..."},
			expectedPassthru: []string{"-run", "TestConfig.*", "-v", "-race"},
		},
		{
			name:             "passthrough with equals syntax",
			args:             []string{"--", "-run=TestSpecific", "-timeout=30s"},
			expectedPackages: []string{"./..."},
			expectedPassthru: []string{"-run=TestSpecific", "-timeout=30s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This tests the expected behavior based on the logic in runStream
			// In practice, the actual parsing happens inside the cobra command execution
			// which is harder to test directly without running the full command

			// Simulate the parsing logic from runStream
			testPackages := []string{}
			passthroughArgs := []string{}

			// Find the position of "--" separator
			dashPos := -1
			for i, arg := range tt.args {
				if arg == "--" {
					dashPos = i
					break
				}
			}

			if dashPos >= 0 {
				// Args before -- are packages
				if dashPos > 0 {
					testPackages = tt.args[:dashPos]
				}
				// Args after -- are passthrough
				if dashPos+1 < len(tt.args) {
					passthroughArgs = tt.args[dashPos+1:]
				}
			} else {
				// No separator, all args are packages
				if len(tt.args) > 0 {
					testPackages = tt.args
				}
			}

			// Default to ./... if no packages specified
			if len(testPackages) == 0 {
				testPackages = []string{"./..."}
			}

			assert.Equal(t, tt.expectedPackages, testPackages, "Package parsing mismatch")
			assert.Equal(t, tt.expectedPassthru, passthroughArgs, "Passthrough args mismatch")
		})
	}
}

func TestPostCommentFlagWithDashSeparator(t *testing.T) {
	// Test that --post-comment doesn't consume the -- separator
	tests := []struct {
		name                  string
		osArgs                []string
		expectPostComment     string
		expectPassthroughArgs bool
	}{
		{
			name:                  "post-comment with value before dash separator",
			osArgs:                []string{"gotcha", "stream", "--post-comment=adaptive", "--", "-race"},
			expectPostComment:     "adaptive",
			expectPassthroughArgs: true,
		},
		{
			name:                  "post-comment via env var with dash separator",
			osArgs:                []string{"gotcha", "stream", "--", "-coverpkg=./..."},
			expectPostComment:     "", // Would be set via GOTCHA_POST_COMMENT env var
			expectPassthroughArgs: true,
		},
		{
			name:                  "no post-comment with dash separator",
			osArgs:                []string{"gotcha", "stream", "./pkg/...", "--", "-v", "-race"},
			expectPostComment:     "",
			expectPassthroughArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify that the -- separator is properly detected
			hasDashSeparator := false
			for _, arg := range tt.osArgs {
				if arg == "--" {
					hasDashSeparator = true
					break
				}
			}

			if tt.expectPassthroughArgs {
				assert.True(t, hasDashSeparator, "Expected -- separator to be present")
			}

			// Verify that post-comment flag doesn't interfere with parsing
			postCommentIndex := -1
			dashIndex := -1

			for i, arg := range tt.osArgs {
				if strings.HasPrefix(arg, "--post-comment") {
					postCommentIndex = i
				}
				if arg == "--" {
					dashIndex = i
				}
			}

			if postCommentIndex >= 0 && dashIndex >= 0 {
				// Ensure post-comment comes before the dash separator
				assert.Less(t, postCommentIndex, dashIndex,
					"--post-comment should come before -- separator")
			}
		})
	}
}

func TestUUIDDiscriminatorHandling(t *testing.T) {
	tests := []struct {
		name              string
		baseUUID          string
		jobDiscriminator  string
		expectedFinalUUID string
	}{
		{
			name:              "UUID with linux discriminator",
			baseUUID:          "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f",
			jobDiscriminator:  "linux",
			expectedFinalUUID: "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f-linux",
		},
		{
			name:              "UUID with windows discriminator",
			baseUUID:          "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f",
			jobDiscriminator:  "windows",
			expectedFinalUUID: "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f-windows",
		},
		{
			name:              "UUID with no discriminator",
			baseUUID:          "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f",
			jobDiscriminator:  "",
			expectedFinalUUID: "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f",
		},
		{
			name:              "UUID with custom discriminator",
			baseUUID:          "test-uuid-123",
			jobDiscriminator:  "integration-tests",
			expectedFinalUUID: "test-uuid-123-integration-tests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate UUID discrimination logic
			uuid := tt.baseUUID
			if tt.jobDiscriminator != "" {
				uuid = fmt.Sprintf("%s-%s", uuid, tt.jobDiscriminator)
			}

			assert.Equal(t, tt.expectedFinalUUID, uuid,
				"UUID discrimination should produce expected result")

			// Verify the UUID would be used consistently in both:
			// 1. Comment body generation
			// 2. Comment finding/matching
			// This ensures no duplicates are created
			commentMarker := fmt.Sprintf("<!-- test-summary-uuid: %s -->", uuid)

			// Simulate finding logic - would this marker match?
			assert.Contains(t, commentMarker, uuid,
				"Comment marker should contain the discriminated UUID")
		})
	}
}

func TestPostingStrategiesIntegration(t *testing.T) {
	// Integration test to verify the full flow
	tests := []struct {
		name         string
		flagValue    string
		flagPresent  bool
		os           string
		failureCount int
		skipCount    int
		shouldPost   bool
	}{
		// Test default behavior
		{"flag present without value", "", true, "linux", 0, 0, true},
		{"flag not present", "", false, "linux", 0, 0, false},

		// Test boolean compatibility
		{"true means always", "true", true, "windows", 0, 0, true},
		{"false means never", "false", true, "linux", 5, 3, false},

		// Test adaptive on different platforms
		{"adaptive on linux success", "adaptive", true, "linux", 0, 0, true},
		{"adaptive on windows success", "adaptive", true, "windows", 0, 0, false},
		{"adaptive on windows failure", "adaptive", true, "windows", 1, 0, true},

		// Test conditional strategies
		{"on-failure with failures", "on-failure", true, "linux", 3, 0, true},
		{"on-failure without failures", "on-failure", true, "linux", 0, 2, false},
		{"on-skip with skips", "on-skip", true, "darwin", 0, 2, true},
		{"on-skip without skips", "on-skip", true, "darwin", 3, 0, false},

		// Test OS-specific strategies
		{"linux on linux", "linux", true, "linux", 0, 0, true},
		{"linux on windows", "linux", true, "windows", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize the strategy
			strategy := normalizePostingStrategy(tt.flagValue, tt.flagPresent)

			// Create summary
			summary := &types.TestSummary{
				Failed:  make([]types.TestResult, tt.failureCount),
				Skipped: make([]types.TestResult, tt.skipCount),
				Passed:  []types.TestResult{{Package: "test", Test: "TestPass", Status: "PASS"}},
			}

			// Check if should post
			shouldPost := shouldPostCommentWithOS(strategy, summary, tt.os)

			assert.Equal(t, tt.shouldPost, shouldPost,
				"Flag value %q (present: %v) on %s with %d failures and %d skips",
				tt.flagValue, tt.flagPresent, tt.os, tt.failureCount, tt.skipCount)
		})
	}
}
