package markdown

import (
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
			uuid: "test-uuid-adaptive",
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
				hasSlowestTests:   true,  // Should include slowest tests
				hasPackageSummary: true,  // Should include package summary
				hasElapsedTime:    true,  // Should include elapsed time
				withinSizeLimit:   true,
			},
		},
		{
			name: "Empty test results still gets full format",
			summary: &types.TestSummary{
				Failed:           []types.TestResult{},
				Skipped:          []types.TestResult{},
				Passed:           []types.TestResult{},
				TotalElapsedTime: 0,
			},
			uuid: "test-uuid-empty",
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
				hasFailedTests:    false,
				hasSkippedTests:   false,
				hasSlowestTests:   false,
				hasPackageSummary: false,
				hasElapsedTime:    false,
				withinSizeLimit:   true,
			},
		},
		{
			name: "Medium test suite with all features",
			summary: &types.TestSummary{
				Failed: []types.TestResult{
					{Package: "pkg/utils", Test: "TestFailed1", Duration: 1.5},
					{Package: "pkg/utils", Test: "TestFailed2", Duration: 2.0},
				},
				Skipped: []types.TestResult{
					{Package: "pkg/core", Test: "TestSkipped1"},
					{Package: "pkg/core", Test: "TestSkipped2"},
				},
				Passed: generateTestsWithPackages(50),
				TotalElapsedTime: 125.5,
				Coverage:         "75.5%",
			},
			uuid: "test-uuid-medium",
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
				hasPackageSummary: true,
				hasElapsedTime:    true,
				withinSizeLimit:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateAdaptiveComment(tt.summary, tt.uuid)

			// Check UUID presence
			if tt.expected.hasUUID {
				assert.Contains(t, result, "test-summary-uuid: "+tt.uuid, "Comment should contain UUID marker")
			}

			// Check badges presence
			if tt.expected.hasBadges {
				assert.Contains(t, result, "shields.io/badge", "Comment should contain test badges")
			}

			// Check failed tests
			if tt.expected.hasFailedTests {
				assert.Contains(t, result, "Failed Tests", "Comment should contain failed tests section")
			}

			// Check skipped tests
			if tt.expected.hasSkippedTests {
				assert.Contains(t, result, "Skipped Tests", "Comment should contain skipped tests section")
			}

			// Check slowest tests (NEW)
			if tt.expected.hasSlowestTests {
				assert.Contains(t, result, "⏱️ Slowest Tests", "Comment should contain slowest tests section")
			}

			// Check package summary (NEW)
			if tt.expected.hasPackageSummary {
				assert.Contains(t, result, "📦 Package Summary", "Comment should contain package summary section")
			}

			// Check elapsed time (NEW)
			if tt.expected.hasElapsedTime {
				assert.Contains(t, result, "Total Time:", "Comment should contain total elapsed time")
			}

			// Check size limit
			if tt.expected.withinSizeLimit {
				assert.LessOrEqual(t, len(result), CommentSizeLimit, "Comment should be within GitHub's size limit")
			}
		})
	}
}

// TestAdaptiveBehavior tests the adaptive fallback behavior
func TestAdaptiveBehavior(t *testing.T) {
	// Create a large test summary that would exceed the size limit
	hugeSummary := &types.TestSummary{
		Failed:  generateManyTests(500),
		Skipped: generateManyTests(500),
		Passed:  generateManyTests(2000),
		CoverageData: &types.CoverageData{
			StatementCoverage: "85.5%",
			FunctionCoverage:  generateManyFunctions(100),
		},
	}

	result := GenerateAdaptiveComment(hugeSummary, "size-test-uuid")

	// Should fall back to concise format
	assert.LessOrEqual(t, len(result), CommentSizeLimit,
		"Large comment should be within size limit due to adaptive fallback")
	
	// Should still have essential information
	assert.Contains(t, result, "test-summary-uuid: size-test-uuid", "Should have UUID")
	assert.Contains(t, result, "PASSED-2000", "Should have pass count")
	assert.Contains(t, result, "FAILED-500", "Should have fail count")
	assert.Contains(t, result, "SKIPPED-500", "Should have skip count")
	
	// Concise version should NOT have these sections
	assert.NotContains(t, result, "⏱️ Slowest Tests", "Concise version should not have slowest tests")
	assert.NotContains(t, result, "📦 Package Summary", "Concise version should not have package summary")
}

// TestGenerateGitHubComment tests backward compatibility
func TestGenerateGitHubComment(t *testing.T) {
	tests := []struct {
		name     string
		summary  *types.TestSummary
		uuid     string
		expected struct {
			hasUUID         bool
			hasBadges       bool
			hasFailedTests  bool
			hasSkippedTests bool
			withinSizeLimit bool
			preservesCore   bool
		}
	}{
		{
			name: "Empty test results",
			summary: &types.TestSummary{
				Failed:  []types.TestResult{},
				Skipped: []types.TestResult{},
				Passed:  []types.TestResult{},
			},
			uuid: "test-uuid-123",
			expected: struct {
				hasUUID         bool
				hasBadges       bool
				hasFailedTests  bool
				hasSkippedTests bool
				withinSizeLimit bool
				preservesCore   bool
			}{
				hasUUID:         true,
				hasBadges:       true,
				hasFailedTests:  false,
				hasSkippedTests: false,
				withinSizeLimit: true,
				preservesCore:   true,
			},
		},
		{
			name: "Small number of tests",
			summary: &types.TestSummary{
				Failed: []types.TestResult{
					{Package: "pkg/test", Test: "TestFailed1", Duration: 1.5},
					{Package: "pkg/test", Test: "TestFailed2", Duration: 2.0},
				},
				Skipped: []types.TestResult{
					{Package: "pkg/test", Test: "TestSkipped1"},
				},
				Passed: []types.TestResult{
					{Package: "pkg/test", Test: "TestPassed1", Duration: 0.1},
					{Package: "pkg/test", Test: "TestPassed2", Duration: 0.2},
				},
			},
			uuid: "test-uuid-456",
			expected: struct {
				hasUUID         bool
				hasBadges       bool
				hasFailedTests  bool
				hasSkippedTests bool
				withinSizeLimit bool
				preservesCore   bool
			}{
				hasUUID:         true,
				hasBadges:       true,
				hasFailedTests:  true,
				hasSkippedTests: true,
				withinSizeLimit: true,
				preservesCore:   true,
			},
		},
		{
			name: "Large number of passed tests should be limited",
			summary: &types.TestSummary{
				Failed:  []types.TestResult{},
				Skipped: []types.TestResult{},
				Passed:  generateManyTests(1000), // This should trigger size limiting
			},
			uuid: "test-uuid-789",
			expected: struct {
				hasUUID         bool
				hasBadges       bool
				hasFailedTests  bool
				hasSkippedTests bool
				withinSizeLimit bool
				preservesCore   bool
			}{
				hasUUID:         true,
				hasBadges:       true,
				hasFailedTests:  false,
				hasSkippedTests: false,
				withinSizeLimit: true,
				preservesCore:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateGitHubComment(tt.summary, tt.uuid)

			// Check UUID presence
			if tt.expected.hasUUID {
				assert.Contains(t, result, tt.uuid, "Comment should contain UUID")
			}

			// Check badges presence
			if tt.expected.hasBadges {
				assert.Contains(t, result, "shields.io/badge", "Comment should contain badges")
			}

			// Check failed tests section
			if tt.expected.hasFailedTests {
				assert.Contains(t, result, "❌ Failed Tests", "Comment should contain failed tests section")
			}

			// Check skipped tests section
			if tt.expected.hasSkippedTests {
				assert.Contains(t, result, "⏭️ Skipped Tests", "Comment should contain skipped tests section")
			}

			// Check size limit
			if tt.expected.withinSizeLimit {
				assert.LessOrEqual(t, len(result), CommentSizeLimit,
					"Comment should be within GitHub size limit (got %d bytes)", len(result))
			}

			// Check core content preservation
			if tt.expected.preservesCore {
				assert.Contains(t, result, "# Test Results", "Comment should preserve core test results header")
				if len(tt.summary.Failed) > 0 || len(tt.summary.Skipped) > 0 || len(tt.summary.Passed) > 0 {
					assert.Contains(t, result, "PASSED", "Comment should preserve passed badge")
				}
			}
		})
	}
}

func TestTruncateToEssentials(t *testing.T) {
	summary := &types.TestSummary{
		Failed:  generateManyTests(50),  // Many failed tests
		Skipped: generateManyTests(30),  // Many skipped tests
		Passed:  generateManyTests(100), // Many passed tests
	}

	result := truncateToEssentials(summary, "test-uuid")

	// Should be much smaller than full content
	assert.LessOrEqual(t, len(result), 10000, "Essential truncation should be quite small")

	// Should preserve core elements
	assert.Contains(t, result, "test-uuid", "Should contain UUID")
	assert.Contains(t, result, "# Test Results", "Should contain main header")
	assert.Contains(t, result, "shields.io/badge", "Should contain badges")
	assert.Contains(t, result, "❌ Failed Tests", "Should contain failed tests section")
	assert.Contains(t, result, "⏭️ Skipped Tests", "Should contain skipped tests section")
	assert.Contains(t, result, "Full test results available", "Should indicate more info available")

	// Should limit number of tests shown
	failedCount := strings.Count(result, "TestGenerated")
	assert.LessOrEqual(t, failedCount, 15, "Should limit number of tests shown (failed + skipped)")
}

func TestAddPassedTestsWithLimit(t *testing.T) {
	tests := []struct {
		name        string
		passed      []types.TestResult
		maxBytes    int
		expectTests bool
	}{
		{
			name:        "No space for tests",
			passed:      generateManyTests(10),
			maxBytes:    100, // Too small
			expectTests: false,
		},
		{
			name:        "Some space for tests",
			passed:      generateManyTests(100),
			maxBytes:    2000, // Reasonable space
			expectTests: true,
		},
		{
			name:        "Plenty of space",
			passed:      generateManyTests(5),
			maxBytes:    10000, // Lots of space
			expectTests: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result strings.Builder
			addPassedTestsWithLimit(&result, tt.passed, tt.maxBytes)
			content := result.String()

			if tt.expectTests {
				assert.Contains(t, content, "✅ Passed Tests", "Should contain passed tests section")
			} else {
				assert.NotContains(t, content, "✅ Passed Tests", "Should not contain passed tests section when no space")
			}

			// Should always be within the byte limit (with some tolerance for estimates)
			assert.LessOrEqual(t, len(content), tt.maxBytes+500, "Should roughly respect byte limit")
		})
	}
}

func TestAddCoverageWithLimit(t *testing.T) {
	tests := []struct {
		name           string
		summary        *types.TestSummary
		maxBytes       int
		expectCoverage bool
	}{
		{
			name: "No space for coverage",
			summary: &types.TestSummary{
				Coverage: "85.2%",
			},
			maxBytes:       100, // Too small
			expectCoverage: false,
		},
		{
			name: "Space for coverage",
			summary: &types.TestSummary{
				CoverageData: &types.CoverageData{
					StatementCoverage: "78.5%",
				},
			},
			maxBytes:       1000, // Reasonable space
			expectCoverage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result strings.Builder
			addCoverageWithLimit(&result, tt.summary, tt.maxBytes)
			content := result.String()

			if tt.expectCoverage {
				assert.Contains(t, content, "📊 Test Coverage", "Should contain coverage section")
			} else {
				assert.NotContains(t, content, "📊 Test Coverage", "Should not contain coverage section when no space")
			}

			// Should be within the byte limit
			assert.LessOrEqual(t, len(content), tt.maxBytes+100, "Should roughly respect byte limit")
		})
	}
}

func TestCommentSizeHandling(t *testing.T) {
	// Create a summary that would normally exceed the size limit
	hugeSummary := &types.TestSummary{
		Failed:  generateManyTests(200), // Many failed tests
		Skipped: generateManyTests(100), // Many skipped tests
		Passed:  generateManyTests(500), // Many passed tests
		CoverageData: &types.CoverageData{
			StatementCoverage: "85.2%",
			FunctionCoverage:  generateManyCoverageFunctions(100),
		},
	}

	result := GenerateGitHubComment(hugeSummary, "size-test-uuid")

	// Should be within the size limit
	assert.LessOrEqual(t, len(result), CommentSizeLimit,
		"Even huge summaries should be constrained to size limit (got %d bytes)", len(result))

	// Should still preserve core elements
	assert.Contains(t, result, "size-test-uuid", "Should contain UUID")
	assert.Contains(t, result, "# Test Results", "Should contain main header")
	assert.Contains(t, result, "shields.io/badge", "Should contain badges")
	assert.Contains(t, result, "❌ Failed Tests", "Should contain failed tests section")
}

// Helper function to generate many test results for testing
func generateManyTests(count int) []types.TestResult {
	tests := make([]types.TestResult, count)
	for i := 0; i < count; i++ {
		tests[i] = types.TestResult{
			Package:  "github.com/example/package/very/long/path/name",
			Test:     "TestGeneratedTestWithVeryLongNameThatTakesUpSpace" + string(rune('A'+i%26)),
			Duration: float64(i%10) * 0.1,
		}
	}
	return tests
}

// generateTestsWithPackages generates tests distributed across multiple packages
func generateTestsWithPackages(count int) []types.TestResult {
	packages := []string{
		"github.com/cloudposse/atmos/pkg/utils",
		"github.com/cloudposse/atmos/pkg/config",
		"github.com/cloudposse/atmos/pkg/stack",
		"github.com/cloudposse/atmos/internal/exec",
		"github.com/cloudposse/atmos/cmd",
	}
	
	tests := make([]types.TestResult, count)
	for i := 0; i < count; i++ {
		tests[i] = types.TestResult{
			Package:  packages[i%len(packages)],
			Test:     "TestGenerated" + string(rune('A'+i%26)),
			Duration: float64(i%20) * 0.5, // Varying durations
		}
	}
	return tests
}

// generateManyFunctions generates many coverage functions for testing
func generateManyFunctions(count int) []types.CoverageFunction {
	functions := make([]types.CoverageFunction, count)
	for i := 0; i < count; i++ {
		functions[i] = types.CoverageFunction{
			File:     "pkg/test/file" + string(rune('A'+i%26)) + ".go",
			Function: "Function" + string(rune('A'+i%26)),
			Coverage: float64(i%2) * 100, // Some covered, some not
		}
	}
	return functions
}

// Helper function to generate many coverage functions for testing
func generateManyCoverageFunctions(count int) []types.CoverageFunction {
	functions := make([]types.CoverageFunction, count)
	for i := 0; i < count; i++ {
		functions[i] = types.CoverageFunction{
			File:     "github.com/example/package/very/long/file/path.go",
			Function: "VeryLongFunctionNameThatTakesUpSpaceInTheComment" + string(rune('A'+i%26)),
			Coverage: float64(i % 100),
		}
	}
	return functions
}

func TestCoverageTableFormat(t *testing.T) {
	summary := &types.TestSummary{
		Failed:  []types.TestResult{},
		Skipped: []types.TestResult{},
		Passed:  []types.TestResult{},
		CoverageData: &types.CoverageData{
			StatementCoverage: "85.2%",
			FunctionCoverage: []types.CoverageFunction{
				{File: "file1.go", Function: "func1", Coverage: 100.0},
				{File: "file2.go", Function: "func2", Coverage: 0.0},
			},
			FilteredFiles: []string{"mock1.go", "mock2.go"},
		},
	}

	result := GenerateGitHubComment(summary, "test-uuid")

	// Should contain coverage table headers
	assert.Contains(t, result, "## 📊 Test Coverage", "Comment should contain coverage section")
	assert.Contains(t, result, "| Metric | Coverage | Details |", "Comment should contain table headers")
	assert.Contains(t, result, "| Statement Coverage |", "Comment should contain statement coverage row")
	assert.Contains(t, result, "| Function Coverage |", "Comment should contain function coverage row")

	// Should contain coverage percentage
	assert.Contains(t, result, "85.2%", "Comment should contain statement coverage percentage")

	// Should contain emoji and excluded files info
	assert.Contains(t, result, "🟢", "Comment should contain coverage emoji for good coverage")
	assert.Contains(t, result, "(excluded 2 mock files)", "Comment should show excluded files count")

	// Should contain function coverage info
	assert.Contains(t, result, "1/2 functions covered", "Comment should show function coverage ratio")
}
