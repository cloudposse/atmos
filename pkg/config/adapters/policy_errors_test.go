package adapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// An optional import may ignore a missing source, but never an empty configured policy bundle.
func TestOptionalLocalImportRejectsEmptyPolicyDirectory(t *testing.T) {
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})
	t.Cleanup(config.ResetImportAdapterRegistry)
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "imported.yaml"), []byte("settings: {}\n"), 0o600))

	paths, err := config.ProcessImportsFromAdapter(nil, dir, []schema.ConfigImport{
		{Path: "imported.yaml", SkipIfMissing: true},
	}, dir, 1, 10)

	require.ErrorIs(t, err, errUtils.ErrNoPoliciesConfigured)
	require.Empty(t, paths)
}
