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

	sandbox, ok := byName["aws/sandbox"]
	require.True(t, ok, "catalog must include aws/sandbox")
	assert.Equal(t, "aws", sandbox.Cloud)
	assert.Equal(t, "sandbox", sandbox.Tier)
	assert.NotEmpty(t, sandbox.Source)
	assert.NotEmpty(t, sandbox.Version)
	assert.NotEmpty(t, sandbox.Description)

	lz, ok := byName["aws/landing-zone"]
	require.True(t, ok, "catalog must include aws/landing-zone")
	assert.Equal(t, "landing-zone", lz.Tier)
}

func TestCatalogEntry_ResolvedSource(t *testing.T) {
	e := CatalogEntry{
		Cloud:  "aws",
		Tier:   "sandbox",
		Source: "github.com/cloudposse/atmos.git//examples/scaffolds/aws/sandbox?ref=main",
	}

	// No override: the remote source is returned verbatim.
	assert.Equal(t, e.Source, e.ResolvedSource(""))

	// Override: a local path under <override>/<cloud>/<tier>.
	got := e.ResolvedSource(filepath.Join("repo", "examples", "scaffolds"))
	assert.Equal(t, filepath.Join("repo", "examples", "scaffolds", "aws", "sandbox"), got)
}

func TestCatalogStubs(t *testing.T) {
	stubs, err := CatalogStubs("")
	require.NoError(t, err)

	stub, ok := stubs["aws/sandbox"]
	require.True(t, ok)
	assert.Equal(t, "aws/sandbox", stub.Name)
	assert.NotEmpty(t, stub.Description)
	assert.NotEmpty(t, stub.Version)
	assert.NotEmpty(t, stub.Source)
	assert.Empty(t, stub.Files, "catalog stubs defer file loading until hydration")

	// With an override the stub source becomes the local path.
	base := filepath.Join("repo", "examples", "scaffolds")
	overridden, err := CatalogStubs(base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "aws", "sandbox"), overridden["aws/sandbox"].Source)
}
