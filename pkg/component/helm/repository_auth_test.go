package helm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
)

func authenticatedChartArchive(t *testing.T) []byte {
	t.Helper()
	chart, err := loader.Load(filepath.Join("testdata", "chart"))
	require.NoError(t, err)
	path, err := chartutil.Save(chart, t.TempDir())
	require.NoError(t, err)
	archive, err := os.ReadFile(path)
	require.NoError(t, err)
	return archive
}

func isolateChartCache(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(root, "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(root, "repository"))
	t.Setenv("HELM_CONTENT_CACHE", filepath.Join(root, "content"))
}

func writeAuthChartIndex(w http.ResponseWriter, chartURL string) {
	_, _ = fmt.Fprintf(w, "apiVersion: v1\nentries:\n  testchart:\n    - apiVersion: v2\n      name: testchart\n      version: 0.1.0\n      urls:\n        - %s\n", chartURL)
}

// TestAuthenticatedRepositoryChartDownload covers the second index fetch performed
// by LocateChart after setup has already authenticated successfully (issue #3464).
func TestAuthenticatedRepositoryChartDownload(t *testing.T) {
	archive := authenticatedChartArchive(t)
	for _, operation := range []string{"render", "upgrade", "invalid password"} {
		t.Run(operation, func(t *testing.T) {
			isolateChartCache(t)
			var indexes, downloads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				if !ok || username != "gitlab-user" || password != "test-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				switch r.URL.Path {
				case "/index.yaml":
					indexes.Add(1)
					writeAuthChartIndex(w, "testchart-0.1.0.tgz")
				case "/testchart-0.1.0.tgz":
					downloads.Add(1)
					_, _ = w.Write(archive)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			spec := &chartSpec{
				Chart: "gitlab/testchart", ReleaseName: "unit", Namespace: "testns",
				Repositories: []chartRepository{{Name: "gitlab", URL: server.URL, Username: "gitlab-user", Password: "test-token"}},
			}
			require.NoError(t, setupHelmRepositories(spec.Repositories))
			require.Equal(t, int32(1), indexes.Load())
			if operation == "invalid password" {
				spec.Repositories[0].Password = "wrong"
			}
			if operation == "upgrade" {
				client := &action.Upgrade{}
				ref := resolveUpgradeChartRef(client, spec)
				path, err := client.LocateChart(ref, newSettings())
				require.NoError(t, err)
				_, err = loadChart(path)
				require.NoError(t, err)
			} else {
				manifest, err := renderManifest(t.Context(), spec)
				if operation == "invalid password" {
					require.ErrorContains(t, err, "401")
					assert.Zero(t, downloads.Load())
					return
				}
				require.NoError(t, err)
				assert.Contains(t, manifest, "kind: ConfigMap")
				assert.Contains(t, manifest, "name: unit")
			}
			assert.GreaterOrEqual(t, indexes.Load(), int32(2))
			assert.Positive(t, downloads.Load())
		})
	}
}

func TestRepositoryCredentialsAcrossOrigins(t *testing.T) {
	archive := authenticatedChartArchive(t)
	for _, passAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("pass_credentials_all=%t", passAll), func(t *testing.T) {
			isolateChartCache(t)
			var downloads atomic.Int32
			chartServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads.Add(1)
				user, password, ok := r.BasicAuth()
				assert.Equal(t, passAll, ok)
				if passAll {
					assert.Equal(t, "user", user)
					assert.Equal(t, "token", password)
				} else {
					assert.Empty(t, r.Header.Get("Authorization"))
				}
				_, _ = w.Write(archive)
			}))
			t.Cleanup(chartServer.Close)
			repoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "user" || password != "token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				writeAuthChartIndex(w, chartServer.URL+"/testchart-0.1.0.tgz")
			}))
			t.Cleanup(repoServer.Close)
			client := &action.Install{}
			ref := resolveChartRef(client, &chartSpec{
				Chart:        "private/testchart",
				Repositories: []chartRepository{{Name: "private", URL: repoServer.URL, Username: "user", Password: "token", PassCredentialsAll: passAll}},
			})
			path, err := client.LocateChart(ref, newSettings())
			require.NoError(t, err)
			assert.FileExists(t, path)
			assert.Positive(t, downloads.Load())
		})
	}
}

func TestRepositoryTLSOptionsForInstallAndUpgrade(t *testing.T) {
	spec := &chartSpec{Chart: "private/testchart", Repositories: []chartRepository{{
		Name: "private", URL: "https://charts.example.test", Username: "user", Password: "token",
		CAFile: "ca.pem", CertFile: "cert.pem", KeyFile: "key.pem", InsecureSkipTLSVerify: true, PassCredentialsAll: true,
	}}}
	install, upgrade := &action.Install{}, &action.Upgrade{}
	assert.Equal(t, "testchart", resolveChartRef(install, spec))
	assert.Equal(t, "testchart", resolveUpgradeChartRef(upgrade, spec))
	for _, options := range []*action.ChartPathOptions{&install.ChartPathOptions, &upgrade.ChartPathOptions} {
		assert.Equal(t, "https://charts.example.test", options.RepoURL)
		assert.Equal(t, "user", options.Username)
		assert.Equal(t, "token", options.Password)
		assert.Equal(t, "ca.pem", options.CaFile)
		assert.Equal(t, "cert.pem", options.CertFile)
		assert.Equal(t, "key.pem", options.KeyFile)
		assert.True(t, options.InsecureSkipTLSVerify)
		assert.True(t, options.PassCredentialsAll)
	}
}
