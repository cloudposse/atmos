package test

import (
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/internal/output"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

func TestHandleOutput(t *testing.T) {
	summary := &types.TestSummary{
		Passed:   []types.TestResult{{Package: "test/pkg", Test: "TestPass1", Status: "pass", Duration: 0.5}, {Package: "test/pkg", Test: "TestPass2", Status: "pass", Duration: 0.3}},
		Failed:   []types.TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
		Coverage: "75.0%",
	}

	tests := []struct {
		name      string
		format    string
		wantError bool
	}{
		{
			name:      "terminal format",
			format:    "terminal",
			wantError: false,
		},
		{
			name:      "stdin format (backward compat)",
			format:    "stdin",
			wantError: false,
		},
		{
			name:      "markdown format",
			format:    "markdown",
			wantError: false,
		},
		{
			name:      "invalid format",
			format:    "invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := output.HandleOutput(summary, tt.format, "", true)
			hasError := (err != nil)
			if hasError != tt.wantError {
				t.Errorf("HandleOutput() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestOpenInput_WithFiles(t *testing.T) {
	// Test reading from a file that exists
	testData := `{"Time":"2023-01-01T00:00:00Z","Action":"pass","Package":"test/pkg","Test":"TestExample"}`

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(testData)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test file input using os.Open (which is used by parse mode)
	file, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Errorf("Expected to open file successfully, got error: %v", err)
	}
	defer file.Close()

	// Read some data to verify it works
	buf := make([]byte, len(testData))
	n, err := file.Read(buf)
	if err != nil {
		t.Errorf("Expected to read from file, got error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
	}
	if !strings.Contains(string(buf), "TestExample") {
		t.Errorf("Expected test data to contain TestExample")
	}
}

func TestNonExistentFile(t *testing.T) {
	_, err := os.Open("nonexistent.json")
	if err == nil {
		t.Errorf("Expected error when opening nonexistent file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Expected file not exist error, got: %v", err)
	}
}
