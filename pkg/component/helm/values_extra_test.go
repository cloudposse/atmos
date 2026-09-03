package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
)

// resolveChartRef must map a "repo/name" reference to an explicit RepoURL plus a
// bare chart name when a matching declarative repository is configured, and pass
// an explicit RepoURL through unchanged.
func TestResolveChartRef_RepoResolution(t *testing.T) {
	actx := memoryActionContext(t)

	// "repo/name" with a matching repository -> RepoURL set, bare name returned.
	client := action.NewInstall(actx.cfg)
	spec := &chartSpec{
		Chart:        "bitnami/nginx",
		Repositories: []chartRepository{{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"}},
	}
	assert.Equal(t, "nginx", resolveChartRef(client, spec))
	assert.Equal(t, "https://charts.bitnami.com/bitnami", client.RepoURL)

	// "repo/name" with no matching repository -> passthrough, no RepoURL.
	client = action.NewInstall(actx.cfg)
	spec = &chartSpec{Chart: "unknown/nginx"}
	assert.Equal(t, "unknown/nginx", resolveChartRef(client, spec))
	assert.Empty(t, client.RepoURL)

	// Explicit RepoURL -> passthrough of the bare chart name, RepoURL set.
	client = action.NewInstall(actx.cfg)
	spec = &chartSpec{Chart: "nginx", RepoURL: "https://example.com/charts"}
	assert.Equal(t, "nginx", resolveChartRef(client, spec))
	assert.Equal(t, "https://example.com/charts", client.RepoURL)
}

func TestLoadValuesFile_MissingAndMalformedAndEmpty(t *testing.T) {
	// Missing file -> error.
	_, err := loadValuesFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)

	// Malformed YAML -> error.
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("\tnot: [valid"), 0o600))
	_, err = loadValuesFile(bad)
	require.Error(t, err)

	// Empty file -> empty (non-nil) map, no error.
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(empty, []byte(""), 0o600))
	out, err := loadValuesFile(empty)
	require.NoError(t, err)
	assert.NotNil(t, out)
	assert.Empty(t, out)
}
