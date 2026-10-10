package github

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureStderr redirects os.Stderr, which the global I/O context writes to dynamically, into a
// temporary file. The returned function reads everything written so far.
func captureStderr(t *testing.T) func() string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)

	original := os.Stderr
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = original
		_ = f.Close()
	})

	return func() string {
		t.Helper()
		data, err := os.ReadFile(f.Name())
		require.NoError(t, err)
		return string(data)
	}
}

// brokenStderr redirects os.Stderr to a closed file so every write to it fails.
func brokenStderr(t *testing.T) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	original := os.Stderr
	os.Stderr = f
	t.Cleanup(func() { os.Stderr = original })
}
