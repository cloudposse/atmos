package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// longHclValue makes the first write longer than the second, so a missing truncation leaves the
// tail of the first write behind.
var longHclValue = strings.Repeat("x", 512)

func TestWriteToFileAsHclReplacesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vpc.tfvars")
	require.NoError(t, WriteToFileAsHcl(path, map[string]any{"name": longHclValue, "extra": "value"}, 0o644))
	require.NoError(t, WriteToFileAsHcl(path, map[string]any{"name": "short"}, 0o644))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "name = \"short\"", strings.TrimSpace(string(content)))
	assert.NotContains(t, string(content), "xxx", "the previous, longer content must not remain")
}

func TestWriteToFileAsHclCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.tfvars")
	require.NoError(t, WriteToFileAsHcl(path, map[string]any{"name": "short"}, 0o644))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "name = \"short\"", strings.TrimSpace(string(content)))
}

func TestWriteTerraformBackendConfigToFileAsHclReplacesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.tf")
	require.NoError(t, WriteTerraformBackendConfigToFileAsHcl(path, "s3", map[string]any{"bucket": longHclValue, "key": "state", "region": "us-east-1"}))
	require.NoError(t, WriteTerraformBackendConfigToFileAsHcl(path, "local", map[string]any{"path": "s"}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "terraform {\n  backend \"local\" {\n    path = \"s\"\n  }\n}\n", string(content))
}

func TestWriteHclRejectsUnwritablePath(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing", "file.tf")
	assert.Error(t, WriteToFileAsHcl(missingDir, map[string]any{"a": "b"}, 0o644))
	assert.Error(t, WriteTerraformBackendConfigToFileAsHcl(missingDir, "local", map[string]any{"path": "s"}))
}
