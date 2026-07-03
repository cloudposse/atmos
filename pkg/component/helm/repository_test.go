package helm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/repo/v1"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeRepositories(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Helm.Repositories = []schema.HelmRepository{
		{Name: "global", URL: "https://global.example.com"},
		{Name: "shared", URL: "https://old.example.com"},
	}
	section := map[string]any{
		"repositories": []any{
			map[string]any{
				"name":                     "shared",
				"url":                      "https://new.example.com",
				"username":                 "user",
				"password":                 "pass",
				"pass_credentials_all":     true,
				"cert_file":                "cert.pem",
				"key_file":                 "key.pem",
				"ca_file":                  "ca.pem",
				"insecure_skip_tls_verify": true,
			},
			map[string]any{"name": "component", "url": "https://component.example.com"},
		},
	}

	got := mergeRepositories(atmosConfig, section)
	require.Len(t, got, 3)
	assert.Equal(t, "global", got[0].Name)
	assert.Equal(t, repositorySourceGlobal, got[0].Source)
	assert.Equal(t, "shared", got[1].Name)
	assert.Equal(t, "https://new.example.com", got[1].URL)
	assert.Equal(t, repositorySourceComponent, got[1].Source)
	assert.Equal(t, "user", got[1].Username)
	assert.Equal(t, "pass", got[1].Password)
	assert.True(t, got[1].PassCredentialsAll)
	assert.Equal(t, "cert.pem", got[1].CertFile)
	assert.Equal(t, "key.pem", got[1].KeyFile)
	assert.Equal(t, "ca.pem", got[1].CAFile)
	assert.True(t, got[1].InsecureSkipTLSVerify)
	assert.Equal(t, "component", got[2].Name)
}

func TestSetupHelmRepositoriesWritesConfigAndDownloadsIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/index.yaml", r.URL.Path)
		_, _ = w.Write([]byte(`apiVersion: v1
entries:
  nginx:
    - apiVersion: v2
      name: nginx
      version: 1.0.0
generated: "2026-06-30T00:00:00Z"
`))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repositories.yaml")
	repoCache := filepath.Join(dir, "repository")
	t.Setenv("HELM_REPOSITORY_CONFIG", repoFile)
	t.Setenv("HELM_REPOSITORY_CACHE", repoCache)

	err := setupHelmRepositories([]chartRepository{
		{Name: "example", URL: server.URL},
	})
	require.NoError(t, err)

	loaded, err := repo.LoadFile(repoFile)
	require.NoError(t, err)
	entry := loaded.Get("example")
	require.NotNil(t, entry)
	assert.Equal(t, server.URL, entry.URL)
	assert.FileExists(t, filepath.Join(repoCache, "example-index.yaml"))

	err = setupHelmRepositories([]chartRepository{
		{Name: "example", URL: server.URL, Username: "next"},
	})
	require.NoError(t, err)
	loaded, err = repo.LoadFile(repoFile)
	require.NoError(t, err)
	entry = loaded.Get("example")
	require.NotNil(t, entry)
	assert.Equal(t, "next", entry.Username)
}

func TestRepositoryEntryRejectsSlashName(t *testing.T) {
	_, err := repositoryEntry(&chartRepository{Name: "bad/name", URL: "https://example.com"})
	require.Error(t, err)
}

func TestLoadRepositoryFileCreatesEmptyWhenMissing(t *testing.T) {
	file, err := loadRepositoryFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Empty(t, file.Repositories)
}

func TestRepositoryLockPath(t *testing.T) {
	assert.Equal(t, "repositories.lock", repositoryLockPath("repositories.yaml"))
	assert.Equal(t, "repositories.lock", repositoryLockPath("repositories"))
}

func TestLoadRepositoryFileRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repositories.yaml")
	require.NoError(t, os.WriteFile(path, []byte("repositories: ["), 0o600))
	_, err := loadRepositoryFile(path)
	require.Error(t, err)
}
