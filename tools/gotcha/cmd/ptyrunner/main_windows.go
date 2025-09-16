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
)

func main() {
	// Get the gotcha binary path
	gotchaBinary := os.Getenv("GOTCHA_BINARY")
	if gotchaBinary == "" {
		// Try to find gotcha in the same directory as ptyrunner
		execPath, err := os.Executable()
		if err == nil {
			gotchaBinary = filepath.Join(filepath.Dir(execPath), "gotcha.exe")
		} else {
			gotchaBinary = "gotcha.exe"
		}
	}

	// Build command with all arguments passed to ptyrunner
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s <gotcha-args>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s stream ./...\n", os.Args[0])
		os.Exit(1)
	}

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