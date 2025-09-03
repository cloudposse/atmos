package main

import (
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

func TestInitGlobalLogger(t *testing.T) {
	// Test that initGlobalLogger properly initializes the global logger
	initGlobalLogger()

	if globalLogger == nil {
		t.Error("initGlobalLogger() should initialize globalLogger")
	}

	// Test that logger can be used without panicking
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("globalLogger should not panic when used: %v", r)
		}
	}()

	globalLogger.Info("test log message")
}

func TestNewStreamCmd(t *testing.T) {
	logger := log.New(nil)

	cmd := newStreamCmd(logger)

	if cmd == nil {
		t.Error("newStreamCmd() should return a non-nil command")
	}

	if cmd.Use != "stream" {
		t.Errorf("newStreamCmd() Use = %v, want 'stream'", cmd.Use)
	}

	// Test that command has expected flags
	expectedFlags := []string{"packages", "show", "timeout", "output", "coverprofile", "exclude-mocks", "include", "exclude"}

	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("newStreamCmd() should have --%s flag", flagName)
		}
	}
}

func TestNewParseCmd(t *testing.T) {
	logger := log.New(nil)

	cmd := newParseCmd(logger)

	if cmd == nil {
		t.Error("newParseCmd() should return a non-nil command")
	}

	if cmd.Use != "parse [input-file]" {
		t.Errorf("newParseCmd() Use = %v, want 'parse [input-file]'", cmd.Use)
	}

	// Test that command has expected flags
	expectedFlags := []string{"input", "format", "output", "coverprofile", "exclude-mocks"}

	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("newParseCmd() should have --%s flag", flagName)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	logger := log.New(nil)

	cmd := newVersionCmd(logger)

	if cmd == nil {
		t.Error("newVersionCmd() should return a non-nil command")
	}

	if cmd.Use != "version" {
		t.Errorf("newVersionCmd() Use = %v, want 'version'", cmd.Use)
	}
}

// Test command execution with invalid parameters.
func TestRunStreamInvalidShow(t *testing.T) {
	// Create a mock command with invalid show parameter
	cmd := &cobra.Command{}
	cmd.Flags().String("packages", "", "")
	cmd.Flags().String("show", "invalid", "") // Invalid show value
	cmd.Flags().String("timeout", "40m", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("coverprofile", "", "")
	cmd.Flags().Bool("exclude-mocks", true, "")
	cmd.Flags().String("include", ".*", "")
	cmd.Flags().String("exclude", "", "")

	logger := log.New(nil)

	err := runStream(cmd, []string{}, logger)

	if err == nil {
		t.Error("runStream() should return error for invalid show parameter")
	}

	expectedError := "invalid show filter 'invalid'"
	if err.Error() != expectedError {
		t.Errorf("runStream() error = %v, want %v", err.Error(), expectedError)
	}
}

func TestRunParseWithInvalidInput(t *testing.T) {
	// Create a mock command with invalid input file
	cmd := &cobra.Command{}
	cmd.Flags().String("input", "/nonexistent/file.json", "")
	cmd.Flags().String("format", "stdin", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("coverprofile", "", "")
	cmd.Flags().Bool("exclude-mocks", true, "")

	logger := log.New(nil)

	err := runParse(cmd, []string{}, logger)

	if err == nil {
		t.Error("runParse() should return error for nonexistent input file")
	}
}

// Test the mock service function.
func TestMockService(t *testing.T) {
	result := MockService()

	// MockService should return false (based on typical mock behavior)
	if result != false {
		t.Errorf("MockService() = %v, want false", result)
	}
}
