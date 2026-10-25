package initcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolveRecordedInitSource(t *testing.T) {
	t.Run("explicit source unchanged", func(t *testing.T) {
		opts := &initOptions{update: true, templateName: "basic"}
		require.NoError(t, resolveRecordedInitSource(opts))
		assert.Empty(t, opts.targetDir)
	})
	t.Run("new init unchanged", func(t *testing.T) {
		opts := &initOptions{}
		require.NoError(t, resolveRecordedInitSource(opts))
		assert.Empty(t, opts.templateName)
	})
	t.Run("missing record", func(t *testing.T) {
		err := resolveRecordedInitSource(&initOptions{update: true, targetDir: t.TempDir()})
		require.ErrorIs(t, err, errUtils.ErrInitialization)
	})
	t.Run("recorded source preserved", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".atmos"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".atmos", "scaffold.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: custom
spec:
  source: git::https://example.invalid/template.git?ref=main
  values:
    project_name: saved
`), 0o600))
		opts := &initOptions{update: true, targetDir: dir, templateVars: map[string]any{"project_name": "override"}}
		require.NoError(t, resolveRecordedInitSource(opts))
		assert.Equal(t, "git::https://example.invalid/template.git?ref=main", opts.templateName)
		assert.Equal(t, "override", opts.templateVars["project_name"])
	})
}
