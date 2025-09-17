//go:build !windows
// +build !windows

// Package main provides a PTY wrapper for running gotcha with a pseudo-terminal.
// This is useful for testing TUI mode in headless environments like CI or AI agents.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/creack/pty"
	"github.com/spf13/viper"
	"golang.org/x/term"
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
		return filepath.Join(filepath.Dir(execPath), "gotcha")
	}
	return "gotcha"
}

// validateArguments checks that command-line arguments were provided.
func validateArguments(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s <gotcha-args>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s stream ./...\n", os.Args[0])
		os.Exit(1)
	}
}

// setupPTYResize handles PTY size changes.
func setupPTYResize(ptmx *os.File) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("Error resizing PTY: %s", err)
			}
		}
	}()

	ch <- syscall.SIGWINCH // Initial resize

	// Return cleanup function
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}

// setupRawMode sets up raw terminal mode if stdin is a terminal.
func setupRawMode() func() {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return func() {} // No-op cleanup
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Printf("Warning: could not set raw mode: %v", err)
		return func() {} // No-op cleanup
	}

	return func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}
}

// runWithPTY executes the command with PTY and handles I/O.
func runWithPTY(cmd *exec.Cmd) error {
	// Start the command with a PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start command with PTY: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Setup PTY resize handling
	cleanupResize := setupPTYResize(ptmx)
	defer cleanupResize()

	// Setup raw mode
	cleanupRaw := setupRawMode()
	defer cleanupRaw()

	// Copy stdin to the PTY and the PTY to stdout
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()
	_, _ = io.Copy(os.Stdout, ptmx)

	// Wait for the command to finish
	return cmd.Wait()
}

func main() {
	// Initialize configuration
	initializeConfig()

	// Find the gotcha binary
	gotchaBinary := findGotchaBinary()

	// Validate arguments
	args := os.Args[1:]
	validateArguments(args)

	// Create the command
	cmd := exec.Command(gotchaBinary, args...)

	// Run with PTY
	if err := runWithPTY(cmd); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("Command failed: %v", err)
	}
}
