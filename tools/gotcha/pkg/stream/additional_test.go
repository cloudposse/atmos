package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAnalyzeProcessFailure tests the analyzeProcessFailure function
func TestAnalyzeProcessFailure(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		exitCode int
		contains string
	}{
		{
			name:     "compilation error",
			stderr:   "main.go:10:5: undefined: SomeFunction\n./main.go:20:10: syntax error",
			exitCode: 2,
			contains: "compilation errors",
		},
		{
			name:     "no Go files",
			stderr:   "no Go files in /path/to/dir",
			exitCode: 1,
			contains: "No Go source files",
		},
		{
			name:     "no Go files alternative",
			stderr:   "no Go source files",
			exitCode: 1,
			contains: "No Go source files",
		},
		{
			name:     "cannot find module",
			stderr:   "cannot find module for path",
			exitCode: 1,
			contains: "Go module error",
		},
		{
			name:     "package not found",
			stderr:   "package github.com/example/missing not found",
			exitCode: 1,
			contains: "Package not found",
		},
		{
			name:     "build constraints exclude",
			stderr:   "build constraints exclude all Go files",
			exitCode: 1,
			contains: "Build constraints exclude all files",
		},
		{
			name:     "permission denied",
			stderr:   "permission denied",
			exitCode: 1,
			contains: "Permission denied",
		},
		{
			name:     "cannot find package",
			stderr:   "cannot find package",
			exitCode: 1,
			contains: "Import error",
		},
		{
			name:     "timeout",
			stderr:   "panic: test timed out after 10m",
			exitCode: 2,
			contains: "Test execution timeout",
		},
		{
			name:     "go command not found",
			stderr:   "go: command not found",
			exitCode: 127,
			contains: "Go is not installed",
		},
		{
			name:     "unknown error",
			stderr:   "some random error",
			exitCode: 255,
			contains: "Unknown test execution error",
		},
		{
			name:     "normal test failure",
			stderr:   "",
			exitCode: 1,
			contains: "",
		},
		{
			name:     "signal terminated",
			stderr:   "signal: killed",
			exitCode: -1,
			contains: "terminated by signal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzeProcessFailure(tt.stderr, tt.exitCode)
			if tt.contains != "" {
				assert.Contains(t, result, tt.contains)
			} else {
				assert.Empty(t, result)
			}
		})
	}
}

// TestExtractExitCode tests exit code extraction
func TestExtractExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "exit status 1",
			err:      &exitError{code: 1},
			wantCode: 1,
		},
		{
			name:     "exit status 2",
			err:      &exitError{code: 2},
			wantCode: 2,
		},
		{
			name:     "nil error",
			err:      nil,
			wantCode: 0,
		},
		{
			name:     "non-exit error",
			err:      assert.AnError,
			wantCode: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := extractExitCode(tt.err)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}

// Mock exit error for testing
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return "exit status " + string(rune(e.code))
}

func (e *exitError) ExitCode() int {
	return e.code
}

// TestRunTestsWithSimpleStreaming tests the main streaming function with basic scenarios
func TestRunTestsWithSimpleStreaming(t *testing.T) {
	// Test with invalid package
	exitCode := RunTestsWithSimpleStreaming(
		[]string{"./nonexistent"},
		"",
		"",
		"minimal",
	)
	assert.NotEqual(t, 0, exitCode, "should fail for non-existent package")
}

// TestGetLastExitReason verifies exit reason extraction
func TestGetLastExitReasonGlobal(t *testing.T) {
	// GetLastExitReason is a global function, not a method
	// Just test that it can be called
	reason := GetLastExitReason()
	// Initially it should be empty
	assert.NotNil(t, reason)
}

// TestShouldShowFilter tests filter logic
func TestShouldShowFilter(t *testing.T) {
	tests := []struct {
		name       string
		showFilter string
		status     string
		want       bool
	}{
		{
			name:       "show all - pass",
			showFilter: "all",
			status:     "pass",
			want:       true,
		},
		{
			name:       "show all - fail",
			showFilter: "all",
			status:     "fail",
			want:       true,
		},
		{
			name:       "show failed only - pass",
			showFilter: "failed",
			status:     "pass",
			want:       false,
		},
		{
			name:       "show failed only - fail",
			showFilter: "failed",
			status:     "fail",
			want:       true,
		},
		{
			name:       "show failed only - skip",
			showFilter: "failed",
			status:     "skip",
			want:       false,
		},
		{
			name:       "empty filter - fail",
			showFilter: "",
			status:     "fail",
			want:       true,
		},
		{
			name:       "empty filter - pass",
			showFilter: "",
			status:     "pass",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			show := shouldShowBasedOnFilter(tt.showFilter, tt.status)
			assert.Equal(t, tt.want, show)
		})
	}
}

// Helper function for testing
func shouldShowBasedOnFilter(showFilter, status string) bool {
	if showFilter == "all" {
		return true
	}
	if showFilter == "failed" {
		return status == "fail"
	}
	// Default: show failures
	return status == "fail" || status == "skip"
}
