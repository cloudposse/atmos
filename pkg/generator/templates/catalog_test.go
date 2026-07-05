package templates

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCatalog(t *testing.T) {
	entries, err := LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, entries, "embedded catalog must advertise at least one template")

	byName := make(map[string]CatalogEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	for _, tc := range []struct {
		name  string
		cloud string
		tier  string
	}{
		{"aws/app", "aws", "app"},
		{"aws/landing-zone", "aws", "landing-zone"},
		{"gcp/landing-zone", "gcp", "landing-zone"},
		{"azure/landing-zone", "azure", "landing-zone"},
	} {
		entry, ok := byName[tc.name]
		require.Truef(t, ok, "catalog must include %s", tc.name)
		assert.Equal(t, tc.cloud, entry.Cloud)
		assert.Equal(t, tc.tier, entry.Tier)
		assert.NotEmpty(t, entry.Source)
		assert.NotEmpty(t, entry.Version)
		assert.NotEmpty(t, entry.Description)
	}
}

func TestCatalogEntry_ResolvedSource(t *testing.T) {
	e := CatalogEntry{
		Cloud:  "aws",
		Tier:   "landing-zone",
		Source: "github.com/cloudposse/atmos.git//examples/scaffolds/aws/landing-zone?ref=main",
	}

	// No override: the remote source is returned verbatim.
	assert.Equal(t, e.Source, e.ResolvedSource(""))

	// Override: a local path under <override>/<cloud>/<tier>.
	got := e.ResolvedSource(filepath.Join("repo", "examples", "scaffolds"))
	assert.Equal(t, filepath.Join("repo", "examples", "scaffolds", "aws", "landing-zone"), got)
}

func TestCatalogStubs(t *testing.T) {
	stubs, err := CatalogStubs("")
	require.NoError(t, err)

	for _, name := range []string{"aws/app", "aws/landing-zone", "gcp/landing-zone", "azure/landing-zone"} {
		stub, ok := stubs[name]
		require.True(t, ok)
		assert.Equal(t, name, stub.Name)
		assert.NotEmpty(t, stub.Description)
		assert.NotEmpty(t, stub.Version)
		assert.NotEmpty(t, stub.Source)
		assert.Empty(t, stub.Files, "catalog stubs defer file loading until hydration")
	}

	// With an override the stub source becomes the local path.
	base := filepath.Join("repo", "examples", "scaffolds")
	overridden, err := CatalogStubs(base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "aws", "app"), overridden["aws/app"].Source)
	assert.Equal(t, filepath.Join(base, "aws", "landing-zone"), overridden["aws/landing-zone"].Source)
	assert.Equal(t, filepath.Join(base, "gcp", "landing-zone"), overridden["gcp/landing-zone"].Source)
	assert.Equal(t, filepath.Join(base, "azure", "landing-zone"), overridden["azure/landing-zone"].Source)
}
