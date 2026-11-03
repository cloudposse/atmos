package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMaterializationTracksSourceQuery(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	target := filepath.Join(config.BasePath, "vendor")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("v1"), 0o644))
	files, err := Inventory(target)
	require.NoError(t, err)
	id := mustArtifactID(t, config, "remote", target)
	declared := "git::https://user:private-password@example.invalid/repo?ref=v1&token=private-token"
	require.NoError(t, Replace(config, id, Artifact{Kind: "remote", Target: target, Source: Source{Declared: declared}, Files: files}))

	for _, tc := range []struct {
		name, source string
		materialized bool
	}{
		{"same ref", declared, true},
		{"changed ref", "git::https://example.invalid/repo?ref=v2&token=private-token", false},
		{"removed query", "git::https://example.invalid/repo", false},
		{"changed options", "git::https://example.invalid/repo?ref=v1&token=private-token&depth=0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, checkErr := IsMaterialized(config, MaterializationParams{ID: id, Target: target, Declared: tc.source})
			require.NoError(t, checkErr)
			require.Equal(t, tc.materialized, check.Materialized)
		})
	}
	lock, err := Load(config)
	require.NoError(t, err)
	// Saving an already-redacted receipt must retain its query identity.
	require.NoError(t, Save(config, lock))
	check, err := IsMaterialized(config, MaterializationParams{ID: id, Target: target, Declared: declared})
	require.NoError(t, err)
	require.True(t, check.Materialized)
	content, err := os.ReadFile(Path(config))
	require.NoError(t, err)
	require.NotContains(t, string(content), "private-password")
	require.NotContains(t, string(content), "private-token")
	require.NotContains(t, string(content), "ref=v1")

	// Old receipts cannot prove which ref was fetched. They remain readable,
	// but a queried source must be fetched once to obtain a complete receipt.
	legacy := lock.Artifacts[id]
	legacy.Source = Source{Declared: "git::https://example.invalid/repo"}
	require.NoError(t, Replace(config, id, legacy))
	check, err = IsMaterialized(config, MaterializationParams{ID: id, Target: target, Declared: declared})
	require.NoError(t, err)
	require.False(t, check.Materialized)
}
