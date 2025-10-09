package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	actualFiltered := applyIgnorePatterns(normalized, nil)
	expectedFiltered := applyIgnorePatterns(readContent, nil)

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
