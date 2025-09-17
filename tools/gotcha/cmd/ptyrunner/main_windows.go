//go:build windows
// +build windows

// Package main provides a simple wrapper for running gotcha on Windows.
// PTY functionality is not available on Windows, so this is a passthrough.
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
	viper.AutomaticEnv()
}

// findGotchaBinary determines the path to the gotcha binary.
func findGotchaBinary() string {
	gotchaBinary := viper.GetString("gotcha.binary")
	if gotchaBinary != "" {
		return gotchaBinary
	}

	// Try to find gotcha in the same directory as ptyrunner
	execPath, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(execPath), "gotcha.exe")
	}
	return "gotcha.exe"
}

// validateArguments checks that command-line arguments were provided.
func validateArguments(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s <gotcha-args>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s stream ./...\n", os.Args[0])
		os.Exit(1)
	}
}

func main() {
	// Initialize configuration
	initializeConfig()

	// Find the gotcha binary
	gotchaBinary := findGotchaBinary()

	// Validate arguments
	args := os.Args[1:]
	validateArguments(args)

	// Create and run the command directly (no PTY on Windows)
	cmd := exec.Command(gotchaBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Command failed: %v\n", err)
		os.Exit(1)
	}
}
