package httpmock

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// decodeJSON decodes a JSON response body into v, for test brevity.
func decodeJSON(body io.Reader, v any) error {
	return json.NewDecoder(body).Decode(v)
}

// tGetenv reads an environment variable directly (os.Getenv), for asserting the effect of
// t.Setenv in a test helper.
func tGetenv(t *testing.T, key string) string {
	t.Helper()
	return os.Getenv(key)
}

func TestGitHubMockServer_ReleasesLatest(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterRelease("jqlang", "jq", ReleaseSpec{TagName: "jq-1.6", Prerelease: true})
	mock.RegisterRelease("jqlang", "jq", ReleaseSpec{TagName: "jq-1.7.1"})
	mock.RegisterRelease("jqlang", "jq", ReleaseSpec{TagName: "jq-1.7.0"})

	resp, err := http.Get(mock.URL() + "/api/v3/repos/jqlang/jq/releases/latest")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		TagName string `json:"tag_name"`
	}
	require.NoError(t, decodeJSON(resp.Body, &body))
	// The prerelease is registered first but must be skipped: latest means the first
	// non-draft, non-prerelease entry.
	assert.Equal(t, "jq-1.7.1", body.TagName)
}

func TestGitHubMockServer_ReleasesLatest_NotFound(t *testing.T) {
	mock := NewGitHubMockServer(t)

	resp, err := http.Get(mock.URL() + "/api/v3/repos/nobody/nothing/releases/latest")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGitHubMockServer_ReleasesList_Pagination(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterRelease("owner", "repo", ReleaseSpec{TagName: "v3"})
	mock.RegisterRelease("owner", "repo", ReleaseSpec{TagName: "v2"})
	mock.RegisterRelease("owner", "repo", ReleaseSpec{TagName: "v1"})

	resp, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases?per_page=2")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	link := resp.Header.Get("Link")
	require.NotEmpty(t, link, "first page should carry a Link: rel=next header")
	assert.Contains(t, link, `rel="next"`)
	assert.Contains(t, link, "page=2")

	var page1 []struct {
		TagName string `json:"tag_name"`
	}
	require.NoError(t, decodeJSON(resp.Body, &page1))
	require.Len(t, page1, 2)
	assert.Equal(t, "v3", page1[0].TagName)
	assert.Equal(t, "v2", page1[1].TagName)

	resp2, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases?per_page=2&page=2")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Empty(t, resp2.Header.Get("Link"), "last page must not carry a next Link header")
	var page2 []struct {
		TagName string `json:"tag_name"`
	}
	require.NoError(t, decodeJSON(resp2.Body, &page2))
	require.Len(t, page2, 1)
	assert.Equal(t, "v1", page2[0].TagName)
}

func TestGitHubMockServer_Tags(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterTag("owner", "repo", "v2.0.0")
	mock.RegisterTag("owner", "repo", "v1.0.0")

	resp, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/tags?per_page=1")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var tags []struct {
		Name string `json:"name"`
	}
	require.NoError(t, decodeJSON(resp.Body, &tags))
	require.Len(t, tags, 1)
	assert.Equal(t, "v2.0.0", tags[0].Name)
}

func TestGitHubMockServer_ReleaseAssetDownload(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterReleaseAsset("jqlang", "jq", "jq-1.7.1", "jq-linux-amd64", []byte("fake-binary"))

	resp, err := http.Get(mock.URL() + "/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "fake-binary", string(body))

	// An asset that was never registered must 404, not panic or fall through.
	miss, err := http.Get(mock.URL() + "/jqlang/jq/releases/download/jq-1.7.1/does-not-exist")
	require.NoError(t, err)
	defer miss.Body.Close()
	assert.Equal(t, http.StatusNotFound, miss.StatusCode)
}

func TestGitHubMockServer_ArchiveDownload(t *testing.T) {
	mock := NewGitHubMockServer(t)
	archive := BuildTarGz(map[string]string{"tool-1.0.0/tool": "#!/bin/sh\necho fake\n"})
	mock.RegisterArchive("owner", "repo", "v1.0.0", archive)

	resp, err := http.Get(mock.URL() + "/owner/repo/archive/refs/tags/v1.0.0.tar.gz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, archive, body)
}

func TestGitHubMockServer_RawGHESShape(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterRawFile("cloudposse", "atmos", "main", "tests/fixtures/scenarios/foo/bar.yaml", "settings:\n  key: value\n")

	resp, err := http.Get(mock.URL() + "/raw/cloudposse/atmos/main/tests/fixtures/scenarios/foo/bar.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "settings:\n  key: value\n", string(body))
}

func TestGitHubMockServer_RawGHESShape_FallsThroughToLegacySuffixMatch(t *testing.T) {
	mock := NewGitHubMockServer(t)
	// No exact RegisterRawFile entry, but a legacy suffix registration for the same
	// requested path must still be served -- the raw-shape route must not shadow it.
	mock.RegisterFile("bar.yaml", "legacy: content")

	resp, err := http.Get(mock.URL() + "/raw/cloudposse/atmos/main/tests/fixtures/scenarios/foo/bar.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "legacy: content", string(body))
}

func TestGitHubMockServer_AquaRegistry_IndexAndPackage(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterAquaTool(&AquaTool{
		Owner:         "jqlang",
		Repo:          "jq",
		Asset:         "jq-{{.OS}}-{{.Arch}}",
		SupportedEnvs: []string{"darwin", "linux"},
	})
	mock.RegisterAquaTool(&AquaTool{
		Owner: "kubernetes",
		Repo:  "kubectl",
		Name:  "kubernetes/kubernetes/kubectl",
	})

	// Top-level index carries type/repo_owner/repo_name/name for every registered tool.
	indexResp, err := http.Get(mock.URL() + "/aqua/registry.yaml")
	require.NoError(t, err)
	defer indexResp.Body.Close()
	require.Equal(t, http.StatusOK, indexResp.StatusCode)

	var index struct {
		Packages []struct {
			Type      string `yaml:"type"`
			RepoOwner string `yaml:"repo_owner"`
			RepoName  string `yaml:"repo_name"`
			Name      string `yaml:"name"`
		} `yaml:"packages"`
	}
	indexBody, err := io.ReadAll(indexResp.Body)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(indexBody, &index))
	require.Len(t, index.Packages, 2)

	// Per-package file for the 2-segment tool is servable at pkgs/<owner>/<repo>/registry.yaml.
	pkgResp, err := http.Get(mock.URL() + "/aqua/pkgs/jqlang/jq/registry.yaml")
	require.NoError(t, err)
	defer pkgResp.Body.Close()
	require.Equal(t, http.StatusOK, pkgResp.StatusCode)

	var pkg struct {
		Packages []struct {
			Type          string   `yaml:"type"`
			RepoOwner     string   `yaml:"repo_owner"`
			RepoName      string   `yaml:"repo_name"`
			Asset         string   `yaml:"asset"`
			SupportedEnvs []string `yaml:"supported_envs"`
		} `yaml:"packages"`
	}
	pkgBody, err := io.ReadAll(pkgResp.Body)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(pkgBody, &pkg))
	require.Len(t, pkg.Packages, 1)
	assert.Equal(t, "github_release", pkg.Packages[0].Type)
	assert.Equal(t, "jqlang", pkg.Packages[0].RepoOwner)
	assert.Equal(t, "jq", pkg.Packages[0].RepoName)
	assert.Equal(t, "jq-{{.OS}}-{{.Arch}}", pkg.Packages[0].Asset)
	assert.Equal(t, []string{"darwin", "linux"}, pkg.Packages[0].SupportedEnvs)

	// The 3-segment (monorepo) tool is servable at its full Name path.
	monoResp, err := http.Get(mock.URL() + "/aqua/pkgs/kubernetes/kubernetes/kubectl/registry.yaml")
	require.NoError(t, err)
	defer monoResp.Body.Close()
	assert.Equal(t, http.StatusOK, monoResp.StatusCode)

	// An unregistered package must 404.
	missResp, err := http.Get(mock.URL() + "/aqua/pkgs/nobody/nothing/registry.yaml")
	require.NoError(t, err)
	defer missResp.Body.Close()
	assert.Equal(t, http.StatusNotFound, missResp.StatusCode)
}

func TestGitHubMockServer_FailWith_Persistent(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterReleaseAsset("owner", "repo", "v1", "asset", []byte("data"))
	mock.FailWith("/owner/repo/releases/download", http.StatusForbidden)

	for i := 0; i < 3; i++ {
		resp, err := http.Get(mock.URL() + "/owner/repo/releases/download/v1/asset")
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestGitHubMockServer_FailWithTimes_RecoversAfterN(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterRelease("owner", "repo", ReleaseSpec{TagName: "v1"})
	mock.FailWithTimes("/api/v3/repos/owner/repo/releases", http.StatusForbidden, 1)

	first, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases/latest")
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, first.StatusCode)
	first.Body.Close()

	second, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases/latest")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, second.StatusCode)
	second.Body.Close()
}

func TestGitHubMockServer_RequestLog(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterRelease("owner", "repo", ReleaseSpec{TagName: "v1"})

	resp1, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases/latest")
	require.NoError(t, err)
	resp1.Body.Close()
	resp2, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases/latest")
	require.NoError(t, err)
	resp2.Body.Close()

	assert.Equal(t, 2, mock.RequestCount("/api/v3/repos/owner/repo"))
	assert.Equal(t, 0, mock.RequestCount("/api/v3/repos/other/repo"))

	requests := mock.Requests()
	require.Len(t, requests, 2)
	assert.Equal(t, "GET", requests[0].Method)
	assert.Equal(t, "/api/v3/repos/owner/repo/releases/latest", requests[0].Path)
}

func TestGitHubMockServer_EnvForSubprocess(t *testing.T) {
	mock := NewGitHubMockServer(t)
	env := mock.EnvForSubprocess()

	base := mock.URL()
	assert.Equal(t, base, env["GITHUB_SERVER_URL"])
	assert.Equal(t, base+"/api/v3", env["GITHUB_API_URL"])
	assert.Equal(t, base, env["ATMOS_TOOLCHAIN_GITHUB_URL"])
	assert.Equal(t, base+"/api/v3", env["ATMOS_TOOLCHAIN_GITHUB_API_URL"])
	assert.Equal(t, base+"/aqua", env["ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL"])
}

func TestGitHubMockServer_Setenv(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.Setenv(t)

	assert.Equal(t, mock.URL(), tGetenv(t, "GITHUB_SERVER_URL"))
	assert.Equal(t, mock.URL()+"/api/v3", tGetenv(t, "GITHUB_API_URL"))
}

func TestBuildTarGzAndZip_RoundTrip(t *testing.T) {
	tarball := BuildTarGz(map[string]string{"tool": "content"})
	assert.NotEmpty(t, tarball)

	zipped := BuildZip(map[string]string{"tool": "content"})
	assert.NotEmpty(t, zipped)
	// A zip archive always starts with the local file header signature "PK\x03\x04".
	require.GreaterOrEqual(t, len(zipped), 4)
	assert.Equal(t, []byte("PK\x03\x04"), zipped[:4])
}
