package main

import (
	"testing"
)

func TestRun_InvalidFormat(t *testing.T) {
	exitCode := run("", "invalid-format", "")
	
	// Should exit with 1 for invalid format
	if exitCode != 1 {
		t.Errorf("Expected exit code 1 for invalid format, got %d", exitCode)
	}
}

func TestHandleOutput(t *testing.T) {
	summary := createTestSummary(2, 1, 0, "75.0%")
	consoleOutput := "test console output"
	
	tests := []struct {
		name       string
		format     string
		wantExit   int
	}{
		{
			name:     "console format",
			format:   formatConsole,
			wantExit: 1, // Summary has 1 failed test
		},
		{
			name:     "invalid format", 
			format:   "invalid",
			wantExit: 1,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := handleOutput(tt.format, "", summary, consoleOutput)
			if exitCode != tt.wantExit {
				t.Errorf("handleOutput() = %d, want %d", exitCode, tt.wantExit)
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