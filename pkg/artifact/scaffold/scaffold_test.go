package scaffold_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/artifact/scaffold"
)

const validManifest = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: landing-zone
  description: A landing zone
  version: 2.0.0
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

func TestValidator_ReadsManifestMetadata(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "scaffold.yaml", validManifest, 0o644)

	meta, err := scaffold.Validator{}.ValidateScaffold(context.Background(), dir)

	require.NoError(t, err)
	assert.Equal(t, produce.ProducerMetadata{Name: "landing-zone", Version: "2.0.0", Description: "A landing zone"}, meta)
}

func TestValidator_RejectsInvalidScaffolds(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		write    bool
	}{
		{name: "missing manifest", write: false},
		{name: "wrong kind", write: true, manifest: "apiVersion: atmos/v1\nkind: Other\nmetadata:\n  name: x\n"},
		{name: "missing name", write: true, manifest: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata: {}\n"},
		{name: "unknown field type", write: true, manifest: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: x\nspec:\n  fields:\n    - name: a\n      type: nonsense\n"},
		{name: "not yaml", write: true, manifest: "{{{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.write {
				fixtures.WriteFile(t, dir, "scaffold.yaml", tt.manifest, 0o644)
			}

			_, err := scaffold.Validator{}.ValidateScaffold(context.Background(), dir)

			require.ErrorIs(t, err, errUtils.ErrArtifactScaffoldInvalid)
		})
	}
}

func TestValidator_AcceptsOnlyTheRootManifest(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "nested/scaffold.yaml", validManifest, 0o644)
	fixtures.WriteFile(t, dir, "scaffold.yml", validManifest, 0o644)

	_, err := scaffold.Validator{}.ValidateScaffold(context.Background(), dir)

	require.ErrorIs(t, err, errUtils.ErrArtifactScaffoldInvalid, "generation reads only scaffold.yaml at the root")
}

func TestValidator_DoesNotGenerateOrRunHooks(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "hook-ran")
	manifest := validManifest + `  hooks:
    marker:
      events: [before.scaffold.generate, after.scaffold.generate]
      kind: step
      type: shell
      with:
        command: "touch ` + filepath.ToSlash(marker) + `"
`
	fixtures.WriteFile(t, dir, "scaffold.yaml", manifest, 0o644)
	fixtures.WriteFile(t, dir, "README.md", "# {{ .project_name }}\n", 0o644)

	_, err := scaffold.Validator{}.ValidateScaffold(context.Background(), dir)

	require.NoError(t, err)
	_, statErr := os.Stat(marker)
	require.ErrorIs(t, statErr, os.ErrNotExist, "validation must not execute scaffold hooks")
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Len(t, entries, 2, "validation writes no generated output into the source")
}

func TestValidator_HonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "scaffold.yaml", validManifest, 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := scaffold.Validator{}.ValidateScaffold(ctx, dir)

	require.ErrorIs(t, err, context.Canceled)
}

func TestRegister_InstallsTheProcessWideValidator(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "scaffold.yaml", validManifest, 0o644)
	produce.RegisterScaffoldValidator(nil)
	t.Cleanup(func() { produce.RegisterScaffoldValidator(nil) })

	scaffold.Register()

	prepared, err := produce.NewScaffold(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source:      dir,
		Packaging:   artifact.PackagingTgz,
		OperationID: "op-1",
		PreparedAt:  time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
	})

	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	assert.Equal(t, "landing-zone", prepared.Record.Name)
	assert.Equal(t, "2.0.0", prepared.Record.Version)
	assert.Equal(t, produce.KindScaffold, prepared.Record.Kind)
}
