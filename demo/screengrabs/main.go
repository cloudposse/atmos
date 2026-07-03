// Command screengrabs regenerates the documentation screengrab artifacts.
//
// It reads a manifest of commands (one per line, # comments allowed), records
// each command's output as an asciicast via pkg/asciicast, and renders static
// .html and .ascii artifacts consumed by the website Screengrab component.
// This replaces the legacy bash/sed/aha pipeline: no external ANSI-to-HTML
// converter and no platform-specific sed hacks are required.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <manifest.txt>\n", os.Args[0]) //nolint:gosec // CLI usage text written to a terminal, not a browser.
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
