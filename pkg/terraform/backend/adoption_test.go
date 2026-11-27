package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalWorkspaceDirectory(t *testing.T) {
	t.Setenv("TF_DATA_DIR", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "main.tf")
	write := func(path, body string) { t.Helper(); require.NoError(t, os.WriteFile(path, []byte(body), 0o600)) }
	for _, native := range []string{"terraform {", "terraform {\n cloud {}\n}", "terraform {\n backend \"s3\" {}\n}", "terraform {\n backend \"local\" { workspace_dir = var.path }\n}"} {
		write(file, native)
		_, err := LocalWorkspaceDirectory(dir, false)
		require.Error(t, err)
	}
	write(file, "terraform {\n backend \"local\" { workspace_dir = \"workspaces\" }\n}")
	actual, err := LocalWorkspaceDirectory(dir, false)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "workspaces"), actual)
	write(filepath.Join(dir, "main.tofu"), "terraform {\n backend \"s3\" {}\n}")
	_, err = LocalWorkspaceDirectory(dir, true)
	require.ErrorContains(t, err, "local backend")
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".terraform"), 0o700))
	cache := filepath.Join(dir, ".terraform", "terraform.tfstate")
	for _, data := range []string{"[", `{"backend":{"type":"s3"}}`, `{"backend":{"type":"local","config":{"path":"custom"}}}`, `{"backend":{"config":{"path":[]}}}`} {
		write(cache, data)
		_, err = LocalWorkspaceDirectory(dir, false)
		require.Error(t, err)
	}
	write(cache, `{"backend":{"type":"local","config":{"path":null,"workspace_dir":"workspaces"}}}`)
	write(filepath.Join(dir, "outputs.tf.json"), "{}")
	_, err = LocalWorkspaceDirectory(dir, false)
	require.NoError(t, err)
	_, err = LocalWorkspaceDirectory(file, false)
	require.Error(t, err)
}
