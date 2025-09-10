package markdown

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestGenerateGitHubComment(t *testing.T) {
	tests := []struct {
		name            string
		summary         *types.TestSummary
		expectComment   bool
		checkContent    []string
		notCheckContent []string
	}{
		{
			name: "Simple passing tests",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg/test", Test: "TestPass1", Duration: 0.1},
					{Package: "pkg/test", Test: "TestPass2", Duration: 0.2},
					{Package: "pkg/test", Test: "TestPass3", Duration: 0.3},
				},
				Failed:  []types.TestResult{},
				Skipped: []types.TestResult{},
			},
			expectComment: true,
			checkContent: []string{
				"✅", // Success badge
				"3", // Test count
			},
		},
		{
			name: "Tests with failures",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg/test", Test: "TestPass1", Duration: 0.1},
					{Package: "pkg/test", Test: "TestPass2", Duration: 0.2},
				},
				Failed: []types.TestResult{
					{
						Package:  "pkg/test",
						Test:     "TestFailed1",
						Duration: 1.5,
					},
					{
						Package:  "pkg/test",
						Test:     "TestFailed2",
						Duration: 2.0,
					},
				},
				Skipped: []types.TestResult{
					{
						Package:    "pkg/test",
						Test:       "TestSkipped1",
						SkipReason: "Not implemented",
					},
				},
			},
			expectComment: true,
			checkContent: []string{
				"❌", // Failure badge
				"Failed Tests",
				"TestFailed1",
				"TestFailed2",
			},
		},
		{
			name: "Tests with skip reasons",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg/test", Test: "TestPassed1", Status: "pass"},
					{Package: "pkg/test", Test: "TestPassed2", Status: "pass"},
				},
				Failed: []types.TestResult{},
				Skipped: []types.TestResult{
					{
						Package:    "pkg/test",
						Test:       "TestSkipped1",
						SkipReason: "Requires external service",
					},
					{
						Package:    "pkg/test",
						Test:       "TestSkipped2",
						SkipReason: "Not supported on Windows",
					},
				},
			},
			expectComment: true,
			checkContent: []string{
				"✅", // Success badge (no failures)
				"Skipped Tests",
				"TestSkipped1",
				"Requires external service",
				"TestSkipped2",
				"Not supported on Windows",
			},
		},
		{
			name: "Tests with coverage",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg/test", Test: "Test1", Status: "pass"},
					{Package: "pkg/test", Test: "Test2", Status: "pass"},
					{Package: "pkg/test", Test: "Test3", Status: "pass"},
					{Package: "pkg/test", Test: "Test4", Status: "pass"},
					{Package: "pkg/test", Test: "Test5", Status: "pass"},
					{Package: "pkg/test", Test: "Test6", Status: "pass"},
					{Package: "pkg/test", Test: "Test7", Status: "pass"},
					{Package: "pkg/test", Test: "Test8", Status: "pass"},
					{Package: "pkg/test", Test: "Test9", Status: "pass"},
					{Package: "pkg/test", Test: "Test10", Status: "pass"},
				},
				Failed:   []types.TestResult{},
				Skipped:  []types.TestResult{},
				Coverage: "coverage: 85.5% of statements",
			},
			expectComment: true,
			checkContent: []string{
				"Coverage",
				"85.5%",
			},
		},
		{
			name: "Empty test results",
			summary: &types.TestSummary{
				Passed:  []types.TestResult{},
				Failed:  []types.TestResult{},
				Skipped: []types.TestResult{},
			},
			expectComment: true,
			checkContent: []string{
				"NO_TESTS", // Badge text
			},
		},
		{
			name: "Large test suite with many packages",
			summary: &types.TestSummary{
				Passed:  make([]types.TestResult, 95),
				Skipped: make([]types.TestResult, 2),
				Failed: []types.TestResult{
					{Package: "pkg/package1", Test: "TestFail1", Duration: 1.5},
					{Package: "pkg/package3", Test: "TestFail2", Duration: 2.0},
					{Package: "pkg/package4", Test: "TestFail3", Duration: 0.5},
				},
			},
			expectComment: true,
			checkContent: []string{
				"❌",         // Failure badge
				"95",        // Passed count in badge
				"3",         // Failed count in badge
				"TestFail1", // Failed test names
				"TestFail2",
				"TestFail3",
				"package1", // Package names (shortened)
				"package3",
				"package4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := GenerateGitHubComment(tt.summary, "test-uuid")

			if tt.expectComment {
				assert.NotEmpty(t, comment, "Expected non-empty comment")
			}

			for _, content := range tt.checkContent {
				assert.Contains(t, comment, content, "Comment should contain: %s", content)
			}

			for _, content := range tt.notCheckContent {
				assert.NotContains(t, comment, content, "Comment should not contain: %s", content)
			}
		})
	}
}

// TestTruncateToEssentials is removed as CommentBuilder no longer exists
// The truncation logic is now internal to the comment generation functions

// TestAddPassedTestsWithLimit is removed as CommentBuilder no longer exists
// The truncation logic is now internal to the comment generation functions

// TestAddCoverageWithLimit is removed as CommentBuilder no longer exists
// Coverage formatting is now internal to the comment generation functions

func TestPlatformInHeader(t *testing.T) {
	tests := []struct {
		platform string
		expected string
	}{
		{"linux", "Test Results (linux)"},
		{"darwin", "Test Results (darwin)"},
		{"windows", "Test Results (windows)"},
		{"ubuntu-latest", "Test Results (ubuntu-latest)"},
		{"", "Test Results"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			summary := &types.TestSummary{
				Passed: []types.TestResult{{Package: "test", Test: "Test1"}},
			}

			comment := GenerateAdaptiveComment(summary, "test-uuid", tt.platform)
			assert.Contains(t, comment, tt.expected)
		})
	}
}

func TestDiscriminatorInCommentTitle(t *testing.T) {
	tests := []struct {
		name          string
		discriminator string
		hasFailed     bool
		expectedTitle string
	}{
		{
			name:          "discriminator with passing tests",
			discriminator: "linux",
			hasFailed:     false,
			expectedTitle: "# ✅ Test Results (linux)",
		},
		{
			name:          "discriminator with failing tests",
			discriminator: "windows",
			hasFailed:     true,
			expectedTitle: "# ❌ Test Results (windows)",
		},
		{
			name:          "no discriminator with passing tests",
			discriminator: "",
			hasFailed:     false,
			expectedTitle: "# ✅ Test Results",
		},
		{
			name:          "matrix job discriminator",
			discriminator: "ubuntu-latest",
			hasFailed:     false,
			expectedTitle: "# ✅ Test Results (ubuntu-latest)",
		},
		{
			name:          "compound discriminator project/os",
			discriminator: "atmos/linux",
			hasFailed:     false,
			expectedTitle: "# ✅ Test Results (atmos/linux)",
		},
		{
			name:          "compound discriminator tool/os",
			discriminator: "gotcha/windows",
			hasFailed:     true,
			expectedTitle: "# ❌ Test Results (gotcha/windows)",
		},
		{
			name:          "project context only",
			discriminator: "atmos",
			hasFailed:     false,
			expectedTitle: "# ✅ Test Results (atmos)",
		},
		{
			name:          "tool context with platform",
			discriminator: "gotcha/ubuntu-latest",
			hasFailed:     false,
			expectedTitle: "# ✅ Test Results (gotcha/ubuntu-latest)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := &types.TestSummary{}
			if tt.hasFailed {
				summary.Failed = []types.TestResult{{Package: "test", Test: "TestFail"}}
			} else {
				summary.Passed = []types.TestResult{{Package: "test", Test: "TestPass"}}
			}

			comment := GenerateAdaptiveComment(summary, "test-uuid", tt.discriminator)
			
			// The title should be on its own line after the UUID comment
			lines := strings.Split(comment, "\n")
			var titleFound bool
			for _, line := range lines {
				if strings.HasPrefix(line, "# ") {
					assert.Equal(t, tt.expectedTitle, line, "Title mismatch for discriminator '%s'", tt.discriminator)
					titleFound = true
					break
				}
			}
			assert.True(t, titleFound, "Could not find title line starting with '# '")
		})
	}
}

func TestCommentStructureOrdering(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg/test", Test: "TestPass1"},
			{Package: "pkg/test", Test: "TestPass2"},
			{Package: "pkg/test", Test: "TestPass3"},
			{Package: "pkg/test", Test: "TestPass4"},
			{Package: "pkg/test", Test: "TestPass5"},
			{Package: "pkg/test", Test: "TestPass6"},
			{Package: "pkg/test", Test: "TestPass7"},
		},
		Failed: []types.TestResult{
			{Package: "pkg/test", Test: "TestFail1"},
			{Package: "pkg/test", Test: "TestFail2"},
		},
		Skipped: []types.TestResult{
			{Package: "pkg/test", Test: "TestSkip1", SkipReason: "Not ready"},
		},
		Coverage: "coverage: 80.0% of statements",
	}

	comment := GenerateGitHubComment(summary, "test-uuid")
	lines := strings.Split(comment, "\n")

	// Check general structure order
	var foundStats, foundFailed, foundSkipped, foundCoverage bool
	var statsLine, failedLine, skippedLine, coverageLine int

	for i, line := range lines {
		if strings.Contains(line, "Total:") {
			foundStats = true
			statsLine = i
		}
		if strings.Contains(line, "Failed Tests") {
			foundFailed = true
			failedLine = i
		}
		if strings.Contains(line, "Skipped Tests") {
			foundSkipped = true
			skippedLine = i
		}
		if strings.Contains(line, "Coverage") && strings.Contains(line, "Package") {
			foundCoverage = true
			coverageLine = i
		}
	}

	assert.True(t, foundStats, "Should have stats section")
	assert.True(t, foundFailed, "Should have failed section")
	assert.True(t, foundSkipped, "Should have skipped section")
	assert.True(t, foundCoverage, "Should have coverage section")

	// Check ordering
	if foundStats && foundFailed {
		assert.Less(t, statsLine, failedLine, "Stats should come before failed tests")
	}
	if foundFailed && foundSkipped {
		assert.Less(t, failedLine, skippedLine, "Failed should come before skipped")
	}
	if foundSkipped && foundCoverage {
		assert.Less(t, skippedLine, coverageLine, "Skipped should come before coverage")
	}
}

func TestCoverageTableFormat(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg/test", Test: "TestPass1"},
			{Package: "pkg/test", Test: "TestPass2"},
			{Package: "pkg/test", Test: "TestPass3"},
			{Package: "pkg/test", Test: "TestPass4"},
			{Package: "pkg/test", Test: "TestPass5"},
		},
		Coverage: "coverage: 85.5% of statements in github.com/test/pkg/main\ncoverage: 92.3% of statements in github.com/test/pkg/utils\ncoverage: 0.0% of statements in github.com/test/pkg/config",
	}

	comment := GenerateAdaptiveComment(summary, "test-uuid", "linux")

	// Check table structure
	assert.Contains(t, comment, "| Package")
	assert.Contains(t, comment, "| Coverage")
	assert.Contains(t, comment, "|---")

	// Check coverage values
	assert.Contains(t, comment, "85.5%")
	assert.Contains(t, comment, "92.3%")
	assert.Contains(t, comment, "0.0%")

	// Check package names (should be shortened)
	assert.Contains(t, comment, "pkg/main")
	assert.Contains(t, comment, "pkg/utils")
	assert.Contains(t, comment, "pkg/config")
}
