package tests

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func verifyOS(t *testing.T, osPatterns []MatchPattern) bool {
	currentOS := runtime.GOOS // Get the current operating system
	success := true

	for _, pattern := range osPatterns {
		// Compile the regex pattern
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			t.Errorf("Invalid OS regex pattern: %q, error: %v", pattern.Pattern, err)
			success = false
			continue
		}

		// Check if the current OS matches the pattern
		match := re.MatchString(currentOS)
		if pattern.Negate && match {
			logger.Info("Reason: OS matched negated pattern", "os", currentOS, "pattern", pattern.Pattern)
			success = false
		} else if !pattern.Negate && !match {
			logger.Info("Reason: OS did not match pattern", "os", currentOS, "pattern", pattern.Pattern)
			success = false
		}
	}

	return success
}

func verifyExitCode(t *testing.T, expected, actual int) bool {
	success := true
	if expected != actual {
		t.Errorf("Reason: Expected exit code %d, got %d", expected, actual)
		success = false
	}
	return success
}

func verifyOutput(t *testing.T, outputType, output string, patterns []MatchPattern) bool {
	success := true
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			t.Errorf("Invalid %s regex: %q, error: %v", outputType, pattern.Pattern, err)
			success = false
			continue
		}

		match := re.MatchString(output)
		if pattern.Negate && match {
			t.Errorf("Reason: %s unexpectedly matched negated pattern %q.", outputType, pattern.Pattern)
			t.Errorf("Output: %q", output)
			success = false
		} else if !pattern.Negate && !match {
			t.Errorf("Reason: %s did not match pattern %q.", outputType, pattern.Pattern)
			t.Errorf("Output: %q", output)
			success = false
		}
	}
	return success
}

// resolveFilePaths resolves relative file paths against baseDir.
// Absolute paths are returned unchanged.
// This is needed because we set cmd.Dir instead of t.Chdir, so relative
// paths in test expectations must be anchored to the workdir explicitly.
//
// When baseDir is empty (tests without a workdir), the original slice is
// returned unmodified. Relative paths in those tests are resolved against the
// test binary's starting directory (recorded in startingDir by TestMain),
// which is the tests/ directory. This is correct because t.Chdir is never
// used (it is incompatible with t.Parallel), so the process CWD stays fixed.
func resolveFilePaths(files []string, baseDir string) []string {
	if baseDir == "" || len(files) == 0 {
		return files
	}
	resolved := make([]string, len(files))
	for i, f := range files {
		if filepath.IsAbs(f) {
			resolved[i] = f
		} else {
			resolved[i] = filepath.Join(baseDir, f)
		}
	}
	return resolved
}

// resolveFilePathsMap resolves relative file paths in a map[string][]MatchPattern
// against baseDir. Absolute paths are returned unchanged.
// See resolveFilePaths for the empty-baseDir semantics.
func resolveFilePathsMap(filePatterns map[string][]MatchPattern, baseDir string) map[string][]MatchPattern {
	if baseDir == "" || len(filePatterns) == 0 {
		return filePatterns
	}
	resolved := make(map[string][]MatchPattern, len(filePatterns))
	for f, patterns := range filePatterns {
		if filepath.IsAbs(f) {
			resolved[f] = patterns
		} else {
			resolved[filepath.Join(baseDir, f)] = patterns
		}
	}
	return resolved
}

func verifyFileExists(t *testing.T, files []string) bool {
	success := true
	for _, file := range files {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			t.Errorf("Reason: Expected file does not exist: %q", file)
			success = false
		}
	}
	return success
}

func verifyFileNotExists(t *testing.T, files []string) bool {
	success := true
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			t.Errorf("Reason: File %q exists but it should not.", file)
			success = false
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Reason: Unexpected error checking file %q: %v", file, err)
			success = false
		}
	}
	return success
}

func verifyFileContains(t *testing.T, filePatterns map[string][]MatchPattern) bool {
	success := true
	for file, patterns := range filePatterns {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("Reason: Failed to read file %q: %v", file, err)
			success = false
			continue
		}
		for _, matchPattern := range patterns {
			re, err := regexp.Compile(matchPattern.Pattern)
			if err != nil {
				t.Errorf("Invalid regex for file %q: %q, error: %v", file, matchPattern.Pattern, err)
				success = false
				continue
			}
			if matchPattern.Negate {
				// Negated pattern: Ensure the pattern does NOT match
				if re.Match(content) {
					t.Errorf("Reason: File %q unexpectedly matched negated pattern %q.", file, matchPattern.Pattern)
					t.Errorf("Content: %q", string(content))
					success = false
				}
			} else {
				// Regular pattern: Ensure the pattern matches
				if !re.Match(content) {
					t.Errorf("Reason: File %q did not match pattern %q.", file, matchPattern.Pattern)
					t.Errorf("Content: %q", string(content))
					success = false
				}
			}
		}
	}
	return success
}

func verifyFormatValidation(t *testing.T, output string, formats []string) bool {
	for _, format := range formats {
		switch format {
		case "json":
			if !verifyJSONFormat(t, output) {
				return false
			}
		case "yaml":
			if !verifyYAMLFormat(t, output) {
				return false
			}
		default:
			t.Logf("Unknown format: %s", format)
			return false
		}
	}
	return true
}

func verifyYAMLFormat(t *testing.T, output string) bool {
	var data interface{}
	err := yaml.Unmarshal([]byte(output), &data)
	if err != nil {
		t.Logf("YAML validation failed: %v", err)
		// Show context around the error if possible.
		lines := strings.Split(output, "\n")
		preview := strings.Join(lines[:min(10, len(lines))], "\n")
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		t.Logf("Output preview:\n%s", preview)
		return false
	}
	return true
}

func verifyJSONFormat(t *testing.T, output string) bool {
	var data interface{}
	err := json.Unmarshal([]byte(output), &data)
	if err != nil {
		t.Logf("JSON validation failed: %v", err)
		// Try to provide context about where the error occurred.
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			offset := syntaxErr.Offset
			// Show a snippet around the error location.
			start := max(0, int(offset)-50)
			end := min(len(output), int(offset)+50)
			snippet := output[start:end]
			t.Logf("Error at offset %d, context: ...%s...", offset, snippet)
		}
		return false
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// validateYAMLFormatSilent checks YAML validity without logging errors.
// Used in tests that expect validation to fail.
func validateYAMLFormatSilent(output string) bool {
	var data interface{}
	err := yaml.Unmarshal([]byte(output), &data)
	return err == nil
}

// validateJSONFormatSilent checks JSON validity without logging errors.
// Used in tests that expect validation to fail.
func validateJSONFormatSilent(output string) bool {
	var data interface{}
	err := json.Unmarshal([]byte(output), &data)
	return err == nil
}

// validateFormatValidationSilent checks if output is valid in any of the specified formats.
// Used in tests that expect validation to fail.
func validateFormatValidationSilent(output string, formats []string) bool {
	for _, format := range formats {
		switch format {
		case "json":
			if !validateJSONFormatSilent(output) {
				return false
			}
		case "yaml":
			if !validateYAMLFormatSilent(output) {
				return false
			}
		default:
			// Unknown format - return false without logging.
			return false
		}
	}
	return true
}

// verifyTestOutputs validates test outputs based on TTY mode.
func verifyTestOutputs(t *testing.T, tc *TestCase, stdout, stderr string) {
	if tc.Tty {
		// TTY mode: validate combined output against tty expectations
		if !verifyOutput(t, "tty", stdout, tc.Expect.Tty) {
			t.Errorf("TTY output mismatch for test: %s", tc.Name)
		}
		return
	}

	// Non-TTY mode: validate stdout and stderr separately
	if !verifyOutput(t, "stdout", stdout, tc.Expect.Stdout) {
		t.Errorf("Stdout mismatch for test: %s", tc.Name)
	}

	if !verifyOutput(t, "stderr", stderr, tc.Expect.Stderr) {
		t.Errorf("Stderr mismatch for test: %s", tc.Name)
	}
}
