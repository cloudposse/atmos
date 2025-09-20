//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintf(os.Stderr, "Error: ptyrunner is not supported on Windows\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "PTYs (pseudo-terminals) are a Unix/Linux concept and are not available on Windows.\n")
	fmt.Fprintf(os.Stderr, "Windows uses different mechanisms for console I/O that are incompatible with PTY.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Alternatives for Windows:\n")
	fmt.Fprintf(os.Stderr, "  - Use Windows Terminal or ConPTY API for terminal emulation\n")
	fmt.Fprintf(os.Stderr, "  - Run the command directly without PTY wrapping\n")
	fmt.Fprintf(os.Stderr, "  - Use WSL (Windows Subsystem for Linux) if Unix compatibility is needed\n")
	os.Exit(1)
}
