package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const listYAML = `apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: a
    namespace: demo
- apiVersion: v1
  kind: Secret
  metadata:
    name: b
    namespace: demo
`

func TestDecodeObjects_ExpandsList(t *testing.T) {
	objects, err := DecodeObjects([]byte(listYAML))
	require.NoError(t, err)
	require.Len(t, objects, 2)
	assert.Equal(t, "ConfigMap", objects[0].GetKind())
	assert.Equal(t, "a", objects[0].GetName())
	assert.Equal(t, "Secret", objects[1].GetKind())
	assert.Equal(t, "b", objects[1].GetName())
}

func TestDecodeObjects_DecodeError(t *testing.T) {
	// Unterminated YAML flow sequence is a hard decode error (not EOF).
	_, err := DecodeObjects([]byte("apiVersion: v1\nkind: [unterminated\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode Kubernetes manifest")
}

func TestWriteObjects_OutputDirSingleManifest(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, WriteObjects(objects, RenderOptions{OutputDir: dir, Noun: "Helm"}))

	// The non-split OutputDir mode writes a single manifest.yaml.
	data, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	require.NoError(t, err)
	decoded, err := DecodeObjects(data)
	require.NoError(t, err)
	require.Len(t, decoded, 2)
}

func TestWriteObjects_DefaultStdout(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)

	// With no Output/OutputDir, the multi-document YAML goes to stdout.
	require.NoError(t, WriteObjects(objects, RenderOptions{Noun: "Helm"}))
}
