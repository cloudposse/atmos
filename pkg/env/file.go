package env

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultFileMode is the file mode for output files.
	defaultFileMode = 0o644
)

// WriteToFile writes content to a file in append mode.
// Creates the file if it doesn't exist.
func WriteToFile(path string, content string) error {
	defer perf.Track(nil, "env.WriteToFile")()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFileMode)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to write to file '%s': %w", path, err)
	}

	return nil
}
