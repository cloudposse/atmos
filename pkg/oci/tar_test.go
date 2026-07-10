package oci

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestTar(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return &buf
}

func TestExtractTarball_WritesFiles(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"main.tf":          "resource \"null_resource\" \"x\" {}\n",
		"nested/output.tf": "output \"x\" {}\n",
	})

	require.NoError(t, extractTarball(tarBuf, dest))

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "null_resource")

	nested, err := os.ReadFile(filepath.Join(dest, "nested", "output.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(nested), "output")
}

func TestExtractTarball_SkipsDirectoryTraversal(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"../../etc/passwd": "malicious\n",
	})

	require.NoError(t, extractTarball(tarBuf, dest))

	_, err := os.Stat(filepath.Join(dest, "..", "..", "etc", "passwd"))
	assert.True(t, os.IsNotExist(err), "traversal entry must not be written outside dest")
}
