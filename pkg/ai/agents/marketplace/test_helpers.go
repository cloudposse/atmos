package marketplace

import (
	"os"
	"path/filepath"
)

// WriteTestFile is a helper function to create test files.
func WriteTestFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
