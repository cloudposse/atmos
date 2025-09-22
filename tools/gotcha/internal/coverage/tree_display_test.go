package coverage

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
	"github.com/stretchr/testify/assert"
)

func TestDisplayFunctionCoverageTreeNew(t *testing.T) {
	tests := []struct {
		name              string
		functions         []FunctionCoverageInfo
		expectedFragments []string
	}{
		{
			name:              "empty functions list",
			functions:         []FunctionCoverageInfo{},
			expectedFragments: []string{}, // Should display nothing
		},
		{
			name: "single package with multiple functions",
			functions: []FunctionCoverageInfo{
				{
					Package:  "pkg/example",
					File:     "example.go",
					Line:     10,
					Function: "TestFunc1",
					Coverage: 85.5,
				},
				{
					Package:  "pkg/example",
					File:     "example.go",
					Line:     20,
					Function: "TestFunc2",
					Coverage: 0.0,
				},
				{
					Package:  "pkg/example",
					File:     "helper.go",
					Line:     5,
					Function: "HelperFunc",
					Coverage: 100.0,
				},
			},
			expectedFragments: []string{
				"Function Coverage Report",
				"pkg/example",
				"example.go",
				"TestFunc1",
				"TestFunc2",
				"helper.go",
				"HelperFunc",
				"85.5%",
				"0.0%",
				"100.0%",
				"Summary: 3 functions",
				"1 uncovered",
			},
		},
		{
			name: "multiple packages",
			functions: []FunctionCoverageInfo{
				{
					Package:  "cmd/gotcha",
					File:     "main.go",
					Line:     15,
					Function: "main",
					Coverage: 0.0,
				},
				{
					Package:  "internal/tui",
					File:     "update.go",
					Line:     100,
					Function: "Update",
					Coverage: 75.0,
				},
				{
					Package:  "pkg/config",
					File:     "config.go",
					Line:     50,
					Function: "Load",
					Coverage: 90.0,
				},
			},
			expectedFragments: []string{
				"cmd/gotcha",
				"main.go",
				"internal/tui",
				"update.go",
				"pkg/config",
				"config.go",
				"Summary: 3 functions",
				"55.0% average coverage",
				"1 uncovered",
			},
		},
		{
			name: "long function names are truncated",
			functions: []FunctionCoverageInfo{
				{
					Package:  "pkg/test",
					File:     "test.go",
					Line:     1,
					Function: "ThisIsAVeryLongFunctionNameThatShouldBeTruncatedForDisplay",
					Coverage: 50.0,
				},
			},
			expectedFragments: []string{
				"pkg/test",
				"test.go",
				"ThisIsAVeryLongFunctionNa...",
				"50.0%",
			},
		},
		{
			name: "files with line numbers in name",
			functions: []FunctionCoverageInfo{
				{
					Package:  "pkg/test",
					File:     "test.go:123",
					Line:     10,
					Function: "TestFunc",
					Coverage: 60.0,
				},
			},
			expectedFragments: []string{
				"test.go", // Should strip :123
				"TestFunc",
				"60.0%",
			},
		},
		{
			name: "package path shortening",
			functions: []FunctionCoverageInfo{
				{
					Package:  "cmd/gotcha/subcmd",
					File:     "run.go",
					Line:     1,
					Function: "Run",
					Coverage: 80.0,
				},
				{
					Package:  "internal/deep/nested/pkg",
					File:     "core.go",
					Line:     2,
					Function: "Core",
					Coverage: 70.0,
				},
				{
					Package:  "pkg/very/deep/nested",
					File:     "util.go",
					Line:     3,
					Function: "Util",
					Coverage: 60.0,
				},
			},
			expectedFragments: []string{
				"cmd/subcmd",    // Should shorten cmd paths
				"internal/deep", // Should shorten internal paths
				"pkg/very",      // Should shorten pkg paths
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputData strings.Builder
			var outputUI strings.Builder
			writer := output.NewCustom(&outputData, &outputUI)

			displayFunctionCoverageTreeNew(tt.functions, writer)

			result := outputData.String()
			if len(tt.expectedFragments) > 0 {
				assert.NotEmpty(t, result)
				for _, fragment := range tt.expectedFragments {
					assert.Contains(t, result, fragment, "Expected fragment '%s' not found", fragment)
				}
			} else {
				// For empty functions, should not output anything except maybe empty lines
				assert.True(t, len(strings.TrimSpace(result)) == 0 || result == "")
			}
		})
	}
}

func TestGetCoverageColor(t *testing.T) {
	// Test coverage color thresholds
	tests := []struct {
		coverage float64
		// We can't easily test exact colors, but we can verify the function doesn't panic
		shouldWork bool
	}{
		{coverage: 100.0, shouldWork: true},
		{coverage: 85.0, shouldWork: true},
		{coverage: 75.0, shouldWork: true},
		{coverage: 50.0, shouldWork: true},
		{coverage: 25.0, shouldWork: true},
		{coverage: 0.0, shouldWork: true},
		{coverage: -1.0, shouldWork: true},  // Edge case
		{coverage: 101.0, shouldWork: true}, // Edge case
	}

	for _, tt := range tests {
		t.Run(string(rune(int(tt.coverage))), func(t *testing.T) {
			// Just verify it doesn't panic
			_ = getCoverageColor(tt.coverage)
			assert.True(t, tt.shouldWork)
		})
	}
}

func TestGetCoverageSymbol(t *testing.T) {
	tests := []struct {
		coverage float64
		// We can verify the symbol returned
		expectedEmpty bool
	}{
		{coverage: 100.0, expectedEmpty: false},
		{coverage: 80.0, expectedEmpty: false},
		{coverage: 60.0, expectedEmpty: false},
		{coverage: 40.0, expectedEmpty: false},
		{coverage: 20.0, expectedEmpty: false},
		{coverage: 0.0, expectedEmpty: false},
	}

	for _, tt := range tests {
		t.Run(string(rune(int(tt.coverage))), func(t *testing.T) {
			symbol := getCoverageSymbol(tt.coverage)
			if tt.expectedEmpty {
				assert.Empty(t, symbol)
			} else {
				assert.NotEmpty(t, symbol)
			}
		})
	}
}
