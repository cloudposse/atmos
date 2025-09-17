package coverage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cloudposse/gotcha/pkg/constants"
)

func TestParseCoverageLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    *CoverageLine
		wantErr bool
	}{
		{
			name: "valid coverage line",
			line: "github.com/example/pkg/file.go:10.2,12.3 2 1",
			want: &CoverageLine{
				Filename:   "github.com/example/pkg/file.go",
				Statements: 2,
				Covered:    2,
			},
		},
		{
			name: "zero statements",
			line: "github.com/example/pkg/empty.go:5.1,5.2 0 0",
			want: &CoverageLine{
				Filename:   "github.com/example/pkg/empty.go",
				Statements: 0,
				Covered:    0,
			},
		},
		{
			name:    "invalid format - missing parts",
			line:    "invalid line",
			wantErr: true,
		},
		{
			name:    "invalid format - bad numbers",
			line:    "file.go:1.1,2.2 abc def",
			wantErr: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoverageLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCoverageLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCoverageLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMockFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{
			name:     "mock file with mock_ prefix",
			filename: "mock_service.go",
			want:     true,
		},
		{
			name:     "mock file in mock directory",
			filename: "internal/mock/service.go",
			want:     true,
		},
		{
			name:     "mock file with _mock suffix",
			filename: "service_mock.go",
			want:     true,
		},
		{
			name:     "regular file",
			filename: "service.go",
			want:     false,
		},
		{
			name:     "file with mock in name but not mock file",
			filename: "mockup_design.go",
			want:     false,
		},
		{
			name:     "empty filename",
			filename: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMockFile(tt.filename)
			if got != tt.want {
				t.Errorf("isMockFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateStatementCoverage(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		covered int
		want    string
	}{
		{
			name:    "full coverage",
			total:   100,
			covered: 100,
			want:    "100.0%",
		},
		{
			name:    "partial coverage",
			total:   100,
			covered: 75,
			want:    "75.0%",
		},
		{
			name:    "no coverage",
			total:   100,
			covered: 0,
			want:    "0.0%",
		},
		{
			name:    "zero total statements",
			total:   0,
			covered: 0,
			want:    "0.0%",
		},
		{
			name:    "decimal result",
			total:   3,
			covered: 2,
			want:    "66.7%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateStatementCoverage(tt.total, tt.covered)
			if got != tt.want {
				t.Errorf("calculateStatementCoverage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseStatementCoverage(t *testing.T) {
	// Create a temporary coverage file with known values.
	tempDir := t.TempDir()
	coverageFile := filepath.Join(tempDir, "coverage.out")

	// Create content with known coverage values.
	coverageContent := `mode: set
test/file1.go:10.2,12.3 2 1
test/file2.go:15.1,16.2 3 1
test/mock_service.go:5.1,6.2 1 0
`
	err := os.WriteFile(coverageFile, []byte(coverageContent), constants.DefaultFilePerms)
	if err != nil {
		t.Fatalf("Failed to create test coverage file: %v", err)
	}

	tests := []struct {
		name         string
		profileFile  string
		excludeMocks bool
		wantCoverage string
		wantFiltered []string
		wantErr      bool
	}{
		{
			name:         "valid coverage with mocks excluded",
			profileFile:  coverageFile,
			excludeMocks: true,
			wantCoverage: "100.0%", // 5 out of 5 statements covered (excluding mock)
			wantFiltered: []string{"test/mock_service.go"},
		},
		{
			name:         "valid coverage with mocks included",
			profileFile:  coverageFile,
			excludeMocks: false,
			wantCoverage: "83.3%", // 5 out of 6 statements covered
			wantFiltered: []string{},
		},
		{
			name:        "non-existent file",
			profileFile: "/non/existent/file",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCoverage, gotFiltered, err := parseStatementCoverage(tt.profileFile, tt.excludeMocks)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStatementCoverage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return // Skip further checks for error cases
			}
			if gotCoverage != tt.wantCoverage {
				t.Errorf("parseStatementCoverage() coverage = %v, want %v", gotCoverage, tt.wantCoverage)
			}
			if !reflect.DeepEqual(gotFiltered, tt.wantFiltered) {
				t.Errorf("parseStatementCoverage() filtered = %v, want %v", gotFiltered, tt.wantFiltered)
			}
		})
	}
}

func TestParseFunctionCoverageLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    CoverageFunction
		wantErr bool
	}{
		{
			name: "valid function line with coverage",
			line: "github.com/example/pkg/file.go:15:	functionName	75.0%",
			want: CoverageFunction{
				Function: "functionName",
				File:     "github.com/example/pkg/file.go",
				Coverage: 75.0,
			},
		},
		{
			name: "valid function line with zero coverage",
			line: "github.com/example/pkg/file.go:20:	anotherFunction	0.0%",
			want: CoverageFunction{
				Function: "anotherFunction",
				File:     "github.com/example/pkg/file.go",
				Coverage: 0.0,
			},
		},
		{
			name: "valid function line with full coverage",
			line: "github.com/example/pkg/file.go:25:	fullyCovered	100.0%",
			want: CoverageFunction{
				Function: "fullyCovered",
				File:     "github.com/example/pkg/file.go",
				Coverage: 100.0,
			},
		},
		{
			name:    "total line (should be skipped)",
			line:    "total:					(statements)	80.5%",
			wantErr: true,
		},
		{
			name:    "invalid format - missing parts",
			line:    "invalid line",
			wantErr: true,
		},
		{
			name:    "invalid format - bad percentage",
			line:    "file.go:10:	function	invalid%",
			wantErr: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFunctionCoverageLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFunctionCoverageLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFunctionCoverageLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCoverageProfile(t *testing.T) {
	// Use existing coverage file that has valid entries.
	coverageFile := "../../fixtures/test.coverage.out"

	tests := []struct {
		name         string
		profileFile  string
		excludeMocks bool
		wantErr      bool
		checkResult  bool
	}{
		{
			name:         "valid coverage profile with mocks excluded",
			profileFile:  coverageFile,
			excludeMocks: true,
			checkResult:  true,
		},
		{
			name:         "valid coverage profile with mocks included",
			profileFile:  coverageFile,
			excludeMocks: false,
			checkResult:  true,
		},
		{
			name:        "non-existent file",
			profileFile: "/non/existent/file",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoverageProfile(tt.profileFile, tt.excludeMocks)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCoverageProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkResult && got != nil {
				// Basic checks for valid result.
				if got.StatementCoverage == "" {
					t.Error("parseCoverageProfile() returned empty StatementCoverage")
				}
				// Note: FilteredFiles will only be non-empty if there are actual mock files in the coverage
				// Since we're using real coverage data, we just ensure the field exists
				if tt.excludeMocks && got.FilteredFiles == nil {
					t.Error("parseCoverageProfile() FilteredFiles should not be nil when excluding mocks")
				}
			}
		})
	}
}
