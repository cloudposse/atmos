package source

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
)

func TestProvenance(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, HasProvenance(dir))
		assert.False(t, HasProvenance(filepath.Join(dir, "missing")))
	})

	t.Run("marker round trip", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, WriteProvenance(dir, &Provenance{Component: "vpc", Source: "https://example.invalid/x", ProvisionedAt: time.Now()}))
		assert.True(t, HasProvenance(dir))
		assert.FileExists(t, filepath.Join(dir, workdir.AtmosDir, ProvenanceFile))
	})

	t.Run("a directory named like the marker is not provenance", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, workdir.AtmosDir, ProvenanceFile), 0o755))
		assert.False(t, HasProvenance(dir))
	})

	t.Run("workdir metadata", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, workdir.WriteMetadata(dir, &workdir.WorkdirMetadata{Component: "vpc", UpdatedAt: time.Now()}))
		assert.True(t, HasProvenance(dir))
	})
}

func TestCheckDeletable(t *testing.T) {
	for _, fx := range componentTypeFixtures() {
		type testCase struct {
			name        string
			provisioned bool
			workdirMeta bool
			siblings    func(dir string) map[string]any
			wantRefused bool
		}
		sourced := map[string]any{"source": map[string]any{"uri": "https://example.invalid/x"}, "component": "vpc"}
		cases := []testCase{
			{name: "provisioned and unowned", provisioned: true},
			{name: "provisioned workdir", workdirMeta: true},
			{name: "no provenance", wantRefused: true},
			{
				name: "owned by sourceless component", provisioned: true, wantRefused: true,
				siblings: func(string) map[string]any { return map[string]any{"x-handmade": map[string]any{"component": "vpc"}} },
			},
			{
				name: "owned by sourceless component via metadata.component", provisioned: true, wantRefused: true,
				siblings: func(string) map[string]any {
					return map[string]any{"x-handmade": map[string]any{"metadata": map[string]any{"component": "vpc"}}}
				},
			},
			{
				name: "owned by sourceless component named like the directory", provisioned: true, wantRefused: true,
				siblings: func(string) map[string]any { return map[string]any{"vpc": map[string]any{}} },
			},
			{
				name: "owned by sourceless component via working_directory", provisioned: true, wantRefused: true,
				siblings: func(dir string) map[string]any {
					return map[string]any{"other": map[string]any{"metadata": map[string]any{"working_directory": dir}}}
				},
			},
			{
				name: "sibling with source sharing the directory is not an owner", provisioned: true,
				siblings: func(string) map[string]any { return map[string]any{"vpc-b": sourced} },
			},
			{
				name: "sourceless sibling in another directory", provisioned: true,
				siblings: func(string) map[string]any { return map[string]any{"other": map[string]any{"component": "other"}} },
			},
			{
				name: "malformed sibling is ignored", provisioned: true,
				siblings: func(string) map[string]any { return map[string]any{"broken": "not-a-map"} },
			},
		}
		for _, tt := range cases {
			t.Run(fx.componentType+"/"+tt.name, func(t *testing.T) {
				root := t.TempDir()
				target := fx.componentDir(root, "vpc")
				require.NoError(t, os.MkdirAll(target, 0o755))
				if tt.provisioned {
					require.NoError(t, WriteProvenance(target, &Provenance{Component: "vpc", Source: "https://example.invalid/x"}))
				}
				if tt.workdirMeta {
					require.NoError(t, workdir.WriteMetadata(target, &workdir.WorkdirMetadata{Component: "vpc", UpdatedAt: time.Now()}))
				}
				var siblings map[string]any
				if tt.siblings != nil {
					siblings = tt.siblings(target)
				}

				err := CheckDeletable(fx.config(root), fx.componentType, target, siblings)
				if tt.wantRefused {
					require.ErrorIs(t, err, errUtils.ErrSourceDeleteRefused)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}
