package oci

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestZip(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return &buf
}

func TestExtractZip_WritesFiles(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"main.tf":          "resource \"null_resource\" \"x\" {}\n",
		"nested/output.tf": "output \"x\" {}\n",
	})

	require.NoError(t, extractZip(zipBuf, dest))

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "null_resource")

	nested, err := os.ReadFile(filepath.Join(dest, "nested", "output.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(nested), "output")
}

func TestExtractZip_SkipsDirectoryTraversal(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"../../etc/passwd": "malicious\n",
	})

	require.NoError(t, extractZip(zipBuf, dest))

	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Empty(t, entries, "traversal entry must not be written to dest")
}
