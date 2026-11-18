package keystore

import (
	"os"
	"path/filepath"
	"testing"
)

func readFile(t *testing.T, dir, name string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(filepath.Join(dir, name))
}

func writeFile(t *testing.T, dir, name string, data []byte) error {
	t.Helper()
	return os.WriteFile(filepath.Join(dir, name), data, 0o600)
}
