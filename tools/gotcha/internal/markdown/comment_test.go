package markdown

import (
	"fmt"
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
				"pkg/main",
				"85.5%",
				"pkg/utils",
				"92.3%",
				"pkg/config",
				"78.0%",
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
				"No tests",
			},
		},
		{
			name: "Large test suite with many packages",
			summary: &types.TestSummary{
				Passed:  make([]types.TestResult, 95),
				Failed:  make([]types.TestResult, 3),
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
				"100",       // Total tests
				"95",        // Passed count
				"3",         // Failed count
				"TestFail1", // Failed test names
				"TestFail2",
				"TestFail3",
				"pkg/package1", // Package names
				"pkg/package2",
				"pkg/package3",
				"pkg/package4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := GenerateGitHubComment(tt.summary)

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

func TestTruncateToEssentials(t *testing.T) {
	builder := &CommentBuilder{
		sections: make(map[string]string),
	}

	// Add various sections
	builder.addSection("header", "# Test Results")
	builder.addSection("stats", strings.Repeat("Statistics ", 50))
	builder.addSection("failed", strings.Repeat("Failed test ", 100))
	builder.addSection("passed", strings.Repeat("Passed test ", 200))
	builder.addSection("coverage", strings.Repeat("Coverage data ", 50))

	// Truncate to a small limit
	builder.truncateToEssentials(500)

	// Essential sections should be preserved
	assert.Contains(t, builder.sections, "header")
	assert.Contains(t, builder.sections, "stats")
	assert.Contains(t, builder.sections, "failed")

	// Non-essential sections might be removed
	result := builder.String()
	assert.Less(t, len(result), 600) // Some buffer for section ordering
}

func TestAddPassedTestsWithLimit(t *testing.T) {
	builder := &CommentBuilder{
		sections: make(map[string]string),
	}

	// Create many passed tests
	var passed []types.TestResult
	for i := 0; i < 100; i++ {
		passed = append(passed, types.TestResult{
			Package: "pkg/test",
			Test:    fmt.Sprintf("TestPassed%d", i),
		})
	}

	// Add with limit
	builder.addPassedTests(passed, 1000)

	result := builder.sections["passed"]
	assert.NotEmpty(t, result)

	// Should contain truncation message when limited
	if strings.Contains(result, "...") {
		assert.Less(t, len(result), 1100) // Some buffer
	}
}

func TestAddCoverageWithLimit(t *testing.T) {
	builder := &CommentBuilder{
		sections: make(map[string]string),
	}

	// Create coverage data
	coverage := []types.CoverageInfo{
		{Package: "pkg/main", Percentage: 85.5},
		{Package: "pkg/utils", Percentage: 92.3},
		{Package: "pkg/config", Percentage: 78.0},
		{Package: "pkg/handler", Percentage: 88.5},
		{Package: "pkg/middleware", Percentage: 95.0},
	}

	// Add with limit
	builder.addCoverage(coverage, 500)

	result := builder.sections["coverage"]
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Coverage")
	assert.Contains(t, result, "85.5%")

	// Check table format
	assert.Contains(t, result, "|")
	assert.Contains(t, result, "Package")
	assert.Contains(t, result, "Coverage")
}

func TestPlatformInHeader(t *testing.T) {
	tests := []struct {
		platform string
		expected string
	}{
		{"linux", "🐧"},
		{"darwin", "🍎"},
		{"windows", "🪟"},
		{"unknown", "💻"},
		{"", "💻"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			summary := &types.TestSummary{
				Total:   1,
				Passed:  1,
				Runtime: tt.platform,
			}

			comment := GenerateGitHubComment(summary)
			assert.Contains(t, comment, tt.expected)
		})
	}
}

func TestCommentStructureOrdering(t *testing.T) {
	summary := &types.TestSummary{
		Total:   10,
		Passed:  7,
		Failed:  2,
		Skipped: 1,
		Runtime: "linux",
		Failed: []types.TestResult{
			{Package: "pkg/test", Test: "TestFail1"},
			{Package: "pkg/test", Test: "TestFail2"},
		},
		Skipped: []types.TestResult{
			{Package: "pkg/test", Test: "TestSkip1", SkipReason: "Not ready"},
		},
		Coverage: []types.CoverageInfo{
			{Package: "pkg/test", Percentage: 80.0},
		},
	}

	comment := GenerateGitHubComment(summary)
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
		Total:   5,
		Passed:  5,
		Runtime: "linux",
		Coverage: []types.CoverageInfo{
			{Package: "github.com/test/pkg/main", Percentage: 85.5},
			{Package: "github.com/test/pkg/utils", Percentage: 92.3},
			{Package: "github.com/test/pkg/config", Percentage: 0.0},
		},
	}

	comment := GenerateGitHubComment(summary)

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
