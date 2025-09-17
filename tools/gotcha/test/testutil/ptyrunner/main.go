//go:build !windows
// +build !windows

// Package main provides a gotcha-specific wrapper around the generic ptyrunner tool.
// This wrapper automatically finds the gotcha binary and passes arguments to it,
// making it convenient for testing gotcha's TUI mode in CI/automated tests.
//
// This is a thin wrapper that just calls: ptyrunner gotcha [args...]
//
// Note: PTYs are not supported on Windows, so this utility is Unix-only.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/viper"
)

// initializeConfig sets up viper configuration for environment variables.
func initializeConfig() {
	_ = viper.BindEnv("gotcha.binary", "GOTCHA_BINARY")
	_ = viper.BindEnv("ptyrunner.binary", "PTYRUNNER_BINARY")
	viper.AutomaticEnv()
}

// findBinary finds a binary, checking environment variable first, then same directory.
func findBinary(envKey string, defaultName string) string {
	binary := viper.GetString(envKey)
	if binary != "" {
		return binary
	}

	// Try to find binary in the same directory as this executable
	execPath, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(execPath), defaultName)
	}
	return defaultName
}

func main() {
	// Initialize configuration
	initializeConfig()

	// Find the ptyrunner binary (generic PTY wrapper)
	ptyrunnerBinary := findBinary("ptyrunner.binary", "ptyrunner")

	// Find the gotcha binary
	gotchaBinary := findBinary("gotcha.binary", "gotcha")

	// Validate arguments
	if len(os.Args) == 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s <gotcha-args>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s stream ./...\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "This is a convenience wrapper that runs: %s %s <gotcha-args>\n", ptyrunnerBinary, gotchaBinary)
		os.Exit(1)
	}

	// Build command: ptyrunner gotcha [args...]
	args := append([]string{gotchaBinary}, os.Args[1:]...)
	cmd := exec.Command(ptyrunnerBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Failed to run ptyrunner: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure the generic ptyrunner tool is built and available\n")
		os.Exit(1)
	}
}