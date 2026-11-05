package starlark

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestDigestModuleIsPredeclared(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `output = [digest.sha256("abc"), digest.sha512("abc")[:16], digest.sha1("abc"), digest.md5("abc")]`)
	require.NoError(t, err)
	assert.JSONEq(t, `["ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad","ddaf35a193617aba","a9993e364706816aba3e25717850c26c9cd0d89d","900150983cd24fb0d6963f7d28e17f72"]`, result.Value)
}

func TestDigestOfBinaryFileContents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Bytes that are not valid UTF-8, including NUL, must hash as the raw file contents.
	blob := []byte{0x00, 0xff, 0xfe, 0x80, 'a', 0x00, 0xc3}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blob.bin"), blob, 0o600))
	sum := sha256.Sum256(blob)

	result, err := New().Execute(t.Context(), script.Spec{Name: "test.star", WorkingDirectory: dir, Source: `output = digest.sha256(fs.read_file("blob.bin"))`})
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(sum[:]), result.Value)
}

// The digest module must not shadow Starlark's own hash() builtin.
func TestDigestModuleKeepsTheUniverseHashBuiltin(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `output = [hash("abc"), type(digest)]`)
	require.NoError(t, err)
	assert.JSONEq(t, `[96354, "module"]`, result.Value)
}
