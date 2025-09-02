package main

import (
	"os"
	"testing"
)

func TestGetFunctionCoverage(t *testing.T) {
	tests := []struct {
		name         string
		profileFile  string
		excludeMocks bool
		wantLen      int
		wantErr      bool
	}{
		{
			name:         "non-existent file",
			profileFile:  "/non/existent/file.out",
			excludeMocks: true,
			wantLen:      0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getFunctionCoverage(tt.profileFile, tt.excludeMocks)

			if (err != nil) != tt.wantErr {
				t.Errorf("getFunctionCoverage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != tt.wantLen {
					t.Errorf("getFunctionCoverage() returned %d functions, want %d", len(got), tt.wantLen)
				}
			}
		})
	}
}

func TestParseCoverageProfileEnhanced(t *testing.T) {
	// Create a comprehensive test coverage file
	testCoverageContent := `mode: set
github.com/cloudposse/atmos/tools/gotcha/main.go:56.13,58.2 2 1
github.com/cloudposse/atmos/tools/gotcha/parser.go:14.98,19.16 4 1
github.com/cloudposse/atmos/tools/gotcha/parser.go:19.16,21.3 1 0
github.com/cloudposse/atmos/tools/gotcha/utils.go:10.32,12.31 2 1
github.com/cloudposse/atmos/tools/gotcha/utils.go:12.31,14.3 1 1
github.com/cloudposse/atmos/tools/gotcha/utils.go:15.2,15.12 1 1
github.com/cloudposse/atmos/tools/gotcha/mock_service.go:4.20,6.2 1 0`

	tmpfile, err := os.CreateTemp("", "coverage_enhanced_*.out")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(testCoverageContent); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		profileFile   string
		excludeMocks  bool
		wantErr       bool
		checkCoverage bool
	}{
		{
			name:          "enhanced coverage with mocks excluded",
			profileFile:   tmpfile.Name(),
			excludeMocks:  true,
			wantErr:       false,
			checkCoverage: true,
		},
		{
			name:          "enhanced coverage with mocks included",
			profileFile:   tmpfile.Name(),
			excludeMocks:  false,
			wantErr:       false,
			checkCoverage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoverageProfile(tt.profileFile, tt.excludeMocks)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseCoverageProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkCoverage && got != nil {
				// Verify that statement coverage is calculated
				if got.StatementCoverage == "" {
					t.Error("parseCoverageProfile() should set StatementCoverage")
				}

				// Verify function coverage data is parsed
				if len(got.FunctionCoverage) == 0 {
					t.Error("parseCoverageProfile() should parse function coverage data")
				}
			}
		})
	}
}

// Test with empty and malformed coverage files.
func TestParseCoverageProfileErrorCases(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		excludeMocks bool
		wantErr      bool
	}{
		{
			name:         "empty file",
			content:      "",
			excludeMocks: true,
			wantErr:      false, // parseStatementCoverage handles empty files gracefully
		},
		{
			name:         "file with only mode line",
			content:      "mode: set\n",
			excludeMocks: true,
			wantErr:      false, // parseStatementCoverage handles mode-only files gracefully
		},
		{
			name: "file with invalid coverage line",
			content: `mode: set
invalid line without proper format`,
			excludeMocks: true,
			wantErr:      false, // parseCoverageProfile handles invalid lines gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp("", "coverage_error_*.out")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.WriteString(tt.content); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			_, err = parseCoverageProfile(tmpfile.Name(), tt.excludeMocks)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseCoverageProfile() with %s: error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
