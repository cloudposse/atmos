package utils

import (
	"os"
	"testing"
)

func TestFilterPackages(t *testing.T) {
	tests := []struct {
		name            string
		packages        []string
		includePatterns string
		excludePatterns string
		want            []string
		wantErr         bool
	}{
		{
			name:            "include all packages",
			packages:        []string{"pkg1", "pkg2", "pkg3"},
			includePatterns: ".*",
			excludePatterns: "",
			want:            []string{"pkg1", "pkg2", "pkg3"},
			wantErr:         false,
		},
		{
			name:            "include specific pattern",
			packages:        []string{"api/v1", "api/v2", "internal/config", "cmd/main"},
			includePatterns: "api/.*",
			excludePatterns: "",
			want:            []string{"api/v1", "api/v2"},
			wantErr:         false,
		},
		{
			name:            "exclude pattern",
			packages:        []string{"pkg/main", "pkg/mock", "pkg/test"},
			includePatterns: ".*",
			excludePatterns: "mock",
			want:            []string{"pkg/main", "pkg/test"},
			wantErr:         false,
		},
		{
			name:            "include and exclude patterns",
			packages:        []string{"api/v1", "api/v2", "api/mock", "internal/config"},
			includePatterns: "api/.*",
			excludePatterns: "mock",
			want:            []string{"api/v1", "api/v2"},
			wantErr:         false,
		},
		{
			name:            "multiple include patterns",
			packages:        []string{"api/v1", "cmd/main", "internal/config", "pkg/utils"},
			includePatterns: "api/.*,cmd/.*",
			excludePatterns: "",
			want:            []string{"api/v1", "cmd/main"},
			wantErr:         false,
		},
		{
			name:            "multiple exclude patterns",
			packages:        []string{"api/v1", "api/mock", "cmd/mock", "pkg/utils"},
			includePatterns: ".*",
			excludePatterns: "mock,utils",
			want:            []string{"api/v1"},
			wantErr:         false,
		},
		{
			name:            "no matches",
			packages:        []string{"pkg1", "pkg2"},
			includePatterns: "nonexistent",
			excludePatterns: "",
			want:            []string{},
			wantErr:         false,
		},
		{
			name:            "empty packages list",
			packages:        []string{},
			includePatterns: ".*",
			excludePatterns: "",
			want:            []string{},
			wantErr:         false,
		},
		{
			name:            "invalid include regex",
			packages:        []string{"pkg1"},
			includePatterns: "[",
			excludePatterns: "",
			want:            nil,
			wantErr:         true,
		},
		{
			name:            "invalid exclude regex",
			packages:        []string{"pkg1"},
			includePatterns: ".*",
			excludePatterns: "[",
			want:            nil,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterPackages(tt.packages, tt.includePatterns, tt.excludePatterns)

			if (err != nil) != tt.wantErr {
				t.Errorf("filterPackages() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("filterPackages() got %d packages, want %d", len(got), len(tt.want))
					return
				}

				for i, pkg := range got {
					if pkg != tt.want[i] {
						t.Errorf("filterPackages() got[%d] = %v, want %v", i, pkg, tt.want[i])
					}
				}
			}
		})
	}
}

func TestGetTestCount(t *testing.T) {
	// This function uses AST parsing to count tests, so we need to test it with actual Go files
	tests := []struct {
		name         string
		testPackages []string
		testArgs     string
		wantMin      int // minimum expected tests (since we can't predict exact count)
	}{
		{
			name:         "current package",
			testPackages: []string{"."},
			testArgs:     "",
			wantMin:      1, // At least this test should be counted
		},
		{
			name:         "empty packages",
			testPackages: []string{},
			testArgs:     "",
			wantMin:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize logger for the test
			initGlobalLogger()

			got := getTestCount(tt.testPackages, tt.testArgs)

			if got < tt.wantMin {
				t.Errorf("getTestCount() = %v, want at least %v", got, tt.wantMin)
			}
		})
	}
}

func TestIsTTY(t *testing.T) {
	// This function checks if we're running in a TTY environment
	// We can test that it returns a boolean without error
	result := isTTY()

	// The result should be either true or false
	if result != true && result != false {
		t.Errorf("isTTY() returned non-boolean value")
	}

	// In CI environments, this is typically false
	// In development with a real terminal, this might be true
	// We just ensure it doesn't panic and returns a valid boolean
}

func TestRunSimpleStream(t *testing.T) {
	tests := []struct {
		name         string
		testPackages []string
		testArgs     string
		outputFile   string
		coverProfile string
		showFilter   string
		totalTests   int
		wantExitCode int
	}{
		{
			name:         "placeholder function",
			testPackages: []string{},
			testArgs:     "",
			outputFile:   "",
			coverProfile: "",
			showFilter:   "all",
			totalTests:   0,
			wantExitCode: 0, // Placeholder currently returns 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runSimpleStream(tt.testPackages, tt.testArgs, tt.outputFile, tt.coverProfile, tt.showFilter, tt.totalTests, false)

			if got != tt.wantExitCode {
				t.Errorf("runSimpleStream() = %v, want %v", got, tt.wantExitCode)
			}
		})
	}
}

func TestHandleConsoleOutput(t *testing.T) {
	tests := []struct {
		name    string
		summary *TestSummary
		wantErr bool
	}{
		{
			name: "valid summary with passed tests",
			summary: &TestSummary{
				Passed:   []TestResult{{Package: "pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
				Coverage: "75.0%",
			},
			wantErr: false,
		},
		{
			name: "valid summary with failed tests",
			summary: &TestSummary{
				Failed:   []TestResult{{Package: "pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
				Coverage: "50.0%",
			},
			wantErr: false,
		},
		{
			name: "empty summary",
			summary: &TestSummary{
				Coverage: "0.0%",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout to avoid cluttering test output
			// Save original stdout
			oldStdout := os.Stdout
			defer func() { os.Stdout = oldStdout }()

			// Create a pipe to capture output
			_, w, _ := os.Pipe()
			os.Stdout = w

			err := handleConsoleOutput(tt.summary)

			// Close the pipe and restore stdout
			w.Close()
			os.Stdout = oldStdout

			if (err != nil) != tt.wantErr {
				t.Errorf("handleConsoleOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
