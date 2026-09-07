//go:build darwin

package installer

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestStripQuarantineAttributesNoAttributesSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain-file")
	require.NoError(t, os.WriteFile(path, []byte("not a binary"), 0o600))

	require.NoError(t, stripQuarantineAttributes(path))
}

func TestStripQuarantineAttributesRemovesSetAttribute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quarantined-file")
	require.NoError(t, os.WriteFile(path, []byte("not a binary"), 0o600))

	require.NoError(t, unix.Setxattr(path, "com.apple.quarantine", []byte("0081;00000000;Atmos;"), 0))

	require.NoError(t, stripQuarantineAttributes(path))

	_, err := unix.Getxattr(path, "com.apple.quarantine", nil)
	require.Error(t, err, "com.apple.quarantine should have been removed")
}

func TestAdHocResignBinary(t *testing.T) {
	dstPath := copyOfTestBinary(t)

	require.NoError(t, adHocResignBinary(dstPath))
}

func TestAdHocResignBinaryNonexistentPath(t *testing.T) {
	err := adHocResignBinary(filepath.Join(t.TempDir(), "does-not-exist"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerifierTrustFailed)
}

func TestTrustVerifierBinaryEndToEnd(t *testing.T) {
	dstPath := copyOfTestBinary(t)

	require.NoError(t, trustVerifierBinary(dstPath))
}

// copyOfTestBinary copies the running test binary (a real Mach-O executable)
// to a temp path so darwin-specific trust logic can be exercised against a
// binary that codesign will actually accept.
func copyOfTestBinary(t *testing.T) string {
	t.Helper()

	src, err := os.Open(os.Args[0])
	require.NoError(t, err)
	defer src.Close()

	dstPath := filepath.Join(t.TempDir(), "verifier-binary")
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	require.NoError(t, err)
	_, err = io.Copy(dst, src)
	require.NoError(t, dst.Close())
	require.NoError(t, err)

	return dstPath
}
