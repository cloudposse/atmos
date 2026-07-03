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

	lz, ok := byName["aws/landing-zone"]
	require.True(t, ok, "catalog must include aws/landing-zone")
	assert.Equal(t, "aws", lz.Cloud)
	assert.Equal(t, "landing-zone", lz.Tier)
	assert.NotEmpty(t, lz.Source)
	assert.NotEmpty(t, lz.Version)
	assert.NotEmpty(t, lz.Description)
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

	stub, ok := stubs["aws/landing-zone"]
	require.True(t, ok)
	assert.Equal(t, "aws/landing-zone", stub.Name)
	assert.NotEmpty(t, stub.Description)
	assert.NotEmpty(t, stub.Version)
	assert.NotEmpty(t, stub.Source)
	assert.Empty(t, stub.Files, "catalog stubs defer file loading until hydration")

	// With an override the stub source becomes the local path.
	base := filepath.Join("repo", "examples", "scaffolds")
	overridden, err := CatalogStubs(base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "aws", "landing-zone"), overridden["aws/landing-zone"].Source)
}
