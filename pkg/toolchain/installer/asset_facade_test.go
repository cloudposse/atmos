package installer

import (
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

// TestBuildAssetURL_ServedByFacadeAcrossPlatforms verifies that the exact URL
// buildAssetURLForPlatform computes for a raw (non-archive) binary asset -- including the
// Windows .exe suffix ensureWindowsExeExtensionForOS appends -- is the URL a mock-registered
// asset for that platform is actually reachable at. Regression coverage for a case where a
// test registered an asset under runtime.GOOS/GOARCH's raw name without accounting for the
// Windows .exe suffix, causing a 404 that only reproduced on Windows CI runners
// (tests/toolchain_custom_commands_test.go, tests/toolchain_aqua_tools_test.go).
//
// This lives in its own file, not asset_test.go: it points ToolchainEndpoints() at the mock
// via t.Setenv, which Go's testing package forbids combining with t.Parallel -- and this
// package's paralleltest lint enforcement covers asset_test.go by name.
func TestBuildAssetURL_ServedByFacadeAcrossPlatforms(t *testing.T) {
	cases := []struct {
		goos   string
		goarch string
	}{
		{"windows", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
	}

	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			mock := httpmock.NewGitHubMockServer(t)
			mock.Setenv(t)

			inst := &Installer{}
			tool := &registry.Tool{
				Type:      "github_release",
				RepoOwner: "jqlang",
				RepoName:  "jq",
				Asset:     "jq-{{.OS}}-{{.Arch}}",
			}

			assetURL, err := inst.buildAssetURLForPlatform(tool, "1.7.1", tc.goos, tc.goarch)
			require.NoError(t, err)

			assetName := path.Base(assetURL)
			mock.RegisterReleaseAsset("jqlang", "jq", "1.7.1", assetName, []byte("fake-jq"))

			resp, err := http.Get(assetURL)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "facade should serve the installer-computed URL %s", assetURL)
		})
	}
}
