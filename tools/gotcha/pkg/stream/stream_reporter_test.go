package stream

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
	"github.com/stretchr/testify/assert"
)

func TestNewStreamReporter(t *testing.T) {
	tests := []struct {
		name           string
		writer         *output.Writer
		showFilter     string
		testFilter     string
		verbosityLevel string
		wantNilWriter  bool
	}{
		{
			name:           "with nil writer",
			writer:         nil,
			showFilter:     "all",
			testFilter:     "",
			verbosityLevel: "standard",
			wantNilWriter:  false,
		},
		{
			name:           "with provided writer",
			writer:         output.New(),
			showFilter:     "failed",
			testFilter:     "TestPattern",
			verbosityLevel: "verbose",
			wantNilWriter:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := NewStreamReporter(tt.writer, tt.showFilter, tt.testFilter, tt.verbosityLevel)
			assert.NotNil(t, reporter)
			assert.Equal(t, tt.showFilter, reporter.showFilter)
			assert.Equal(t, tt.testFilter, reporter.testFilter)
			assert.Equal(t, tt.verbosityLevel, reporter.verbosityLevel)
			assert.NotNil(t, reporter.writer)
			assert.NotNil(t, reporter.displayedPackages)
			assert.NotNil(t, reporter.packageCoverages)
			assert.NotNil(t, reporter.packageStatementCoverages)
			assert.NotNil(t, reporter.packageFunctionCoverages)
			assert.NotNil(t, reporter.buildFailedPackages)
		})
	}
}

func TestStreamReporter_OnPackageStart(t *testing.T) {
	reporter := NewStreamReporter(nil, "all", "", "standard")
	pkg := &PackageResult{Package: "test/package"}

	// Should not panic and do nothing
	reporter.OnPackageStart(pkg)

	// Verify no packages were marked as displayed
	assert.Empty(t, reporter.displayedPackages)
}

func TestStreamReporter_OnPackageComplete(t *testing.T) {
	tests := []struct {
		name              string
		pkg               *PackageResult
		expectOutput      bool
		expectedFragments []string
	}{
		{
			name: "successful package with tests",
			pkg: &PackageResult{
				Package:           "test/package",
				Status:            TestStatusPass,
				HasTests:          true,
				StatementCoverage: "85.5%",
				FunctionCoverage:  "90.0%",
				Tests: map[string]*TestResult{
					"TestSuccess": {
						Name:    "TestSuccess",
						Status:  constants.PassStatus,
						Elapsed: 1.5,
					},
				},
				TestOrder: []string{"TestSuccess"},
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/package",
				"TestSuccess",
				"All 1 tests passed",
				"statements: 85.5%, functions: 90.0%",
			},
		},
		{
			name: "failed package with no tests (build failure)",
			pkg: &PackageResult{
				Package: "test/failed",
				Status:  TestStatusFail,
				Tests:   map[string]*TestResult{},
				Output:  []string{"compilation error: undefined: xyz\n"},
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/failed",
				"Package failed to build",
				"compilation error",
			},
		},
		{
			name: "package with no tests",
			pkg: &PackageResult{
				Package:  "test/empty",
				HasTests: false,
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/empty",
				"No tests",
			},
		},
		{
			name: "package with no tests but filter applied",
			pkg: &PackageResult{
				Package:  "test/filtered",
				HasTests: false,
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/filtered",
				"No tests matching filter",
			},
		},
		{
			name: "package with failed and passed tests",
			pkg: &PackageResult{
				Package:  "test/mixed",
				Status:   TestStatusFail,
				HasTests: true,
				Coverage: "75.0%",
				Tests: map[string]*TestResult{
					"TestPass": {
						Name:    "TestPass",
						Status:  constants.PassStatus,
						Elapsed: 0.5,
					},
					"TestFail": {
						Name:    "TestFail",
						Status:  TestStatusFail,
						Elapsed: 1.0,
						Output:  []string{"assertion failed\n"},
					},
				},
				TestOrder: []string{"TestPass", "TestFail"},
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/mixed",
				"TestPass",
				"TestFail",
				"1 tests failed, 1 passed",
				"75.0% coverage",
			},
		},
		{
			name: "package with skipped tests",
			pkg: &PackageResult{
				Package:  "test/skipped",
				HasTests: true,
				Tests: map[string]*TestResult{
					"TestSkip": {
						Name:       "TestSkip",
						Status:     TestStatusSkip,
						SkipReason: "not implemented",
					},
				},
				TestOrder: []string{"TestSkip"},
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/skipped",
				"TestSkip",
				"not implemented",
				"1 tests skipped",
			},
		},
		{
			name: "package with subtests",
			pkg: &PackageResult{
				Package:  "test/subtests",
				HasTests: true,
				Tests: map[string]*TestResult{
					"TestParent": {
						Name:   "TestParent",
						Status: TestStatusFail,
						Subtests: map[string]*TestResult{
							"TestParent/sub1": {
								Name:   "sub1",
								Status: constants.PassStatus,
							},
							"TestParent/sub2": {
								Name:   "sub2",
								Status: TestStatusFail,
								Output: []string{"subtest failed\n"},
							},
						},
						SubtestOrder: []string{"TestParent/sub1", "TestParent/sub2"},
					},
				},
				TestOrder: []string{"TestParent"},
			},
			expectOutput: true,
			expectedFragments: []string{
				"test/subtests",
				"TestParent",
				"50% passed", // Mini progress indicator
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputUI strings.Builder
			var outputData strings.Builder
			writer := output.NewCustom(&outputData, &outputUI)
			reporter := &StreamReporter{
				writer:                    writer,
				showFilter:                "all",
				testFilter:                "",
				verbosityLevel:            "standard",
				displayedPackages:         make(map[string]bool),
				packageCoverages:          make([]float64, 0),
				packageStatementCoverages: make([]float64, 0),
				packageFunctionCoverages:  make([]float64, 0),
				buildFailedPackages:       make([]string, 0),
			}

			// Special case for filtered test
			if tt.name == "package with no tests but filter applied" {
				reporter.testFilter = "TestFilter"
			}

			reporter.OnPackageComplete(tt.pkg)

			result := outputUI.String()
			if tt.expectOutput {
				assert.NotEmpty(t, result)
				for _, fragment := range tt.expectedFragments {
					assert.Contains(t, result, fragment)
				}
			}

			// Verify package is marked as displayed
			assert.True(t, reporter.displayedPackages[tt.pkg.Package])

			// Test duplicate display prevention
			outputUI.Reset()
			reporter.OnPackageComplete(tt.pkg)
			assert.Empty(t, outputUI.String(), "Duplicate package should not be displayed")
		})
	}
}

func TestStreamReporter_shouldShowTestStatus(t *testing.T) {
	tests := []struct {
		name       string
		showFilter string
		status     string
		expected   bool
	}{
		{"all filter - pass", "all", constants.PassStatus, true},
		{"all filter - fail", "all", TestStatusFail, true},
		{"all filter - skip", "all", TestStatusSkip, true},
		{"failed filter - pass", "failed", constants.PassStatus, false},
		{"failed filter - fail", "failed", TestStatusFail, true},
		{"failed filter - skip", "failed", TestStatusSkip, true},
		{"passed filter - pass", "passed", constants.PassStatus, true},
		{"passed filter - fail", "passed", TestStatusFail, false},
		{"passed filter - skip", "passed", TestStatusSkip, false},
		{"skipped filter - skip", "skipped", TestStatusSkip, true},
		{"skipped filter - pass", "skipped", constants.PassStatus, false},
		{"collapsed filter - any", "collapsed", constants.PassStatus, false},
		{"none filter - any", "none", TestStatusFail, false},
		{"default filter - pass", "default", constants.PassStatus, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := &StreamReporter{showFilter: tt.showFilter}
			result := reporter.shouldShowTestStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCoverageValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"percentage only", "85.5%", 85.5},
		{"with suffix", "90.0% of statements", 90.0},
		{"zero coverage", "0.0%", 0.0},
		{"100 percent", "100%", 100.0},
		{"invalid format", "invalid", -1},
		{"empty string", "", -1},
		{"with spaces", "  75.5%  ", 75.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCoverageValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStreamReporter_Finalize(t *testing.T) {
	tests := []struct {
		name                      string
		passed                    int
		failed                    int
		skipped                   int
		elapsed                   time.Duration
		packageCoverages          []float64
		packageStatementCoverages []float64
		packageFunctionCoverages  []float64
		buildFailedPackages       []string
		expectedFragments         []string
	}{
		{
			name:              "all tests passed",
			passed:            10,
			failed:            0,
			skipped:           2,
			elapsed:           5 * time.Second,
			expectedFragments: []string{"Test Results:", "Passed:", "10", "All tests passed", "5.00s"},
		},
		{
			name:              "some tests failed",
			passed:            8,
			failed:            2,
			skipped:           1,
			elapsed:           3 * time.Second,
			expectedFragments: []string{"Test Results:", "Failed:", "2", "2 tests failed"},
		},
		{
			name:                      "with coverage",
			passed:                    5,
			failed:                    0,
			skipped:                   0,
			elapsed:                   2 * time.Second,
			packageStatementCoverages: []float64{80.0, 90.0, 70.0},
			expectedFragments:         []string{"Statement Coverage:", "80.0%"},
		},
		{
			name:                      "with statement and function coverage",
			passed:                    5,
			failed:                    0,
			skipped:                   0,
			elapsed:                   2 * time.Second,
			packageStatementCoverages: []float64{80.0, 90.0},
			packageFunctionCoverages:  []float64{85.0, 95.0},
			expectedFragments:         []string{"Statement Coverage:", "85.0%", "Function Coverage:", "90.0%"},
		},
		{
			name:                "build failures only",
			passed:              0,
			failed:              0,
			skipped:             0,
			elapsed:             1 * time.Second,
			buildFailedPackages: []string{"pkg1", "pkg2"},
			expectedFragments:   []string{"Build Failed:", "2", "2 packages failed to build"},
		},
		{
			name:                "build and test failures",
			passed:              5,
			failed:              3,
			skipped:             0,
			elapsed:             4 * time.Second,
			buildFailedPackages: []string{"pkg1"},
			expectedFragments:   []string{"3 tests failed and 1 package failed to build"},
		},
		{
			name:              "no tests found",
			passed:            0,
			failed:            0,
			skipped:           0,
			elapsed:           100 * time.Millisecond,
			expectedFragments: []string{}, // Should return empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputUI strings.Builder
			var outputData strings.Builder
			writer := output.NewCustom(&outputData, &outputUI)
			reporter := &StreamReporter{
				writer:                    writer,
				packageCoverages:          tt.packageCoverages,
				packageStatementCoverages: tt.packageStatementCoverages,
				packageFunctionCoverages:  tt.packageFunctionCoverages,
				buildFailedPackages:       tt.buildFailedPackages,
			}

			result := reporter.Finalize(tt.passed, tt.failed, tt.skipped, tt.elapsed)

			if len(tt.expectedFragments) > 0 {
				assert.NotEmpty(t, result)
				for _, fragment := range tt.expectedFragments {
					assert.Contains(t, result, fragment)
				}
			} else if tt.passed == 0 && tt.failed == 0 && tt.skipped == 0 && len(tt.buildFailedPackages) == 0 {
				assert.Empty(t, result)
			}
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
		{100, "s"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.count)), func(t *testing.T) {
			result := pluralize(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStreamReporter_generateSubtestProgress(t *testing.T) {
	tests := []struct {
		name     string
		passed   int
		total    int
		expected string
	}{
		{
			name:     "all passed",
			passed:   5,
			total:    5,
			expected: "●●●●●",
		},
		{
			name:     "none passed",
			passed:   0,
			total:    5,
			expected: "●●●●●",
		},
		{
			name:     "mixed results",
			passed:   3,
			total:    5,
			expected: "●●●●●",
		},
		{
			name:     "zero total",
			passed:   0,
			total:    0,
			expected: "",
		},
		{
			name:     "large numbers scaled down",
			passed:   50,
			total:    100,
			expected: "●●●●●●●●●●",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := &StreamReporter{}
			result := reporter.generateSubtestProgress(tt.passed, tt.total)
			// Just verify length since colors are stripped in test
			assert.Equal(t, len(tt.expected), len(result))
		})
	}
}

func TestStreamReporter_displayTestLine(t *testing.T) {
	tests := []struct {
		name              string
		test              *TestResult
		indent            string
		verbosityLevel    string
		showFilter        string
		expectedFragments []string
	}{
		{
			name: "passing test",
			test: &TestResult{
				Name:    "TestPass",
				Status:  constants.PassStatus,
				Elapsed: 1.5,
			},
			expectedFragments: []string{"TestPass", "(1.50s)"},
		},
		{
			name: "failing test with output",
			test: &TestResult{
				Name:    "TestFail",
				Status:  TestStatusFail,
				Elapsed: 2.0,
				Output:  []string{"assertion failed\n", "expected: 1\n", "actual: 2\n"},
			},
			expectedFragments: []string{"TestFail", "(2.00s)", "assertion failed"},
		},
		{
			name: "skipped test with reason",
			test: &TestResult{
				Name:       "TestSkip",
				Status:     TestStatusSkip,
				SkipReason: "requires admin privileges",
			},
			expectedFragments: []string{"TestSkip", "requires admin privileges"},
		},
		{
			name: "test with verbose output",
			test: &TestResult{
				Name:    "TestVerbose",
				Status:  TestStatusFail,
				Elapsed: 0.5,
				Output:  []string{"line1\\tindented\\n", "line2\\n"},
			},
			verbosityLevel:    "verbose",
			expectedFragments: []string{"TestVerbose", "line1\tindented", "line2"},
		},
		{
			name: "running test should not display",
			test: &TestResult{
				Name:   "TestRunning",
				Status: "run",
			},
			expectedFragments: []string{}, // Should not display anything
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputUI strings.Builder
			var outputData strings.Builder
			writer := output.NewCustom(&outputData, &outputUI)
			reporter := &StreamReporter{
				writer:         writer,
				showFilter:     "all",
				verbosityLevel: "standard",
			}

			if tt.verbosityLevel != "" {
				reporter.verbosityLevel = tt.verbosityLevel
			}
			if tt.showFilter != "" {
				reporter.showFilter = tt.showFilter
			}

			reporter.displayTestLine(tt.test, tt.indent)

			result := outputUI.String()
			if len(tt.expectedFragments) > 0 {
				for _, fragment := range tt.expectedFragments {
					assert.Contains(t, result, fragment)
				}
			} else {
				assert.Empty(t, result)
			}
		})
	}
}

func TestStreamReporter_OnTestMethods(t *testing.T) {
	reporter := NewStreamReporter(nil, "all", "", "standard")
	pkg := &PackageResult{Package: "test/package"}
	test := &TestResult{Name: "TestSomething"}

	// These methods should do nothing but not panic
	reporter.OnTestStart(pkg, test)
	reporter.OnTestComplete(pkg, test)
	reporter.UpdateProgress(5, 10, 2*time.Second)
	reporter.SetEstimatedTotal(100)

	// Verify no side effects
	assert.Empty(t, reporter.displayedPackages)
}

func TestStreamReporter_displayTest(t *testing.T) {
	tests := []struct {
		name              string
		test              *TestResult
		indent            string
		showFilter        string
		verbosityLevel    string
		expectedFragments []string
		notExpected       []string
	}{
		{
			name: "test with subtests shows mini progress",
			test: &TestResult{
				Name:    "TestParent",
				Status:  TestStatusPass,
				Elapsed: 2.5,
				Subtests: map[string]*TestResult{
					"TestParent/sub1": {
						Name:   "sub1",
						Status: constants.PassStatus,
					},
					"TestParent/sub2": {
						Name:   "sub2",
						Status: constants.PassStatus,
					},
					"TestParent/sub3": {
						Name:   "sub3",
						Status: TestStatusFail,
					},
				},
				SubtestOrder: []string{"TestParent/sub1", "TestParent/sub2", "TestParent/sub3"},
			},
			showFilter:        "all",
			expectedFragments: []string{"TestParent", "66% passed", "(2.50s)"},
		},
		{
			name: "failed test with subtests shows all in failed filter",
			test: &TestResult{
				Name:    "TestParent",
				Status:  TestStatusFail,
				Elapsed: 1.0,
				Subtests: map[string]*TestResult{
					"TestParent/sub1": {
						Name:   "sub1",
						Status: constants.PassStatus,
					},
					"TestParent/sub2": {
						Name:   "sub2",
						Status: TestStatusFail,
						Output: []string{"subtest error\n"},
					},
				},
				SubtestOrder: []string{"TestParent/sub1", "TestParent/sub2"},
			},
			showFilter:        "failed",
			expectedFragments: []string{"TestParent", "50% passed", "sub2"},
			notExpected:       []string{"sub1"}, // Passed subtest should not show
		},
		{
			name: "test with no subtests displays normally",
			test: &TestResult{
				Name:    "TestSimple",
				Status:  constants.PassStatus,
				Elapsed: 0.5,
			},
			showFilter:        "all",
			expectedFragments: []string{"TestSimple", "(0.50s)"},
			notExpected:       []string{"% passed"}, // No mini progress for non-subtest
		},
		{
			name: "running test should not display",
			test: &TestResult{
				Name:   "TestRunning",
				Status: "run",
			},
			showFilter:        "all",
			expectedFragments: []string{},
		},
		{
			name: "skipped test shows skip icon",
			test: &TestResult{
				Name:   "TestSkipped",
				Status: TestStatusSkip,
			},
			showFilter:        "all",
			expectedFragments: []string{"TestSkipped"},
		},
		{
			name: "test with verbose output and special characters",
			test: &TestResult{
				Name:    "TestVerbose",
				Status:  TestStatusFail,
				Elapsed: 0.3,
				Output:  []string{"line1\\tindented\\n", "special\\nchars\\t"},
			},
			verbosityLevel:    "verbose",
			showFilter:        "all",
			expectedFragments: []string{"TestVerbose", "line1\tindented", "special\nchars\t"},
		},
		{
			name: "test with standard output mode",
			test: &TestResult{
				Name:    "TestStandard",
				Status:  TestStatusFail,
				Elapsed: 0.2,
				Output:  []string{"error message\n", "stack trace\n"},
			},
			verbosityLevel:    "standard",
			showFilter:        "all",
			expectedFragments: []string{"TestStandard", "error message", "stack trace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputUI strings.Builder
			var outputData strings.Builder
			writer := output.NewCustom(&outputData, &outputUI)
			reporter := &StreamReporter{
				writer:         writer,
				showFilter:     tt.showFilter,
				verbosityLevel: "standard",
			}

			if tt.verbosityLevel != "" {
				reporter.verbosityLevel = tt.verbosityLevel
			}

			// Call the displayTest method (not displayTestLine)
			reporter.displayTest(tt.test, tt.indent)

			result := outputUI.String()
			if len(tt.expectedFragments) > 0 {
				for _, fragment := range tt.expectedFragments {
					assert.Contains(t, result, fragment, "Expected fragment '%s' not found", fragment)
				}
			} else {
				assert.Empty(t, result)
			}

			for _, notExpected := range tt.notExpected {
				assert.NotContains(t, result, notExpected, "Unexpected fragment '%s' found", notExpected)
			}
		})
	}
}
