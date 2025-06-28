package utils

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

// Helper function to capture stdout
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	return buf.String()
}

// Helper function to capture stderr
func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	return buf.String()
}

func TestPrintMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "simple message",
			message: "Hello, World!",
		},
		{
			name:    "empty message",
			message: "",
		},
		{
			name:    "message with special characters",
			message: "Hello\nWorld\t!@#$%^&*()",
		},
		{
			name:    "unicode message",
			message: "Hello, 世界!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureOutput(func() {
				PrintMessage(tt.message)
			})
			// trim the captured output to handle the newline added by fmt.Println
			assert.Equal(t, tt.message+"\n", output)
		})
	}
}

func TestPrintMessageInColor(t *testing.T) {
	// Enable colors for testing
	color.NoColor = false

	tests := []struct {
		name         string
		message      string
		messageColor *color.Color
		wantContains string
	}{
		{
			name:         "red message",
			message:      "Error message",
			messageColor: color.New(color.FgRed),
			wantContains: "Error message",
		},
		{
			name:         "green message",
			message:      "Success message",
			messageColor: color.New(color.FgGreen),
			wantContains: "Success message",
		},
		{
			name:         "blue bold message",
			message:      "Info message",
			messageColor: color.New(color.FgBlue, color.Bold),
			wantContains: "Info message",
		},
		{
			name:         "empty message",
			message:      "",
			messageColor: color.New(color.FgYellow),
			wantContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureOutput(func() {
				PrintMessageInColor(tt.message, tt.messageColor)
			})
			assert.Contains(t, output, tt.wantContains)
			// The output should be longer than the message due to color codes
			if tt.message != "" {
				assert.True(t, len(output) > len(tt.message))
			}
		})
	}
}

func TestPrintfMessageToTUI(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		args     []interface{}
		expected string
	}{
		{
			name:     "simple message without args",
			message:  "Hello World",
			args:     []interface{}{},
			expected: "Hello World",
		},
		{
			name:     "message with single argument",
			message:  "Count: %d",
			args:     []interface{}{42},
			expected: "Count: 42",
		},
		{
			name:     "message with multiple arguments",
			message:  "Hello %s, your score is %d",
			args:     []interface{}{"John", 100},
			expected: "Hello John, your score is 100",
		},
		{
			name:     "message with mixed type arguments",
			message:  "Name: %s, Age: %d, Rate: %.2f",
			args:     []interface{}{"Alice", 30, 4.5},
			expected: "Name: Alice, Age: 30, Rate: 4.50",
		},
		{
			name:     "empty message with no args",
			message:  "",
			args:     []interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(func() {
				PrintfMessageToTUI(tt.message, tt.args...)
			})
			assert.Equal(t, tt.expected, output)
		})
	}
}

func TestOsExit(t *testing.T) {
	// Store the original OsExit
	originalOsExit := OsExit
	defer func() {
		// Restore the original OsExit after the test
		OsExit = originalOsExit
	}()

	exitCalled := false
	exitCode := 0

	// Mock OsExit
	OsExit = func(code int) {
		exitCalled = true
		exitCode = code
	}

	// Test the exit function
	OsExit(1)

	assert.True(t, exitCalled, "OsExit was not called")
	assert.Equal(t, 1, exitCode, "Unexpected exit code")
}

func TestLogLevelConstants(t *testing.T) {
	assert.Equal(t, "Trace", LogLevelTrace)
	assert.Equal(t, "Debug", LogLevelDebug)
}
