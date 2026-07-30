//go:build !windows

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDescribeToolchainPathEntry_UnreadableDir covers the os.ReadDir failure
// branch: a toolchain-like PATH entry that exists but can't be listed (e.g.
// permissions clamped mid-write by a concurrent installer) must still produce
// a forensics line, just with "dir unreadable" instead of a contents list.
// Windows' ACL model doesn't support chmod 0o000 the same way as POSIX, so
// this is unix-only, matching the existing pattern in
// pkg/config/adapters/adapters_error_paths_unix_test.go.
func TestDescribeToolchainPathEntry_UnreadableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	parent := t.TempDir()
	locked := filepath.Join(parent, "locked-toolchain-dir")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	var b strings.Builder
	describeToolchainPathEntry(&b, locked, []string{"tofu"})
	got := b.String()

	assert.Contains(t, got, "target present=false")
	assert.Contains(t, got, "dir unreadable")
}
