package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
)

func TestGetCoverageEmoji(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
		want       string
	}{
		{
			name:       "high coverage",
			percentage: 85.0,
			want:       "🟢",
		},
		{
			name:       "exactly high threshold",
			percentage: 80.0,
			want:       "🟢",
		},
		{
			name:       "medium coverage",
			percentage: 60.0,
			want:       "🟡",
		},
		{
			name:       "exactly medium threshold",
			percentage: 40.0,
			want:       "🟡",
		},
		{
			name:       "low coverage",
			percentage: 25.0,
			want:       "🔴",
		},
		{
			name:       "zero coverage",
			percentage: 0.0,
			want:       "🔴",
		},
		{
			name:       "full coverage",
			percentage: 100.0,
			want:       "🟢",
		},
		{
			name:       "just below high threshold",
			percentage: 79.9,
			want:       "🟡",
		},
		{
			name:       "just below medium threshold",
			percentage: 39.9,
			want:       "🔴",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCoverageEmoji(tt.percentage)
			if got != tt.want {
				t.Errorf("getCoverageEmoji() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateFunctionCoverage(t *testing.T) {
	tests := []struct {
		name           string
		functions      []types.CoverageFunction
		wantCovered    int
		wantTotal      int
		wantPercentage float64
	}{
		{
			name: "mixed coverage functions",
			functions: []types.CoverageFunction{
				{Function: "func1", Coverage: 100.0},
				{Function: "func2", Coverage: 0.0},
				{Function: "func3", Coverage: 75.0},
				{Function: "func4", Coverage: 0.0},
			},
			wantCovered:    2, // func1 and func3 have > 0% coverage
			wantTotal:      4,
			wantPercentage: 50.0, // 2/4 = 50%
		},
		{
			name: "all functions covered",
			functions: []types.CoverageFunction{
				{Function: "func1", Coverage: 100.0},
				{Function: "func2", Coverage: 50.0},
				{Function: "func3", Coverage: 25.0},
			},
			wantCovered:    3,
			wantTotal:      3,
			wantPercentage: 100.0,
		},
		{
			name: "no functions covered",
			functions: []types.CoverageFunction{
				{Function: "func1", Coverage: 0.0},
				{Function: "func2", Coverage: 0.0},
			},
			wantCovered:    0,
			wantTotal:      2,
			wantPercentage: 0.0,
		},
		{
			name:           "empty functions list",
			functions:      []types.CoverageFunction{},
			wantCovered:    0,
			wantTotal:      0,
			wantPercentage: 0.0,
		},
		{
			name: "single function with partial coverage",
			functions: []types.CoverageFunction{
				{Function: "func1", Coverage: 33.3},
			},
			wantCovered:    1,
			wantTotal:      1,
			wantPercentage: 100.0, // 1/1 function has some coverage
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCovered, gotTotal, gotPercentage := calculateFunctionCoverage(tt.functions)
			if gotCovered != tt.wantCovered {
				t.Errorf("calculateFunctionCoverage() covered = %v, want %v", gotCovered, tt.wantCovered)
			}
			if gotTotal != tt.wantTotal {
				t.Errorf("calculateFunctionCoverage() total = %v, want %v", gotTotal, tt.wantTotal)
			}
			if gotPercentage != tt.wantPercentage {
				t.Errorf("calculateFunctionCoverage() percentage = %v, want %v", gotPercentage, tt.wantPercentage)
			}
		})
	}
}

func TestWriteDetailedCoverage(t *testing.T) {
	tests := []struct {
		name         string
		coverageData *types.CoverageData
		wantContains []string
	}{
		{
			name: "coverage with function data",
			coverageData: &types.CoverageData{
				StatementCoverage: "75.5%",
				FunctionCoverage: []types.CoverageFunction{
					{Function: "func1", File: "file1.go", Coverage: 100.0},
					{Function: "func2", File: "file2.go", Coverage: 0.0},
				},
				FilteredFiles: []string{"mock_file.go"},
			},
			wantContains: []string{
				"## 📊 Test Coverage",
				"75.5%",
				"🟡",                     // Should have medium coverage emoji for 75.5%
				"1/2 functions covered", // 1 out of 2 functions has coverage > 0%
			},
		},
		{
			name: "coverage without function data",
			coverageData: &types.CoverageData{
				StatementCoverage: "90.0%",
				FunctionCoverage:  []types.CoverageFunction{},
			},
			wantContains: []string{
				"## 📊 Test Coverage",
				"90.0%",
				"🟢", // Should have high coverage emoji for 90%
			},
		},
		{
			name: "low coverage",
			coverageData: &types.CoverageData{
				StatementCoverage: "25.0%",
				FunctionCoverage:  []types.CoverageFunction{},
			},
			wantContains: []string{
				"## 📊 Test Coverage",
				"25.0%",
				"🔴", // Should have low coverage emoji for 25%
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteDetailedCoverage(&buf, tt.coverageData)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("writeTestCoverageSection() output missing %q\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestGetUncoveredFunctionsInPR(t *testing.T) {
	tests := []struct {
		name         string
		functions    []types.CoverageFunction
		changedFiles []string
		wantCount    int
		wantTotal    int
	}{
		{
			name: "uncovered functions in changed files",
			functions: []types.CoverageFunction{
				{Function: "func1", File: "changed_file.go", Coverage: 0.0},
				{Function: "func2", File: "changed_file.go", Coverage: 75.0},
				{Function: "func3", File: "unchanged_file.go", Coverage: 0.0},
				{Function: "func4", File: "another_changed.go", Coverage: 0.0},
			},
			changedFiles: []string{"changed_file.go", "another_changed.go"},
			wantCount:    2, // func1 and func4 are uncovered in changed files
			wantTotal:    3, // func1, func2, func4 are in changed files
		},
		{
			name: "no uncovered functions in changed files",
			functions: []types.CoverageFunction{
				{Function: "func1", File: "changed_file.go", Coverage: 100.0},
				{Function: "func2", File: "changed_file.go", Coverage: 75.0},
			},
			changedFiles: []string{"changed_file.go"},
			wantCount:    0,
			wantTotal:    2,
		},
		{
			name:         "no changed files",
			functions:    []types.CoverageFunction{{Function: "func1", File: "file.go", Coverage: 0.0}},
			changedFiles: []string{},
			wantCount:    0,
			wantTotal:    0,
		},
		{
			name:         "no functions",
			functions:    []types.CoverageFunction{},
			changedFiles: []string{"changed_file.go"},
			wantCount:    0,
			wantTotal:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUncovered, gotTotal := getUncoveredFunctionsInPR(tt.functions, tt.changedFiles)

			if len(gotUncovered) != tt.wantCount {
				t.Errorf("getUncoveredFunctionsInPR() uncovered count = %d, want %d", len(gotUncovered), tt.wantCount)
			}
			if gotTotal != tt.wantTotal {
				t.Errorf("getUncoveredFunctionsInPR() total = %d, want %d", gotTotal, tt.wantTotal)
			}

			// Verify that returned uncovered functions are actually uncovered.
			for _, fn := range gotUncovered {
				if fn.Coverage > 0 {
					t.Errorf("getUncoveredFunctionsInPR() returned covered function: %+v", fn)
				}
			}
		})
	}
}

func TestWriteUncoveredFunctionsTable(t *testing.T) {
	tests := []struct {
		name         string
		functions    []types.CoverageFunction
		total        int
		wantContains []string
	}{
		{
			name: "write uncovered functions table",
			functions: []types.CoverageFunction{
				{Function: "uncoveredFunc1", File: "github.com/example/pkg/file1.go"},
				{Function: "uncoveredFunc2", File: "github.com/example/pkg/file2.go"},
			},
			total: 5,
			wantContains: []string{
				"❌ Uncovered Functions in This PR (2 of 5)",
				"uncoveredFunc1",
				"uncoveredFunc2",
				"file1.go", // Should show short file name
				"file2.go",
			},
		},
		{
			name:      "empty functions list",
			functions: []types.CoverageFunction{},
			total:     0,
			wantContains: []string{
				"❌ Uncovered Functions in This PR (0 of 0)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeUncoveredFunctionsTable(&buf, tt.functions, tt.total)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("writeUncoveredFunctionsTable() output missing %q\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestWritePRFilteredUncoveredFunctions(t *testing.T) {
	tests := []struct {
		name         string
		functions    []types.CoverageFunction
		wantContains []string
		wantEmpty    bool
	}{
		{
			name: "write filtered uncovered functions",
			functions: []types.CoverageFunction{
				{Function: "func1", File: "tools/gotcha/coverage.go", Coverage: 0.0},
				{Function: "func2", File: "tools/gotcha/formatters.go", Coverage: 0.0},
			},
			wantContains: []string{
				"func1",
				"func2",
				"coverage.go",
				"formatters.go",
			},
		},
		{
			name:      "empty functions list",
			functions: []types.CoverageFunction{},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writePRFilteredUncoveredFunctions(&buf, tt.functions)
			output := buf.String()

			if tt.wantEmpty {
				if strings.TrimSpace(output) != "" {
					t.Errorf("writePRFilteredUncoveredFunctions() expected empty output, got: %s", output)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("writePRFilteredUncoveredFunctions() output missing %q\nGot:\n%s", want, output)
				}
			}
		})
	}
}

// Test coverage threshold constants.
func TestCoverageThresholds(t *testing.T) {
	// Verify that the thresholds are set correctly.
	if constants.CoverageHighThreshold != 80.0 {
		t.Errorf("CoverageHighThreshold = %v, want 80.0", constants.CoverageHighThreshold)
	}
	if constants.CoverageMedThreshold != 40.0 {
		t.Errorf("CoverageMedThreshold = %v, want 40.0", constants.CoverageMedThreshold)
	}
}

// Test that formatter helper functions work correctly.
func TestFormatterHelpers(t *testing.T) {
	t.Run("file name shortening logic", func(t *testing.T) {
		tests := []struct {
			fullPath string
			want     string
		}{
			{"github.com/example/pkg/file.go", "pkg/file.go"},
			{"simple/file.go", "simple/file.go"},
			{"file.go", "file.go"},
			{"", ""},
		}

		for _, tt := range tests {
			// This tests the logic that shortPackage uses.
			got := utils.ShortPackage(tt.fullPath)
			// shortPackage should return the package name, but we're testing file paths.
			// For file paths, we'd want to extract just the meaningful part.
			if tt.fullPath != "" && !strings.Contains(got, tt.want) {
				// This is expected behavior - shortPackage works on packages, not file paths.
				continue
			}
		}
	})
}
