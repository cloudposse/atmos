package vhs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// PreprocessTape creates a preprocessed tape file with Source directives inlined.
// This allows VHS to run from any directory since the tape becomes self-contained.
// Returns the absolute path to a temporary file (caller must delete with os.Remove).
func PreprocessTape(tapeFile string) (string, error) {
	// Create temp file in system temp dir (works cross-platform).
	tempFile, err := os.CreateTemp("", "preprocessed-*.tape")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// Process the tape file, inlining Source directives.
	tapeDir := filepath.Dir(tapeFile)
	if err := inlineSources(tapeFile, tapeDir, tempFile); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

// inlineSources reads a tape file and writes its content to out,
// recursively inlining any Source directives.
func inlineSources(tapeFile, baseDir string, out *os.File) error {
	file, err := os.Open(tapeFile)
	if err != nil {
		return fmt.Errorf("failed to open tape file: %w", err)
	}
	defer file.Close()

	// Regex to match Source directives: "Source path/to/file.tape".
	sourceRegex := regexp.MustCompile(`^Source\s+"?([^"]+)"?\s*$`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is a Source directive.
		if matches := sourceRegex.FindStringSubmatch(line); matches != nil {
			sourcePath := matches[1]

			// Resolve relative to tape file directory.
			if !filepath.IsAbs(sourcePath) {
				sourcePath = filepath.Join(baseDir, sourcePath)
			}

			// Check if source file exists.
			if _, err := os.Stat(sourcePath); err == nil {
				// Write comment indicating where the content came from.
				if _, err := fmt.Fprintf(out, "# Inlined from: %s\n", filepath.Base(sourcePath)); err != nil {
					return err
				}

				// Recursively process the source file (it might have nested Sources).
				if err := inlineSources(sourcePath, filepath.Dir(sourcePath), out); err != nil {
					return fmt.Errorf("failed to inline source %s: %w", sourcePath, err)
				}
				continue // Don't write the original Source line.
			}
			// If file doesn't exist, keep the original line (VHS will error on it).
		}

		// Write line to output.
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	return scanner.Err()
}
