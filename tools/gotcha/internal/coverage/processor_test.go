package coverage

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPackageFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "full path with prefix",
			path:     "github.com/cloudposse/atmos/tools/gotcha/pkg/cache/cache.go",
			expected: "pkg/cache",
		},
		{
			name:     "path without prefix",
			path:     "internal/coverage/processor.go",
			expected: "internal/coverage",
		},
		{
			name:     "root level file",
			path:     "main.go",
			expected: "main",
		},
		{
			name:     "dot directory",
			path:     "./file.go",
			expected: "main",
		},
		{
			name:     "nested package",
			path:     "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/github/client.go",
			expected: "pkg/ci/github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPackageFromPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "string shorter than max",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "string equal to max",
			input:    "exactly10!",
			maxLen:   10,
			expected: "exactly10!",
		},
		{
			name:     "string longer than max",
			input:    "this is a very long string",
			maxLen:   10,
			expected: "this is...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   5,
			expected: "",
		},
		{
			name:     "very small max length",
			input:    "hello",
			maxLen:   3,
			expected: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "long path",
			path:     "very/long/path/to/some/file.go",
			expected: ".../some/file.go",
		},
		{
			name:     "short path",
			path:     "pkg/file.go",
			expected: "pkg/file.go",
		},
		{
			name:     "exactly 3 parts",
			path:     "one/two/three",
			expected: "one/two/three",
		},
		{
			name:     "single file",
			path:     "file.go",
			expected: "file.go",
		},
		{
			name:     "4 parts path",
			path:     "one/two/three/four",
			expected: ".../three/four",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortenPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldExcludeMocks(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		expected bool
	}{
		{
			name:     "contains mock pattern",
			patterns: []string{"test", "mock", "vendor"},
			expected: true,
		},
		{
			name:     "contains mock in pattern",
			patterns: []string{"test", "**/mock_*.go", "vendor"},
			expected: true,
		},
		{
			name:     "no mock patterns",
			patterns: []string{"test", "vendor", "generated"},
			expected: false,
		},
		{
			name:     "empty patterns",
			patterns: []string{},
			expected: false,
		},
		{
			name:     "mock as substring",
			patterns: []string{"mocking", "unmocked"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldExcludeMocks(tt.patterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterUncoveredFunctions(t *testing.T) {
	functions := []types.CoverageFunction{
		{Function: "func1", Coverage: 100.0},
		{Function: "func2", Coverage: 0.0},
		{Function: "func3", Coverage: 50.0},
		{Function: "func4", Coverage: 0.0},
		{Function: "func5", Coverage: 75.0},
	}

	result := filterUncoveredFunctions(functions)

	assert.Len(t, result, 2)
	assert.Equal(t, "func2", result[0].Function)
	assert.Equal(t, "func4", result[1].Function)
}

func TestShowFunctionCoverage(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		uncovered      bool
		expectedOutput string
	}{
		{
			name:           "detailed format",
			format:         "detailed",
			uncovered:      false,
			expectedOutput: "Function Coverage (Detailed)",
		},
		{
			name:           "summary format",
			format:         "summary",
			uncovered:      false,
			expectedOutput: "Function Coverage Summary",
		},
		{
			name:           "none format",
			format:         "none",
			uncovered:      false,
			expectedOutput: "", // Should not output anything
		},
		{
			name:           "default format",
			format:         "",
			uncovered:      false,
			expectedOutput: "Function Coverage Summary",
		},
		{
			name:           "uncovered filter",
			format:         "summary",
			uncovered:      true,
			expectedOutput: "Function Coverage Summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			data := &types.CoverageData{
				FunctionCoverage: []types.CoverageFunction{
					{Function: "TestFunc1", Coverage: 80.0, File: "test.go"},
					{Function: "TestFunc2", Coverage: 0.0, File: "test.go"},
				},
			}

			cfg := &config.CoverageConfig{
				Analysis: config.AnalysisConfig{
					Uncovered: tt.uncovered,
				},
				Output: config.OutputConfig{
					Terminal: config.TerminalConfig{
						Format:        tt.format,
						ShowUncovered: 5,
					},
				},
			}

			var buf bytes.Buffer
			writer := output.NewWriter(&buf, io.Discard, false)
			logger := log.New(io.Discard)

			showFunctionCoverage(data, cfg, writer, logger)

			output := buf.String()
			if tt.expectedOutput == "" {
				assert.Empty(t, output)
			} else {
				assert.Contains(t, output, tt.expectedOutput)
			}
		})
	}
}

func TestShowDetailedFunctionCoverage(t *testing.T) {
	functions := []types.CoverageFunction{
		{Function: "TestFunc1", Coverage: 80.0, File: "test.go"},
		{Function: "TestFunc2", Coverage: 0.0, File: "test.go"},
	}

	var buf bytes.Buffer
	writer := output.NewWriter(&buf, io.Discard, false)

	showDetailedFunctionCoverage(functions, writer)

	output := buf.String()
	assert.Contains(t, output, "Function Coverage (Detailed)")
	assert.Contains(t, output, "TestFunc1")
	assert.Contains(t, output, "TestFunc2")
	assert.Contains(t, output, "80.0%")
	assert.Contains(t, output, "0.0%")
}

func TestShowFunctionCoverageSummary(t *testing.T) {
	functions := []types.CoverageFunction{
		{Function: "CoveredFunc", Coverage: 90.0, File: "covered.go"},
		{Function: "UncoveredFunc1", Coverage: 0.0, File: "test.go"},
		{Function: "UncoveredFunc2", Coverage: 0.0, File: "test.go"},
		{Function: "PartialFunc", Coverage: 50.0, File: "partial.go"},
	}

	var buf bytes.Buffer
	writer := output.NewWriter(&buf, io.Discard, false)
	logger := log.New(io.Discard)

	showFunctionCoverageSummary(functions, 2, writer, logger)

	output := buf.String()
	assert.Contains(t, output, "Function Coverage Summary")
	assert.Contains(t, output, "Total Functions: 4")
	assert.Contains(t, output, "Covered: 2")
	assert.Contains(t, output, "Uncovered: 2")
	assert.Contains(t, output, "Coverage: 35.0%")
	assert.Contains(t, output, "Top 2 Uncovered Functions")
	assert.Contains(t, output, "UncoveredFunc1")
	assert.Contains(t, output, "UncoveredFunc2")
}

func TestCheckCoverageThresholds(t *testing.T) {
	tests := []struct {
		name             string
		coverageStr      string
		functionCount    int
		coveredCount     int
		thresholdEnabled bool
		thresholdValue   float64
		expectedError    bool
	}{
		{
			name:             "above threshold",
			coverageStr:      "85.0%",
			functionCount:    100,
			coveredCount:     85,
			thresholdEnabled: true,
			thresholdValue:   80.0,
			expectedError:    false,
		},
		{
			name:             "below threshold",
			coverageStr:      "75.0%",
			functionCount:    100,
			coveredCount:     75,
			thresholdEnabled: true,
			thresholdValue:   80.0,
			expectedError:    true,
		},
		{
			name:             "exactly at threshold",
			coverageStr:      "80.0%",
			functionCount:    100,
			coveredCount:     80,
			thresholdEnabled: true,
			thresholdValue:   80.0,
			expectedError:    false,
		},
		{
			name:             "threshold disabled",
			coverageStr:      "50.0%",
			functionCount:    100,
			coveredCount:     50,
			thresholdEnabled: false,
			thresholdValue:   80.0,
			expectedError:    false,
		},
		{
			name:             "invalid coverage string",
			coverageStr:      "invalid",
			functionCount:    100,
			coveredCount:     50,
			thresholdEnabled: true,
			thresholdValue:   80.0,
			expectedError:    false, // Parse error returns nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &types.CoverageData{
				StatementCoverage: tt.coverageStr,
				FunctionCoverage:  make([]types.CoverageFunction, tt.functionCount),
			}

			// Set up function coverage
			for i := 0; i < tt.coveredCount; i++ {
				data.FunctionCoverage[i].Coverage = 100.0
			}

			thresholds := config.CoverageThresholds{
				Statement: config.ThresholdConfig{
					Enabled: tt.thresholdEnabled,
					Value:   tt.thresholdValue,
				},
				Function: config.ThresholdConfig{
					Enabled: tt.thresholdEnabled,
					Value:   tt.thresholdValue,
				},
			}

			logger := log.New(io.Discard)
			err := checkCoverageThresholds(data, thresholds, logger)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "below threshold")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestShowStatementCoverage(t *testing.T) {
	data := &types.CoverageData{
		StatementCoverage: "85.5%",
		FilteredFiles:     []string{"mock_test.go", "mock_client.go"},
	}

	cfg := &config.CoverageConfig{
		Output: config.OutputConfig{
			Terminal: config.TerminalConfig{
				Format: "summary",
			},
		},
	}

	var buf bytes.Buffer
	writer := output.NewWriter(&buf, io.Discard, false)

	showStatementCoverage(data, cfg, writer)

	output := buf.String()
	assert.Contains(t, output, "Statement Coverage")
	assert.Contains(t, output, "85.5%")
	assert.Contains(t, output, "excluding 2 mocks")
}

func TestOpenBrowser(t *testing.T) {
	// Skip this test in CI environments
	if _, ok := os.LookupEnv("CI"); ok {
		t.Skip("Skipping browser test in CI environment")
	}

	logger := log.New(io.Discard)

	// Test with invalid path (won't actually open)
	err := OpenBrowser("/nonexistent/path.html", logger)

	// We expect no error even with invalid path, as cmd.Start() doesn't validate the file
	// The error would come from the browser, not from our code
	assert.NoError(t, err)
}

func TestProcessCoverage(t *testing.T) {
	// Create mock coverage profile data
	mockProfile := `mode: set
github.com/cloudposse/atmos/tools/gotcha/pkg/test/file.go:10.1,12.1 1 1
github.com/cloudposse/atmos/tools/gotcha/pkg/test/file.go:15.1,17.1 1 0
github.com/cloudposse/atmos/tools/gotcha/pkg/test/file.go:20.1,22.1 1 1`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "coverage*.out")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(mockProfile)
	require.NoError(t, err)
	tmpFile.Close()

	// Create config
	cfg := &config.CoverageConfig{
		ProfilePath: tmpFile.Name(),
		Thresholds: config.ThresholdConfig{
			Enabled: true,
			Value:   50.0,
		},
		Output: config.OutputConfig{
			Terminal: config.TerminalConfig{
				Enabled:       true,
				Format:        "summary",
				ShowUncovered: 5,
			},
		},
		Analysis: config.AnalysisConfig{
			Uncovered:     false,
			ShowFunctions: true,
		},
	}

	var buf bytes.Buffer
	writer := output.NewWriter(&buf, io.Discard, false)
	logger := log.New(io.Discard)

	err = ProcessCoverage(cfg, writer, logger)
	assert.NoError(t, err)

	output := buf.String()
	// Should contain some coverage output
	assert.NotEmpty(t, output)
}

func TestParseFunctionCoverageOutput(t *testing.T) {
	input := `github.com/cloudposse/atmos/tools/gotcha/pkg/test/file.go:10:	TestFunc1	80.0%
github.com/cloudposse/atmos/tools/gotcha/pkg/test/file.go:20:	TestFunc2	0.0%
github.com/cloudposse/atmos/tools/gotcha/pkg/utils/helper.go:5:	HelperFunc	100.0%
total:	(statements)	60.0%`

	functions := parseFunctionCoverageOutput(input)

	assert.Len(t, functions, 3)
	assert.Equal(t, "TestFunc1", functions[0].Function)
	assert.Equal(t, 80.0, functions[0].Coverage)
	assert.Equal(t, "pkg/test", functions[0].Package)
	assert.Equal(t, 10, functions[0].Line)

	assert.Equal(t, "TestFunc2", functions[1].Function)
	assert.Equal(t, 0.0, functions[1].Coverage)

	assert.Equal(t, "HelperFunc", functions[2].Function)
	assert.Equal(t, 100.0, functions[2].Coverage)
	assert.Equal(t, "pkg/utils", functions[2].Package)
}
