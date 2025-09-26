//go:build !windows
// +build !windows

// Package main provides a generic PTY (pseudo-terminal) wrapper for running commands.
// This tool wraps any command in a PTY, which is useful for:
//   - Testing TUI applications in CI/headless environments
//   - Running interactive commands that require a TTY
//   - Capturing output from programs that behave differently when connected to a terminal
//
// Note: PTYs are not supported on Windows, so this utility is Unix-only.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// validateArguments checks that command-line arguments were provided.
func validateArguments(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [args...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s ls -la\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s gotcha stream ./...\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s vim file.txt\n", os.Args[0])
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
	// Validate arguments
	args := os.Args[1:]
	validateArguments(args)

	// Create the command
	cmd := exec.Command(args[0], args[1:]...)

	// Run with PTY
	if err := runWithPTY(cmd); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("Command failed: %v", err)
	}
}
