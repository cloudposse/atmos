package utils

import (
	"regexp"
	"strings"
)

// Common test name patterns in Go.
var (
	testNamePattern      = regexp.MustCompile(`^(Test|Example|Benchmark)([A-Z]|_).*`)
	subtestPattern       = regexp.MustCompile(`^(Test|Example|Benchmark)([A-Z]|_).*/.*`)
	multipleTestPattern  = regexp.MustCompile(`^(Test|Example|Benchmark)([A-Z]|_).*\|(Test|Example|Benchmark)([A-Z]|_).*`)
)

// IsLikelyTestName checks if a string looks like a Go test name.
func IsLikelyTestName(s string) bool {
	// Check if it matches common test patterns
	if testNamePattern.MatchString(s) {
		return true
	}
	
	// Check for subtest patterns (TestFoo/subtest)
	if subtestPattern.MatchString(s) {
		return true
	}
	
	// Check for multiple tests with | separator
	if multipleTestPattern.MatchString(s) {
		return true
	}
	
	return false
}

// ExtractTestNamesFromArgs processes arguments to extract test names and package paths.
func ExtractTestNamesFromArgs(args []string) (testFilter string, packages []string) {
	var testNames []string
	
	for _, arg := range args {
		// Skip flags and flag values
		if strings.HasPrefix(arg, "-") {
			continue
		}
		
		// Check if it looks like a test name
		if IsLikelyTestName(arg) {
			testNames = append(testNames, arg)
		} else {
			// It's likely a package path
			packages = append(packages, arg)
		}
	}
	
	// Combine test names with | for regex OR
	if len(testNames) > 0 {
		testFilter = strings.Join(testNames, "|")
	}
	
	return testFilter, packages
}

// HasRunFlag checks if the -run flag is already present in the arguments.
func HasRunFlag(args []string) bool {
	for i, arg := range args {
		if arg == "-run" || arg == "--run" {
			return true
		}
		// Check for -run=value format
		if strings.HasPrefix(arg, "-run=") || strings.HasPrefix(arg, "--run=") {
			return true
		}
		// Check for combined short flags like -vrun
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(arg, "run") {
			// This is a bit tricky, but we'll be conservative and assume it might be -run
			// combined with other flags
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				return true
			}
		}
	}
	return false
}

// IsPackagePath checks if a string looks like a Go package path.
func IsPackagePath(s string) bool {
	// Common package path patterns
	if s == "." || s == ".." {
		return true
	}
	
	// Paths starting with ./ or ../
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	
	// Recursive pattern
	if strings.HasSuffix(s, "/...") {
		return true
	}
	
	// Absolute paths
	if strings.HasPrefix(s, "/") {
		return true
	}
	
	// Module paths (contains dots but not starting with Test/Example/Benchmark)
	if strings.Contains(s, "/") && !IsLikelyTestName(s) {
		return true
	}
	
	return false
}

// ProcessTestArguments intelligently processes command arguments to detect test names.
func ProcessTestArguments(args []string) (packages []string, testFilter string) {
	var detectedTests []string
	
	for _, arg := range args {
		// First check if it's definitely a package path
		if IsPackagePath(arg) {
			packages = append(packages, arg)
		} else if IsLikelyTestName(arg) {
			// It looks like a test name
			detectedTests = append(detectedTests, arg)
		} else {
			// Ambiguous - could be a package name without path indicators
			// Default to treating as package for backward compatibility
			packages = append(packages, arg)
		}
	}
	
	// Combine test names for -run filter
	if len(detectedTests) > 0 {
		testFilter = strings.Join(detectedTests, "|")
	}
	
	return packages, testFilter
}