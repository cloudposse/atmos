package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/version"
	"github.com/cloudposse/atmos/tests/testhelpers"
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
		assert.NotContains(t, entry.Source, "ref=", "catalog sources must not hardcode a ref; ResolvedSource applies one at runtime")
		assert.NotEmpty(t, entry.Version)
		assert.NotEmpty(t, entry.Description)
	}
}

// TestCatalogEntriesExistOnDisk asserts that every catalog entry's advertised
// cloud/tier actually has a matching examples/scaffolds/<cloud>/<tier>/scaffold.yaml
// in the working tree. It catches catalog/directory drift (renames, typos, deletions)
// independently of network access or go-getter.
func TestCatalogEntriesExistOnDisk(t *testing.T) {
	entries, err := LoadCatalog()
	require.NoError(t, err)

	root, err := testhelpers.FindRepoRoot()
	require.NoError(t, err)

	for _, e := range entries {
		scaffoldPath := filepath.Join(root, "examples", "scaffolds", e.Cloud, e.Tier, "scaffold.yaml")
		_, statErr := os.Stat(scaffoldPath)
		assert.NoErrorf(t, statErr, "catalog entry %s expects %s to exist", e.Name, scaffoldPath)
	}
}

func TestCatalogEntry_ResolvedSource(t *testing.T) {
	e := CatalogEntry{
		Cloud:  "aws",
		Tier:   "landing-zone",
		Source: "github.com/cloudposse/atmos.git//examples/scaffolds/aws/landing-zone",
	}

	// No override, no build commit (the default for `go test`, `go run`, `go
	// install`): falls back to ref=main, no shallow-clone override needed.
	assert.Equal(t, e.Source+"?ref=main", e.ResolvedSource(""))

	// Override: a local path under <override>/<cloud>/<tier>.
	got := e.ResolvedSource(filepath.Join("repo", "examples", "scaffolds"))
	assert.Equal(t, filepath.Join("repo", "examples", "scaffolds", "aws", "landing-zone"), got)
}

// TestCatalogEntry_ResolvedSource_PinnedToBuildCommit verifies that once a
// binary is built with a commit SHA (via ldflags, see scripts/build-atmos.sh
// and .goreleaser*.yml), catalog sources pin to that exact commit and disable
// the shallow clone go-getter otherwise applies -- git rejects a shallow
// clone (`--depth`) combined with a ref that isn't a branch or tag, which a
// full commit SHA never is.
func TestCatalogEntry_ResolvedSource_PinnedToBuildCommit(t *testing.T) {
	const sha = "0cf62afa883b1546f07f2eaf2d6f1690353d31b7"
	original := version.Commit
	version.Commit = sha
	t.Cleanup(func() { version.Commit = original })

	e := CatalogEntry{
		Cloud:  "aws",
		Tier:   "app",
		Source: "github.com/cloudposse/atmos.git//examples/scaffolds/aws/app",
	}

	assert.Equal(t, e.Source+"?ref="+sha+"&depth=0", e.ResolvedSource(""))
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
	assert.Equal(t, "github.com/cloudposse/atmos.git//examples/scaffolds/aws/app?ref=main", stubs["aws/app"].Source,
		"without a build commit, the default ref falls back to main")

	// With an override the stub source becomes the local path.
	base := filepath.Join("repo", "examples", "scaffolds")
	overridden, err := CatalogStubs(base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "aws", "app"), overridden["aws/app"].Source)
	assert.Equal(t, filepath.Join(base, "aws", "landing-zone"), overridden["aws/landing-zone"].Source)
	assert.Equal(t, filepath.Join(base, "gcp", "landing-zone"), overridden["gcp/landing-zone"].Source)
	assert.Equal(t, filepath.Join(base, "azure", "landing-zone"), overridden["azure/landing-zone"].Source)
}
