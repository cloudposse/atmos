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

	fmt.Printf("NOTICE file generated successfully: %s\n", outputPath)
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  - Total dependencies: %d\n", summary.Total)
	fmt.Printf("  - Apache-2.0: %d\n", summary.Apache)
	fmt.Printf("  - BSD: %d\n", summary.BSD)
	if summary.MPL > 0 {
		fmt.Printf("  - MPL-2.0: %d\n", summary.MPL)
	}
	if summary.MIT > 0 {
		fmt.Printf("  - MIT: %d\n", summary.MIT)
	}
	return nil
}
