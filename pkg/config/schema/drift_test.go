package configschema

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/datafetcher"
)

// TestEmbeddedSchemaIsCurrent is the "never stale" mechanism for the atmos.yaml
// JSON Schema: it regenerates the schema from the live configuration structs and
// byte-compares it with the committed artifact. Any change to
// schema.AtmosConfiguration (or anything it transitively references, including
// doc comments) fails this test until the artifact is regenerated.
func TestEmbeddedSchemaIsCurrent(t *testing.T) {
	generated := generatedSchema(t)

	repoRoot, err := RepoRoot()
	require.NoError(t, err)
	committed, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(EmbeddedPath)))
	require.NoError(t, err)

	if !bytes.Equal(generated, committed) {
		t.Fatalf("the atmos.yaml JSON Schema is stale: the Go configuration structs no longer match %s.\n"+
			"Run `go generate ./pkg/config/schema` and commit the regenerated file.", EmbeddedPath)
	}
}

// TestEmbeddedSchemaIsServed asserts the committed artifact is embedded into the
// binary and served at the atmos:// source used by `atmos config schema` and
// `atmos validate schema`.
func TestEmbeddedSchemaIsServed(t *testing.T) {
	served, err := datafetcher.NewDataFetcher(nil).GetData(EmbeddedSource)
	require.NoError(t, err, "the committed schema must be served at %s", EmbeddedSource)

	repoRoot, err := RepoRoot()
	require.NoError(t, err)
	committed, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(EmbeddedPath)))
	require.NoError(t, err)

	require.Equal(t, string(committed), string(served),
		"the embedded copy served at %s must match %s", EmbeddedSource, EmbeddedPath)
}
