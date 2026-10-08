package initcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPreflightBeforeConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("pacts", 0o755))
	require.NoError(t, os.Mkdir("target", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("target", "file"), []byte("keep"), 0o600))
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("invalid: ["), 0o600))
	for _, args := range [][]string{
		{"pacts", "target"},
		{"--no-git", "pacts", "target"},
		{"github.com/cloudposse/atmos//examples/demo", "target"},
		{"--copy", "https://example.com/source.zip", "target"},
	} {
		require.ErrorIs(t, Preflight(args), errUtils.ErrTargetDirectoryNotEmpty, "%v", args)
	}
	for _, args := range [][]string{
		{"pacts", "target", "--force"},
		{"pacts", "target", "-f"},
		{"pacts", "target", "--update"},
		{"pacts", "new-target"},
		{"pacts", "target", "--use-version", "1.0.0"},
	} {
		require.NoError(t, Preflight(args), "%v", args)
	}
}

func TestPreflightPreservesManifestAndEnvironmentOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("pacts", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("pacts", "scaffold.yaml"), []byte("invalid: ["), 0o600))
	require.NoError(t, os.Mkdir("target", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("target", "file"), []byte("keep"), 0o600))
	require.NoError(t, Preflight([]string{"pacts", "target"}), "leave scaffold validation and update prompting to init")
	t.Setenv("ATMOS_INIT_COPY", "true")
	require.ErrorIs(t, Preflight([]string{"pacts", "target"}), errUtils.ErrTargetDirectoryNotEmpty)
	t.Setenv("ATMOS_INIT_FORCE", "true")
	require.NoError(t, Preflight([]string{"pacts", "target"}))
	require.ErrorIs(t, Preflight([]string{"pacts", "target", "--force=false"}), errUtils.ErrTargetDirectoryNotEmpty)
}
