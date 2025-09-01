package main

import (
	"testing"
)

func TestShouldShowErrorLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "error line with FAIL",
			line: "FAIL\tgithub.com/example/pkg\t0.123s",
			want: true,
		},
		{
			name: "error line with ERROR",
			line: "ERROR: test failed",
			want: true,
		},
		{
			name: "error line with WARN",
			line: "WARN: deprecated function",
			want: true,
		},
		{
			name: "panic line",
			line: "panic: runtime error",
			want: true,
		},
		{
			name: "compilation error",
			line: "# github.com/example/pkg",
			want: true,
		},
		{
			name: "build error",
			line: "./main.go:10:1: syntax error",
			want: true,
		},
		{
			name: "race condition",
			line: "WARNING: DATA RACE",
			want: true,
		},
		{
			name: "normal pass line",
			line: "PASS\tgithub.com/example/pkg\t0.123s",
			want: false,
		},
		{
			name: "test start line",
			line: "=== RUN   TestExample",
			want: false,
		},
		{
			name: "coverage line",
			line: "coverage: 85.0% of statements",
			want: false,
		},
		{
			name: "empty line",
			line: "",
			want: false,
		},
		{
			name: "whitespace only",
			line: "   \t   ",
			want: false,
		},
		{
			name: "normal output line",
			line: "    some_test.go:15: test output",
			want: false,
		},
		{
			name: "verbose test output",
			line: "=== CONT  TestExample",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldShowErrorLine(tt.line)
			
			if got != tt.want {
				t.Errorf("shouldShowErrorLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestShouldShowErrorLineEdgeCases(t *testing.T) {
	// Test case-insensitive matching
	errorLines := []string{
		"fail github.com/example/pkg",
		"FAIL github.com/example/pkg", 
		"Fail github.com/example/pkg",
		"error: something went wrong",
		"ERROR: something went wrong",
		"Error: something went wrong",
		"warning: deprecated",
		"WARNING: deprecated",
		"Warning: deprecated",
		"PANIC: runtime error",
		"panic: runtime error", 
		"Panic: runtime error",
	}

	for _, line := range errorLines {
		t.Run("error_line_"+line, func(t *testing.T) {
			if !shouldShowErrorLine(line) {
				t.Errorf("shouldShowErrorLine(%q) should return true for error line", line)
			}
		})
	}

	// Test normal lines that should not be shown as errors
	normalLines := []string{
		"PASS github.com/example/pkg",
		"ok  \tgithub.com/example/pkg\t0.123s",
		"testing: coverage disabled",
		"?   \tgithub.com/example/pkg\t[no test files]",
	}

	for _, line := range normalLines {
		t.Run("normal_line_"+line, func(t *testing.T) {
			if shouldShowErrorLine(line) {
				t.Errorf("shouldShowErrorLine(%q) should return false for normal line", line)
			}
		})
	}
}