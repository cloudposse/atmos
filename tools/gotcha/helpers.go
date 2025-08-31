package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// filterPackages applies include/exclude regex patterns to filter packages
func filterPackages(packages []string, includePatterns, excludePatterns string) ([]string, error) {
	// If no packages provided, return as-is
	if len(packages) == 0 {
		return packages, nil
	}

	// Parse include patterns
	var includeRegexes []*regexp.Regexp
	if includePatterns != "" {
		for _, pattern := range strings.Split(includePatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid include pattern '%s': %v", pattern, err)
				}
				includeRegexes = append(includeRegexes, regex)
			}
		}
	}

	// Parse exclude patterns
	var excludeRegexes []*regexp.Regexp
	if excludePatterns != "" {
		for _, pattern := range strings.Split(excludePatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid exclude pattern '%s': %v", pattern, err)
				}
				excludeRegexes = append(excludeRegexes, regex)
			}
		}
	}

	// If no patterns specified, return original packages
	if len(includeRegexes) == 0 && len(excludeRegexes) == 0 {
		return packages, nil
	}

	// Filter packages
	var filtered []string
	for _, pkg := range packages {
		// Check include patterns (if any)
		included := len(includeRegexes) == 0 // Default to include if no include patterns
		for _, regex := range includeRegexes {
			if regex.MatchString(pkg) {
				included = true
				break
			}
		}

		// Check exclude patterns (if any)
		excluded := false
		for _, regex := range excludeRegexes {
			if regex.MatchString(pkg) {
				excluded = true
				break
			}
		}

		// Include if it matches include patterns and doesn't match exclude patterns
		if included && !excluded {
			filtered = append(filtered, pkg)
		}
	}

	return filtered, nil
}

// getTestCount uses AST parsing to quickly count Test and Example functions
func getTestCount(testPackages []string, testArgs string) int {
	globalLogger.Info("Pre-calculating test count using AST parsing", "packages", len(testPackages))

	totalTests := 0
	fset := token.NewFileSet()

	for _, pkg := range testPackages {
		// Handle special package patterns
		var searchDir string
		if pkg == "./..." {
			searchDir = "."
		} else if strings.HasSuffix(pkg, "/...") {
			searchDir = strings.TrimSuffix(pkg, "/...")
		} else {
			searchDir = pkg
		}

		// Walk through directories to find Go test files
		err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip non-Go files and non-test files
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}

			// Skip vendor directories and hidden directories
			if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.") {
				return nil
			}

			// Parse the Go file
			src, err := os.ReadFile(path)
			if err != nil {
				globalLogger.Warn("Failed to read test file", "file", path, "error", err)
				return nil
			}

			file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
			if err != nil {
				globalLogger.Warn("Failed to parse test file", "file", path, "error", err)
				return nil
			}

			// Count Test and Example functions
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					if fn.Name != nil {
						name := fn.Name.Name
						if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example") {
							totalTests++
						}
					}
				}
			}

			return nil
		})
		if err != nil {
			globalLogger.Warn("Failed to walk directory", "pkg", pkg, "error", err)
		}
	}

	globalLogger.Info("Test count discovery completed", "tests", totalTests, "packages", len(testPackages))
	return totalTests
}

// isTTY checks if we're running in a terminal and Bubble Tea can actually use it
func isTTY() bool {
	// Provide an environment override
	if os.Getenv("FORCE_NO_TTY") != "" {
		return false
	}

	// Debug: Force TTY mode for testing (but only if TTY is actually usable)
	if os.Getenv("FORCE_TTY") != "" {
		// Still check if we can actually open /dev/tty
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			tty.Close()
			return true
		}
		// If we can't open /dev/tty, fall back to normal detection
	}

	// Check if both stdin and stdout are terminals
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	isStdoutTTY := (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice

	stat, err = os.Stdin.Stat()
	if err != nil {
		return false
	}
	isStdinTTY := (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice

	// Most importantly, check if we can actually open /dev/tty
	// This is what Bubble Tea will try to do
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err != nil {
		return false
	} else {
		tty.Close()
	}

	return isStdoutTTY && isStdinTTY
}

// runSimpleStream runs tests with simple non-interactive streaming output
func runSimpleStream(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, totalTests int) int {
	// For now, return a placeholder implementation
	// This would contain the full streaming implementation
	fmt.Fprintf(os.Stderr, "Simple streaming not yet implemented\n")
	return 0
}

// handleOutput handles writing output in the specified format
func handleOutput(summary *TestSummary, format, outputFile string) error {
	switch format {
	case "stdin":
		return handleConsoleOutput(summary)
	case "markdown":
		return writeSummary(summary, format, outputFile)
	case "github":
		return writeSummary(summary, format, outputFile)
	case "both":
		if err := handleConsoleOutput(summary); err != nil {
			return err
		}
		return writeSummary(summary, "markdown", outputFile)
	}
	return fmt.Errorf("unsupported format: %s", format)
}

// handleConsoleOutput writes console-formatted output
func handleConsoleOutput(summary *TestSummary) error {
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	if len(summary.Failed) > 0 {
		fmt.Print("test failed")
	} else {
		fmt.Printf("test console output")
	}

	if total > 0 || summary.Coverage != "" {
		// Add coverage if available
		if summary.Coverage != "" {
			fmt.Printf("Coverage: %s\n", summary.Coverage)
		}
	}

	return nil
}
