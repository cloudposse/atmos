package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	outputPath := filepath.Join(root, "NOTICE")
	if len(os.Args) > 2 {
		outputPath = os.Args[2]
	}

	summary, err := Generate(root, outputPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "NOTICE file generated successfully: %s\n", outputPath)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Summary:")
	fmt.Fprintf(os.Stderr, "  - Total dependencies: %d\n", summary.Total)
	fmt.Fprintf(os.Stderr, "  - Apache-2.0: %d\n", summary.Apache)
	fmt.Fprintf(os.Stderr, "  - BSD: %d\n", summary.BSD)
	if summary.MPL > 0 {
		fmt.Fprintf(os.Stderr, "  - MPL-2.0: %d\n", summary.MPL)
	}
	if summary.MIT > 0 {
		fmt.Fprintf(os.Stderr, "  - MIT: %d\n", summary.MIT)
	}
	return nil
}
