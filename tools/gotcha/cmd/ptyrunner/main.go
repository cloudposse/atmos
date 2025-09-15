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
	"golang.org/x/term"
)

func main() {
	// Get the gotcha binary path
	gotchaBinary := os.Getenv("GOTCHA_BINARY")
	if gotchaBinary == "" {
		// Try to find gotcha in the same directory as ptyrunner
		execPath, err := os.Executable()
		if err == nil {
			gotchaBinary = filepath.Join(filepath.Dir(execPath), "gotcha")
		} else {
			gotchaBinary = "gotcha"
		}
	}

	// Build command with all arguments passed to ptyrunner
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s <gotcha-args>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s stream ./...\n", os.Args[0])
		os.Exit(1)
	}

	// Create the command
	cmd := exec.Command(gotchaBinary, args...)

	// Start the command with a PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("Failed to start command with PTY: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Handle PTY size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("Error resizing PTY: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH                        // Initial resize
	defer func() { signal.Stop(ch); close(ch) }() // Cleanup signals when done

	// Check if stdin is a terminal
	if term.IsTerminal(int(os.Stdin.Fd())) {
		// Set stdin in raw mode for interactive use
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			log.Printf("Warning: could not set raw mode: %v", err)
		} else {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		}
	}

	// Copy stdin to the PTY and the PTY to stdout
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()
	_, _ = io.Copy(os.Stdout, ptmx)

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("Command failed: %v", err)
	}
}
