package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	goModPath := "go.mod"
	if len(os.Args) > 1 {
		goModPath = os.Args[1]
	}

	file, err := os.Open(goModPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", goModPath, err)
	}
	defer file.Close()

	var replaceDirectives []string
	var excludeDirectives []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for replace directives (inline or block start).
		if strings.HasPrefix(trimmed, "replace ") {
			replaceDirectives = append(replaceDirectives, line)
		}

		// Check for exclude directives (inline or block start).
		if strings.HasPrefix(trimmed, "exclude ") {
			excludeDirectives = append(excludeDirectives, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading %s: %w", goModPath, err)
	}

	// Report errors.
	if len(replaceDirectives) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: go.mod contains 'replace' directives which break 'go install'.\n\n")
		fmt.Fprintf(os.Stderr, "Replace directives found:\n")
		for _, directive := range replaceDirectives {
			fmt.Fprintf(os.Stderr, "  %s\n", directive)
		}
		fmt.Fprintf(os.Stderr, "\nThis breaks a documented installation method for Atmos.\n")
		fmt.Fprintf(os.Stderr, "Consider alternative approaches that don't break go install compatibility.\n")
		return fmt.Errorf("replace directives not allowed")
	}

	if len(excludeDirectives) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: go.mod contains 'exclude' directives which break 'go install'.\n\n")
		fmt.Fprintf(os.Stderr, "Exclude directives found:\n")
		for _, directive := range excludeDirectives {
			fmt.Fprintf(os.Stderr, "  %s\n", directive)
		}
		fmt.Fprintf(os.Stderr, "\nThis breaks a documented installation method for Atmos.\n")
		fmt.Fprintf(os.Stderr, "Consider alternative approaches that don't break go install compatibility.\n")
		return fmt.Errorf("exclude directives not allowed")
	}

	fmt.Println("✓ go.mod is compatible with 'go install'")
	return nil
}
