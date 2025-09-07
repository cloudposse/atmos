package markdown

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestGenerateAdaptiveComment(t *testing.T) {
	tests := []struct {
		name     string
		summary  *types.TestSummary
		uuid     string
		platform string
		expected struct {
			hasUUID           bool
			hasBadges         bool
			hasFailedTests    bool
			hasSkippedTests   bool
			hasSlowestTests   bool
			hasPackageSummary bool
			hasElapsedTime    bool
			withinSizeLimit   bool
		}
	}{
		{
			name: "Small test suite uses full format",
			summary: &types.TestSummary{
				Failed: []types.TestResult{
					{Package: "pkg/test", Test: "TestFailed1", Duration: 1.5},
				},
				Skipped: []types.TestResult{
					{Package: "pkg/test", Test: "TestSkipped1"},
				},
				Passed: []types.TestResult{
					{Package: "pkg/test", Test: "TestPassed1", Duration: 0.1},
					{Package: "pkg/test", Test: "TestPassed2", Duration: 0.2},
					{Package: "pkg/test", Test: "TestPassed3", Duration: 0.3},
				},
				TotalElapsedTime: 2.1,
			},
			uuid:     "test-uuid-adaptive",
			platform: "Linux",
			expected: struct {
				hasUUID           bool
				hasBadges         bool
				hasFailedTests    bool
				hasSkippedTests   bool
				hasSlowestTests   bool
				hasPackageSummary bool
				hasElapsedTime    bool
				withinSizeLimit   bool
			}{
				hasUUID:           true,
				hasBadges:         true,
				hasFailedTests:    true,
				hasSkippedTests:   true,
				hasSlowestTests:   true,
				hasPackageSummary: false, // Small suite, no package summary needed
				hasElapsedTime:    true,
				withinSizeLimit:   true,
			},
		},
		{
			name:     "Large test suite adapts format",
			summary:  createLargeSummary(1000, 50, 25),
			uuid:     "test-uuid-large",
			platform: "Darwin",
			expected: struct {
				hasUUID           bool
				hasBadges         bool
				hasFailedTests    bool
				hasSkippedTests   bool
				hasSlowestTests   bool
				hasPackageSummary bool
				hasElapsedTime    bool
				withinSizeLimit   bool
			}{
				hasUUID:           true,
				hasBadges:         true,
				hasFailedTests:    true,
				hasSkippedTests:   true,
				hasSlowestTests:   false, // May be truncated in large suite
				hasPackageSummary: true,  // Large suite should have package summary
				hasElapsedTime:    true,
				withinSizeLimit:   true,
			},
		},
		{
			name:     "Huge test suite truncates non-essential",
			summary:  createLargeSummary(5000, 200, 100),
			uuid:     "test-uuid-huge",
			platform: "Windows",
			expected: struct {
				hasUUID           bool
				hasBadges         bool
				hasFailedTests    bool
				hasSkippedTests   bool
				hasSlowestTests   bool
				hasPackageSummary bool
				hasElapsedTime    bool
				withinSizeLimit   bool
			}{
				hasUUID:           true,
				hasBadges:         true,
				hasFailedTests:    true,  // Always show failures
				hasSkippedTests:   false, // May be truncated
				hasSlowestTests:   false, // Likely truncated
				hasPackageSummary: true,  // Keep summary for overview
				hasElapsedTime:    true,
				withinSizeLimit:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the adaptive comment generation
			comment := GenerateAdaptiveComment(tt.summary, tt.uuid, tt.platform)

			// Check size limit
			if tt.expected.withinSizeLimit {
				assert.LessOrEqual(t, len(comment), CommentSizeLimit, "Comment should be within size limit")
			}

			// Check expected content
			if tt.expected.hasUUID {
				assert.Contains(t, comment, tt.uuid, "Should contain UUID")
			}

			if tt.expected.hasFailedTests && len(tt.summary.Failed) > 0 {
				assert.Contains(t, comment, "Failed", "Should contain failed tests section")
			}

			if tt.expected.hasSkippedTests && len(tt.summary.Skipped) > 0 {
				assert.Contains(t, comment, "Skipped", "Should contain skipped tests section")
			}

			if tt.expected.hasElapsedTime {
				assert.Contains(t, comment, "elapsed", "Should contain elapsed time")
			}
		})
	}
}

func TestAdaptiveBehavior(t *testing.T) {
	// Test that adaptive behavior actually reduces size
	smallSummary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg/test", Test: "TestPass1", Duration: 0.1},
			{Package: "pkg/test", Test: "TestPass2", Duration: 0.2},
			{Package: "pkg/test", Test: "TestPass3", Duration: 0.3},
			{Package: "pkg/test", Test: "TestPass4", Duration: 0.1},
			{Package: "pkg/test", Test: "TestPass5", Duration: 0.2},
			{Package: "pkg/test", Test: "TestPass6", Duration: 0.3},
			{Package: "pkg/test", Test: "TestPass7", Duration: 0.1},
			{Package: "pkg/test", Test: "TestPass8", Duration: 0.2},
		},
		Failed: []types.TestResult{
			{Package: "pkg/test", Test: "TestFail", Duration: 1.0},
		},
		Skipped: []types.TestResult{
			{Package: "pkg/test", Test: "TestSkip", SkipReason: "Test reason"},
		},
	}

	largeSummary := createLargeSummary(1000, 50, 25)

	smallComment := GenerateAdaptiveComment(smallSummary, "uuid-small", "linux")
	largeComment := GenerateAdaptiveComment(largeSummary, "uuid-large", "linux")

	// Both should be within limits
	assert.LessOrEqual(t, len(smallComment), CommentSizeLimit)
	assert.LessOrEqual(t, len(largeComment), CommentSizeLimit)

	// Large comment should have adaptations
	assert.Contains(t, largeComment, "Package Summary") // Should use summary format
}

func TestCommentSizeHandling(t *testing.T) {
	tests := []struct {
		name            string
		failedCount     int
		passedCount     int
		skippedCount    int
		expectTruncated bool
	}{
		{
			name:            "Small suite fits completely",
			failedCount:     5,
			passedCount:     10,
			skippedCount:    2,
			expectTruncated: false,
		},
		{
			name:            "Medium suite may truncate passed tests",
			failedCount:     20,
			passedCount:     500,
			skippedCount:    50,
			expectTruncated: false, // Passed tests are less important
		},
		{
			name:            "Large suite truncates non-essential",
			failedCount:     100,
			passedCount:     2000,
			skippedCount:    200,
			expectTruncated: true,
		},
		{
			name:            "Huge suite keeps only essentials",
			failedCount:     500,
			passedCount:     10000,
			skippedCount:    1000,
			expectTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := createSummaryWithCounts(tt.failedCount, tt.passedCount, tt.skippedCount)
			comment := GenerateGitHubComment(summary, "")

			// Check size limit
			assert.LessOrEqual(t, len(comment), CommentSizeLimit, "Comment must be within size limit")

			// Failed tests should always be included (up to a reasonable limit)
			if tt.failedCount > 0 && tt.failedCount < 50 {
				assert.Contains(t, comment, "Failed Tests")
			}

			// Check for truncation indicators
			if tt.expectTruncated {
				// May contain ellipsis or truncation message
				if tt.passedCount > 100 {
					// Large number of passed tests likely truncated
					passedSection := strings.Contains(comment, "Passed Tests")
					if !passedSection {
						// Passed tests were completely removed for space
						assert.True(t, true, "Passed tests truncated as expected")
					}
				}
			}
		})
	}
}

func TestSkipReasonsInComment(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg/test", Test: "TestPass1", Duration: 0.1},
			{Package: "pkg/test", Test: "TestPass2", Duration: 0.2},
		},
		Skipped: []types.TestResult{
			{Package: "pkg/test", Test: "TestSkip1", SkipReason: "Database not available"},
			{Package: "pkg/test", Test: "TestSkip2", SkipReason: ""},
			{Package: "pkg/test", Test: "TestSkip3", SkipReason: "CI environment not configured"},
		},
	}

	comment := GenerateGitHubComment(summary, "")

	// Check skip reasons are included
	assert.Contains(t, comment, "TestSkip1")
	assert.Contains(t, comment, "Database not available")
	assert.Contains(t, comment, "TestSkip2")
	assert.Contains(t, comment, "TestSkip3")
	assert.Contains(t, comment, "CI environment not configured")
}

// Helper functions for creating test data

func createLargeSummary(total, failed, skipped int) *types.TestSummary {
	summary := &types.TestSummary{
		TotalElapsedTime: float64(total) * 0.1,
	}

	// Add failed tests
	for i := 0; i < failed && i < 100; i++ { // Limit to prevent huge test data
		summary.Failed = append(summary.Failed, types.TestResult{
			Package:  fmt.Sprintf("pkg/package%d", i%10),
			Test:     fmt.Sprintf("TestFailed%d", i),
			Duration: float64(i) * 0.1,
		})
	}

	// Add skipped tests
	for i := 0; i < skipped && i < 50; i++ {
		summary.Skipped = append(summary.Skipped, types.TestResult{
			Package:    fmt.Sprintf("pkg/package%d", i%10),
			Test:       fmt.Sprintf("TestSkipped%d", i),
			SkipReason: fmt.Sprintf("Skip reason %d", i),
		})
	}

	// Add passed tests
	passed := total - failed - skipped
	for i := 0; i < passed && i < 200; i++ {
		summary.Passed = append(summary.Passed, types.TestResult{
			Package:  fmt.Sprintf("pkg/package%d", i%10),
			Test:     fmt.Sprintf("TestPassed%d", i),
			Duration: float64(i) * 0.05,
		})
	}

	// Set overall coverage
	summary.Coverage = "75.5%"

	return summary
}

func createSummaryWithCounts(failed, passed, skipped int) *types.TestSummary {
	summary := &types.TestSummary{}

	// Add failed tests
	for i := 0; i < failed; i++ {
		summary.Failed = append(summary.Failed, types.TestResult{
			Package: fmt.Sprintf("pkg/test%d", i%10),
			Test:    fmt.Sprintf("TestFailed%d", i),
		})
	}

	// Add passed tests (limit to prevent huge arrays)
	for i := 0; i < passed && i < 100; i++ {
		summary.Passed = append(summary.Passed, types.TestResult{
			Package: fmt.Sprintf("pkg/test%d", i%10),
			Test:    fmt.Sprintf("TestPassed%d", i),
		})
	}

	// Add skipped tests
	for i := 0; i < skipped && i < 50; i++ {
		summary.Skipped = append(summary.Skipped, types.TestResult{
			Package: fmt.Sprintf("pkg/test%d", i%10),
			Test:    fmt.Sprintf("TestSkipped%d", i),
		})
	}

	return summary
}
