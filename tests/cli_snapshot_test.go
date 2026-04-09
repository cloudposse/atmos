package tests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/term"
)

func updateSnapshot(fullPath, output string) {
	err := os.MkdirAll(filepath.Dir(fullPath), 0o755) // Ensure parent directories exist
	if err != nil {
		panic(fmt.Sprintf("Failed to create snapshot directory: %v", err))
	}
	// Normalize line endings to LF for cross-platform consistency.
	// This ensures snapshots work reliably across Windows, macOS, and Linux.
	normalized := normalizeLineEndings(output)
	err = os.WriteFile(fullPath, []byte(normalized), 0o644) // Write snapshot
	if err != nil {
		panic(fmt.Sprintf("Failed to write snapshot file: %v", err))
	}
}

func readSnapshot(t *testing.T, fullPath string) string {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Error reading snapshot file %q: %v", fullPath, err)
	}
	// Normalize line endings when reading to gracefully handle any existing
	// snapshots that were committed with CRLF line endings.
	return normalizeLineEndings(string(data))
}

// normalizeLineEndings converts CRLF line endings to LF for cross-platform consistency.
// This ensures snapshots work reliably across Windows, macOS, and Linux development.
//
// Important: Only CRLF sequences (\r\n) are converted to LF (\n).
// Standalone CR (\r) characters are preserved, as they're used by spinners and
// progress indicators to overwrite terminal lines.
//
// Examples:
//   - "line1\r\nline2\r\n" → "line1\nline2\n" (CRLF normalized)
//   - "line1\nline2\n" → "line1\nline2\n" (LF unchanged)
//   - "Progress\r" → "Progress\r" (spinner CR preserved)
func normalizeLineEndings(s string) string {
	// Only replace CRLF with LF, preserve standalone CR for spinners.
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// Generate a unified diff using gotextdiff.
// Colors are suppressed when stderr is not a TTY (e.g. CI, piped output).
func generateUnifiedDiff(actual, expected string) string {
	edits := myers.ComputeEdits(span.URIFromPath("actual"), expected, actual)
	unified := gotextdiff.ToUnified("expected", "actual", expected, edits)

	colorize := term.IsTerminal(int(os.Stderr.Fd()))
	var buf bytes.Buffer
	for _, line := range strings.Split(fmt.Sprintf("%v", unified), "\n") {
		switch {
		case colorize && strings.HasPrefix(line, "+"):
			fmt.Fprintln(&buf, addedStyle.Render(line))
		case colorize && strings.HasPrefix(line, "-"):
			fmt.Fprintln(&buf, removedStyle.Render(line))
		default:
			fmt.Fprintln(&buf, line)
		}
	}
	return buf.String()
}

// Generate a diff using diffmatchpatch.
func DiffStrings(x, y string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(x, y, false)
	dmp.DiffCleanupSemantic(diffs) // Clean up the diff for readability
	return dmp.DiffPrettyText(diffs)
}

// Colorize diff output based on the threshold.
func colorizeDiffWithThreshold(actual, expected string, threshold int) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(expected, actual, false)
	dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	for _, diff := range diffs {
		text := diff.Text
		switch diff.Type {
		case diffmatchpatch.DiffInsert, diffmatchpatch.DiffDelete:
			if len(text) < threshold {
				// For short diffs, highlight entire line
				sb.WriteString(fmt.Sprintf("\033[1m\033[33m%s\033[0m", text))
			} else {
				// For long diffs, highlight at word/character level
				color := "\033[32m" // Insert: green
				if diff.Type == diffmatchpatch.DiffDelete {
					color = "\033[31m" // Delete: red
				}
				sb.WriteString(fmt.Sprintf("%s%s\033[0m", color, text))
			}
		case diffmatchpatch.DiffEqual:
			sb.WriteString(text)
		}
	}

	return sb.String()
}

// getSnapshotFilenames returns the appropriate snapshot filenames based on whether TTY mode is enabled.
// When isTty is true, returns only the .tty.golden filename.
// When isTty is false, returns .stdout.golden and .stderr.golden filenames.
func getSnapshotFilenames(testName string, isTty bool) (stdout, stderr, tty string) {
	sanitized := sanitizeTestName(testName)
	if isTty {
		return "", "", filepath.Join(snapshotBaseDir, sanitized+".tty.golden")
	}
	return filepath.Join(snapshotBaseDir, sanitized+".stdout.golden"),
		filepath.Join(snapshotBaseDir, sanitized+".stderr.golden"),
		""
}

// verifyTTYSnapshot handles snapshot verification for TTY mode tests.
func verifyTTYSnapshot(t *testing.T, tc *TestCase, ttyPath, combinedOutput string, regenerate bool) bool {
	if regenerate {
		// Strip trailing whitespace from output before saving snapshot if requested.
		outputToSave := combinedOutput
		if tc.Expect.IgnoreTrailingWhitespace {
			outputToSave = stripTrailingWhitespace(combinedOutput)
		}

		t.Logf("Updating TTY snapshot at %q", ttyPath)
		updateSnapshot(ttyPath, outputToSave)
		return true
	}

	if _, err := os.Stat(ttyPath); errors.Is(err, os.ErrNotExist) {
		t.Fatalf(`TTY snapshot file not found: %q
Run the following command to create it:
$ go test ./tests -run %q -regenerate-snapshots`, ttyPath, t.Name())
	}

	filteredActual := applyIgnorePatterns(t, combinedOutput, tc.Expect.Diff)
	filteredExpected := applyIgnorePatterns(t, readSnapshot(t, ttyPath), tc.Expect.Diff)

	// Strip trailing whitespace if requested.
	if tc.Expect.IgnoreTrailingWhitespace {
		filteredActual = stripTrailingWhitespace(filteredActual)
		filteredExpected = stripTrailingWhitespace(filteredExpected)
	}

	if filteredExpected != filteredActual {
		var diff string
		if isCIEnvironment() || !term.IsTerminal(int(os.Stdout.Fd())) {
			diff = generateUnifiedDiff(filteredActual, filteredExpected)
		} else {
			diff = colorizeDiffWithThreshold(filteredActual, filteredExpected, 10)
		}
		t.Errorf("TTY output mismatch for %q:\n%s", ttyPath, diff)
	}

	return true
}

func verifySnapshot(t *testing.T, tc TestCase, stdoutOutput, stderrOutput string, regenerate bool) bool {
	if !tc.Snapshot {
		return true
	}

	// Sanitize outputs and fail the test if sanitization fails.
	var err error
	var sanitizeOpts []sanitizeOption
	if len(tc.Expect.Sanitize) > 0 {
		sanitizeOpts = append(sanitizeOpts, WithCustomReplacements(tc.Expect.Sanitize))
	}

	stdoutOutput, err = sanitizeOutput(stdoutOutput, sanitizeOpts...)
	if err != nil {
		t.Fatalf("failed to sanitize stdout output: %v", err)
	}
	stderrOutput, err = sanitizeOutput(stderrOutput, sanitizeOpts...)
	if err != nil {
		t.Fatalf("failed to sanitize stderr output: %v", err)
	}

	// Normalize line endings in actual output for cross-platform consistency.
	// This handles cases where CLI might output CRLF on Windows but snapshots use LF.
	stdoutOutput = normalizeLineEndings(stdoutOutput)
	stderrOutput = normalizeLineEndings(stderrOutput)

	stdoutPath, stderrPath, ttyPath := getSnapshotFilenames(t.Name(), tc.Tty)

	// TTY mode: combined output in single .tty.golden file
	if tc.Tty {
		// In TTY mode, stdout contains the combined output (from PTY)
		return verifyTTYSnapshot(t, &tc, ttyPath, stdoutOutput, regenerate)
	}

	// Non-TTY mode: separate stdout and stderr snapshots
	if regenerate {
		// Strip trailing whitespace from output before saving snapshot if requested.
		stdoutToSave := stdoutOutput
		stderrToSave := stderrOutput
		if tc.Expect.IgnoreTrailingWhitespace {
			stdoutToSave = stripTrailingWhitespace(stdoutOutput)
			stderrToSave = stripTrailingWhitespace(stderrOutput)
		}

		t.Logf("Updating stdout snapshot at %q", stdoutPath)
		updateSnapshot(stdoutPath, stdoutToSave)
		t.Logf("Updating stderr snapshot at %q", stderrPath)
		updateSnapshot(stderrPath, stderrToSave)
		return true
	}

	// Verify stdout
	if _, err := os.Stat(stdoutPath); errors.Is(err, os.ErrNotExist) {
		t.Fatalf(`Stdout snapshot file not found: %q
Run the following command to create it:
$ go test ./tests -run %q -regenerate-snapshots`, stdoutPath, t.Name())
	}

	filteredStdoutActual := applyIgnorePatterns(t, stdoutOutput, tc.Expect.Diff)
	filteredStdoutExpected := applyIgnorePatterns(t, readSnapshot(t, stdoutPath), tc.Expect.Diff)

	// Strip trailing whitespace if requested.
	if tc.Expect.IgnoreTrailingWhitespace {
		filteredStdoutActual = stripTrailingWhitespace(filteredStdoutActual)
		filteredStdoutExpected = stripTrailingWhitespace(filteredStdoutExpected)
	}

	if filteredStdoutExpected != filteredStdoutActual {
		var diff string
		if isCIEnvironment() || !term.IsTerminal(int(os.Stdout.Fd())) {
			// Generate a colorized diff for better readability
			diff = generateUnifiedDiff(filteredStdoutActual, filteredStdoutExpected)
		} else {
			diff = colorizeDiffWithThreshold(filteredStdoutActual, filteredStdoutExpected, 10)
		}

		t.Errorf("Stdout mismatch for %q:\n%s", stdoutPath, diff)
	}

	// Verify stderr
	if _, err := os.Stat(stderrPath); errors.Is(err, os.ErrNotExist) {
		t.Fatalf(`Stderr snapshot file not found: %q
Run the following command to create it:
$ go test -run=%q -regenerate-snapshots`, stderrPath, t.Name())
	}
	filteredStderrActual := applyIgnorePatterns(t, stderrOutput, tc.Expect.Diff)
	filteredStderrExpected := applyIgnorePatterns(t, readSnapshot(t, stderrPath), tc.Expect.Diff)

	// Strip trailing whitespace if requested.
	if tc.Expect.IgnoreTrailingWhitespace {
		filteredStderrActual = stripTrailingWhitespace(filteredStderrActual)
		filteredStderrExpected = stripTrailingWhitespace(filteredStderrExpected)
	}

	if filteredStderrExpected != filteredStderrActual {
		var diff string
		if isCIEnvironment() || !term.IsTerminal(int(os.Stdout.Fd())) {
			diff = generateUnifiedDiff(filteredStderrActual, filteredStderrExpected)
		} else {
			// Generate a colorized diff for better readability
			diff = colorizeDiffWithThreshold(filteredStderrActual, filteredStderrExpected, 10)
		}
		t.Errorf("Stderr diff mismatch for %q:\n%s", stderrPath, diff)
	}

	return true
}

// TestSnapshotRoundTrip tests that content written via updateSnapshot
// is normalized and can be read back via readSnapshot with same normalized result.
func TestSnapshotRoundTrip(t *testing.T) {
	tests := []struct {
		name               string
		content            string
		expectedNormalized string
	}{
		{
			name:               "normal_unix_line_endings",
			content:            "line1\nline2\nline3\n",
			expectedNormalized: "line1\nline2\nline3\n",
		},
		{
			name:               "windows_line_endings_normalized",
			content:            "line1\r\nline2\r\nline3\r\n",
			expectedNormalized: "line1\nline2\nline3\n",
		},
		{
			name:               "mixed_line_endings_normalized",
			content:            "line1\r\nline2\nline3\r\n",
			expectedNormalized: "line1\nline2\nline3\n",
		},
		{
			name:               "trailing_whitespace",
			content:            "line1  \nline2  \nline3  \n",
			expectedNormalized: "line1  \nline2  \nline3  \n",
		},
		{
			name:               "no_trailing_newline",
			content:            "line1\nline2\nline3",
			expectedNormalized: "line1\nline2\nline3",
		},
		{
			name:               "empty_content",
			content:            "",
			expectedNormalized: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			snapshotPath := filepath.Join(tmpDir, "test.golden")

			// Write the snapshot (should normalize)
			updateSnapshot(snapshotPath, tt.content)

			// Read it back using readSnapshot (also normalizes)
			got := readSnapshot(t, snapshotPath)

			if got != tt.expectedNormalized {
				t.Errorf("Round-trip mismatch:\nInput:    %q\nExpected: %q\nGot:      %q",
					tt.content, tt.expectedNormalized, got)
			}
		})
	}
}

// TestSanitizeOutputPreservesLineEndings verifies that sanitizeOutput
// doesn't alter line endings or whitespace.
func TestSanitizeOutputPreservesLineEndings(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "unix_line_endings",
			input:   "line1\nline2\nline3\n",
			wantErr: false,
		},
		{
			name:    "windows_line_endings",
			input:   "line1\r\nline2\r\nline3\r\n",
			wantErr: false,
		},
		{
			name:    "mixed_line_endings",
			input:   "line1\r\nline2\nline3\r\n",
			wantErr: false,
		},
		{
			name:    "trailing_spaces",
			input:   "line1  \nline2  \n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := sanitizeOutput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("sanitizeOutput() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check CRLF count preservation
			inputCRLF := strings.Count(tt.input, "\r\n")
			outputCRLF := strings.Count(output, "\r\n")
			if inputCRLF != outputCRLF {
				t.Errorf("CRLF count changed: input=%d, output=%d", inputCRLF, outputCRLF)
			}

			// Check CR count preservation (for mixed endings detection)
			inputCR := strings.Count(tt.input, "\r")
			outputCR := strings.Count(output, "\r")
			if inputCR != outputCR {
				t.Errorf("CR count changed: input=%d, output=%d", inputCR, outputCR)
			}

			// Check line count preservation
			inputLines := strings.Split(tt.input, "\n")
			outputLines := strings.Split(output, "\n")
			if len(inputLines) != len(outputLines) {
				t.Errorf("Line count changed: input=%d, output=%d", len(inputLines), len(outputLines))
			}
		})
	}
}

// TestSnapshotComparisonSymmetry verifies that the write path and read path
// use the same normalization, so comparisons are symmetric.
func TestSnapshotComparisonSymmetry(t *testing.T) {
	testContent := "line1\r\nline2\nline3\r\n"

	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "test.golden")

	// Simulate what verifySnapshot does when writing
	sanitized, err := sanitizeOutput(testContent)
	if err != nil {
		t.Fatalf("sanitizeOutput failed: %v", err)
	}
	normalized := normalizeLineEndings(sanitized)
	updateSnapshot(snapshotPath, normalized)

	// Simulate what verifySnapshot does when reading
	readContent := readSnapshot(t, snapshotPath)

	// Apply the same ignore patterns (empty in this test)
	actualFiltered := applyIgnorePatterns(t, normalized, nil)
	expectedFiltered := applyIgnorePatterns(t, readContent, nil)

	if actualFiltered != expectedFiltered {
		t.Errorf("Symmetry broken:\nActual (normalized before write): %q\nExpected (read from file): %q",
			actualFiltered, expectedFiltered)
	}
}

// TestNormalizeLineEndings tests the normalizeLineEndings function directly.
func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "LF unchanged",
			input:    "line1\nline2\nline3\n",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "CRLF converted to LF",
			input:    "line1\r\nline2\r\nline3\r\n",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "mixed CRLF and LF normalized to LF",
			input:    "line1\r\nline2\nline3\r\n",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "standalone CR preserved for spinners",
			input:    "Progress: 50%\rProgress: 100%\r",
			expected: "Progress: 50%\rProgress: 100%\r",
		},
		{
			name:     "spinner with newlines",
			input:    "Starting...\rProcessing...\rDone!\n",
			expected: "Starting...\rProcessing...\rDone!\n",
		},
		{
			name:     "CRLF and standalone CR mixed",
			input:    "line1\r\nProgress\rline2\r\n",
			expected: "line1\nProgress\rline2\n",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no line endings",
			input:    "single line no ending",
			expected: "single line no ending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLineEndings(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeLineEndings() mismatch:\nInput:    %q\nExpected: %q\nGot:      %q",
					tt.input, tt.expected, got)
			}
		})
	}
}

// TestSnapshotRoundTripWithNormalization verifies that CRLF content is normalized
// when written and read back.
func TestSnapshotRoundTripWithNormalization(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOnDisk string
	}{
		{
			name:           "CRLF normalized to LF on write",
			input:          "line1\r\nline2\r\nline3\r\n",
			expectedOnDisk: "line1\nline2\nline3\n",
		},
		{
			name:           "standalone CR preserved",
			input:          "Progress\rDone\r",
			expectedOnDisk: "Progress\rDone\r",
		},
		{
			name:           "mixed CRLF and CR",
			input:          "line1\r\nProgress\rline2\r\n",
			expectedOnDisk: "line1\nProgress\rline2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			snapshotPath := filepath.Join(tmpDir, "test.golden")

			// Write via updateSnapshot (should normalize)
			updateSnapshot(snapshotPath, tt.input)

			// Read raw file content
			rawContent, err := os.ReadFile(snapshotPath)
			if err != nil {
				t.Fatalf("Failed to read snapshot: %v", err)
			}

			got := string(rawContent)
			if got != tt.expectedOnDisk {
				t.Errorf("On-disk content mismatch:\nExpected: %q\nGot:      %q",
					tt.expectedOnDisk, got)
				t.Logf("Expected CRLF count: %d", strings.Count(tt.expectedOnDisk, "\r\n"))
				t.Logf("Got CRLF count: %d", strings.Count(got, "\r\n"))
				t.Logf("Expected CR count: %d", strings.Count(tt.expectedOnDisk, "\r"))
				t.Logf("Got CR count: %d", strings.Count(got, "\r"))
			}
		})
	}
}

// TestSnapshotComparisonCrossPlatform verifies that CRLF output from CLI
// matches LF snapshots after normalization.
func TestSnapshotComparisonCrossPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "test.golden")

	// Create snapshot with LF (as it would be on disk)
	snapshotContent := "line1\nline2\nline3\n"
	err := os.WriteFile(snapshotPath, []byte(snapshotContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write snapshot: %v", err)
	}

	// Simulate CLI output with CRLF (as it might be on Windows)
	cliOutput := "line1\r\nline2\r\nline3\r\n"

	// Normalize both (as verifySnapshot does)
	normalizedCLI := normalizeLineEndings(cliOutput)
	normalizedSnapshot := normalizeLineEndings(snapshotContent)

	if normalizedCLI != normalizedSnapshot {
		t.Errorf("Cross-platform comparison failed:\nCLI output (CRLF):     %q\nSnapshot (LF):         %q\nNormalized CLI:        %q\nNormalized snapshot:   %q",
			cliOutput, snapshotContent, normalizedCLI, normalizedSnapshot)
	}
}

// TestSpinnerOutputPreserved verifies that standalone CR characters used by
// spinners and progress indicators are not stripped by normalization.
func TestSpinnerOutputPreserved(t *testing.T) {
	spinnerOutput := "Processing item 1/10\rProcessing item 2/10\rProcessing item 3/10\rDone!\n"

	normalized := normalizeLineEndings(spinnerOutput)

	// Count CR characters
	crCount := strings.Count(normalized, "\r")
	expectedCRCount := 3 // Three CRs in the spinner sequence

	if crCount != expectedCRCount {
		t.Errorf("Spinner CRs were modified:\nOriginal CR count: %d\nNormalized CR count: %d\nOriginal:   %q\nNormalized: %q",
			expectedCRCount, crCount, spinnerOutput, normalized)
	}

	// Verify exact equality for spinner output
	if normalized != spinnerOutput {
		t.Errorf("Spinner output was modified:\nOriginal:   %q\nNormalized: %q",
			spinnerOutput, normalized)
	}
}
