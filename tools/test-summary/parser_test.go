package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestProcessLine(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		initialTests   map[string]TestResult
		expectedOutput string
		expectedTests  map[string]TestResult
	}{
		{
			name: "valid test pass event",
			line: `{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"github.com/test/pkg","Test":"TestExample","Elapsed":0.5}`,
			initialTests: make(map[string]TestResult),
			expectedOutput: "",
			expectedTests: map[string]TestResult{
				"github.com/test/pkg.TestExample": {
					Package:  "github.com/test/pkg",
					Test:     "TestExample",
					Status:   "pass",
					Duration: 0.5,
				},
			},
		},
		{
			name: "valid test fail event",
			line: `{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"github.com/test/pkg","Test":"TestFailing","Elapsed":1.2}`,
			initialTests: make(map[string]TestResult),
			expectedOutput: "",
			expectedTests: map[string]TestResult{
				"github.com/test/pkg.TestFailing": {
					Package:  "github.com/test/pkg",
					Test:     "TestFailing",
					Status:   "fail",
					Duration: 1.2,
				},
			},
		},
		{
			name: "coverage output line",
			line: `{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"github.com/test/pkg","Output":"coverage: 75.5% of statements\n"}`,
			initialTests: make(map[string]TestResult),
			expectedOutput: "coverage: 75.5% of statements\n",
			expectedTests: make(map[string]TestResult),
		},
		{
			name: "regular output line",
			line: `{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"github.com/test/pkg","Test":"TestExample","Output":"=== RUN   TestExample\n"}`,
			initialTests: make(map[string]TestResult),
			expectedOutput: "",
			expectedTests: make(map[string]TestResult),
		},
		{
			name:           "invalid JSON",
			line:           `invalid json line`,
			initialTests:   make(map[string]TestResult),
			expectedOutput: "",
			expectedTests:  make(map[string]TestResult),
		},
		{
			name:           "empty line",
			line:           "",
			initialTests:   make(map[string]TestResult),
			expectedOutput: "",
			expectedTests:  make(map[string]TestResult),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutput := processLine(tt.line, tt.initialTests)
			
			if gotOutput != tt.expectedOutput {
				t.Errorf("processLine() output = %v, want %v", gotOutput, tt.expectedOutput)
			}
			
			if !reflect.DeepEqual(tt.initialTests, tt.expectedTests) {
				t.Errorf("processLine() tests map = %v, want %v", tt.initialTests, tt.expectedTests)
			}
		})
	}
}

func TestExtractCoverage(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "valid coverage output",
			output: "coverage: 75.5% of statements\n",
			want:   "75.5%",
		},
		{
			name:   "coverage output with additional text",
			output: "some text\ncoverage: 82.3% of statements\nmore text",
			want:   "82.3%",
		},
		{
			name:   "no coverage in output",
			output: "just some regular output without coverage info",
			want:   "",
		},
		{
			name:   "coverage 0%",
			output: "coverage: 0.0% of statements",
			want:   "0.0%",
		},
		{
			name:   "coverage 100%",
			output: "coverage: 100.0% of statements",
			want:   "100.0%",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "malformed coverage line",
			output: "coverage: invalid% of statements",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCoverage(tt.output)
			if got != tt.want {
				t.Errorf("extractCoverage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordTestResult(t *testing.T) {
	tests := []struct {
		name         string
		event        *TestEvent
		initialTests map[string]TestResult
		want         map[string]TestResult
	}{
		{
			name: "record pass event",
			event: &TestEvent{
				Action:  "pass",
				Package: "github.com/test/pkg",
				Test:    "TestExample",
				Elapsed: 0.5,
			},
			initialTests: make(map[string]TestResult),
			want: map[string]TestResult{
				"github.com/test/pkg.TestExample": {
					Package:  "github.com/test/pkg",
					Test:     "TestExample",
					Status:   "pass",
					Duration: 0.5,
				},
			},
		},
		{
			name: "record fail event",
			event: &TestEvent{
				Action:  "fail",
				Package: "github.com/test/pkg",
				Test:    "TestFailing",
				Elapsed: 1.2,
			},
			initialTests: make(map[string]TestResult),
			want: map[string]TestResult{
				"github.com/test/pkg.TestFailing": {
					Package:  "github.com/test/pkg",
					Test:     "TestFailing",
					Status:   "fail",
					Duration: 1.2,
				},
			},
		},
		{
			name: "record skip event",
			event: &TestEvent{
				Action:  "skip",
				Package: "github.com/test/pkg",
				Test:    "TestSkipped",
			},
			initialTests: make(map[string]TestResult),
			want: map[string]TestResult{
				"github.com/test/pkg.TestSkipped": {
					Package:  "github.com/test/pkg",
					Test:     "TestSkipped",
					Status:   "skip",
					Duration: 0,
				},
			},
		},
		{
			name: "update existing test result",
			event: &TestEvent{
				Action:  "pass",
				Package: "github.com/test/pkg",
				Test:    "TestExample",
				Elapsed: 0.8,
			},
			initialTests: map[string]TestResult{
				"github.com/test/pkg.TestExample": {
					Package:  "github.com/test/pkg",
					Test:     "TestExample",
					Status:   "run",
					Duration: 0,
				},
			},
			want: map[string]TestResult{
				"github.com/test/pkg.TestExample": {
					Package:  "github.com/test/pkg",
					Test:     "TestExample",
					Status:   "pass",
					Duration: 0.8,
				},
			},
		},
		{
			name: "ignore non-final actions",
			event: &TestEvent{
				Action:  "run",
				Package: "github.com/test/pkg",
				Test:    "TestExample",
			},
			initialTests: make(map[string]TestResult),
			want:         make(map[string]TestResult),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordTestResult(tt.event, tt.initialTests)
			
			if !reflect.DeepEqual(tt.initialTests, tt.want) {
				t.Errorf("recordTestResult() result = %v, want %v", tt.initialTests, tt.want)
			}
		})
	}
}

func TestParseTestJSON(t *testing.T) {
	tests := []struct {
		name         string
		jsonInput    string
		coverProfile string
		excludeMocks bool
		wantErr      bool
		checkCounts  bool
		expectPassed int
		expectFailed int
		expectSkipped int
	}{
		{
			name: "simple test results",
			jsonInput: `{"Action":"pass","Package":"github.com/test/pkg","Test":"TestOne","Elapsed":0.1}
{"Action":"fail","Package":"github.com/test/pkg","Test":"TestTwo","Elapsed":0.2}
{"Action":"skip","Package":"github.com/test/pkg","Test":"TestThree"}
`,
			checkCounts:   true,
			expectPassed:  1,
			expectFailed:  1,
			expectSkipped: 1,
		},
		{
			name: "test results with coverage output",
			jsonInput: `{"Action":"pass","Package":"github.com/test/pkg","Test":"TestOne","Elapsed":0.1}
{"Action":"output","Package":"github.com/test/pkg","Output":"coverage: 75.5% of statements\n"}
`,
			checkCounts:  true,
			expectPassed: 1,
		},
		{
			name:      "empty input",
			jsonInput: "",
			checkCounts: true,
			expectPassed: 0,
			expectFailed: 0,
			expectSkipped: 0,
		},
		{
			name: "malformed JSON line",
			jsonInput: `{"Action":"pass","Package":"github.com/test/pkg","Test":"TestOne","Elapsed":0.1}
invalid json line
{"Action":"fail","Package":"github.com/test/pkg","Test":"TestTwo","Elapsed":0.2}
`,
			checkCounts:  true,
			expectPassed: 1,
			expectFailed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.jsonInput)
			
			got, err := parseTestJSON(reader, tt.coverProfile, tt.excludeMocks)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if tt.checkCounts {
				if len(got.Passed) != tt.expectPassed {
					t.Errorf("parseTestJSON() passed count = %d, want %d", len(got.Passed), tt.expectPassed)
				}
				if len(got.Failed) != tt.expectFailed {
					t.Errorf("parseTestJSON() failed count = %d, want %d", len(got.Failed), tt.expectFailed)
				}
				if len(got.Skipped) != tt.expectSkipped {
					t.Errorf("parseTestJSON() skipped count = %d, want %d", len(got.Skipped), tt.expectSkipped)
				}
			}
		})
	}
}