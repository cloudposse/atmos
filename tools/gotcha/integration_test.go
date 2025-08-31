package main

import (
	"testing"
)

func TestRun_InvalidFormat(t *testing.T) {
	exitCode := run("", "invalid-format", "", "", false)

	// Should exit with 1 for invalid format.
	if exitCode != 1 {
		t.Errorf("Expected exit code 1 for invalid format, got %d", exitCode)
	}
}

func TestHandleOutput(t *testing.T) {
	summary := &TestSummary{
		Passed:   []TestResult{{Package: "test/pkg", Test: "TestPass1", Status: "pass", Duration: 0.5}, {Package: "test/pkg", Test: "TestPass2", Status: "pass", Duration: 0.3}},
		Failed:   []TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
		Coverage: "75.0%",
	}

	tests := []struct {
		name      string
		format    string
		wantError bool
	}{
		{
			name:      "console format",
			format:    formatStdin,
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
			err := handleOutput(summary, tt.format, "")
			hasError := (err != nil)
			if hasError != tt.wantError {
				t.Errorf("handleOutput() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestOpenInput(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "stdin",
			filename: stdinMarker,
			wantErr:  false,
		},
		{
			name:     "nonexistent file",
			filename: "nonexistent.json",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := openInput(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("openInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
