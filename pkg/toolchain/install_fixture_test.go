package toolchain

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	toolinstaller "github.com/cloudposse/atmos/pkg/toolchain/installer"
)

const fixtureBinaryContents = "local installation fixture\n"

// localInstallFixture exercises real HTTP download, archive extraction, and
// version-file updates without downloading large public Terraform/Helm releases.
// It returns explicit configuration, so instance-based tests need no globals.
func localInstallFixture(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	archives := map[string][]byte{}
	for _, name := range []string{"terraform", "helm"} {
		var buffer bytes.Buffer
		archive := zip.NewWriter(&buffer)
		filename := name
		if runtime.GOOS == "windows" {
			filename += ".exe"
		}
		header := &zip.FileHeader{Name: filename, Method: zip.Deflate}
		header.SetMode(0o755)
		entry, err := archive.CreateHeader(header)
		require.NoError(t, err)
		_, err = entry.Write([]byte(fixtureBinaryContents))
		require.NoError(t, err)
		require.NoError(t, archive.Close())
		archives["/"+name+".zip"] = buffer.Bytes()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset, ok := archives[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(asset)
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	tools := map[string]any{}
	for _, id := range []string{"hashicorp/terraform", "helm/helm"} {
		name := strings.Split(id, "/")[1]
		tools[id] = map[string]any{"type": "http", "url": server.URL + "/" + name + ".zip", "format": "zip"}
	}
	return &schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: filepath.Join(root, ".tool-versions"), InstallPath: filepath.Join(root, "tools"),
		Aliases:    map[string]string{"terraform": "hashicorp/terraform", "helm": "helm/helm"},
		Registries: []schema.ToolchainRegistry{{Name: "fixture", Type: "atmos", Priority: 100, Tools: tools}},
	}}
}

// useLocalInstallFixture is only for serial public-adapter tests, which exercise
// the actual configuration/global adapter. Instance tests use the returned config directly.
func useLocalInstallFixture(t *testing.T, versions map[string][]string) *schema.AtmosConfiguration {
	t.Helper()
	config := localInstallFixture(t)
	previous := atmosConfig
	SetAtmosConfig(config)
	t.Cleanup(func() { SetAtmosConfig(previous) })
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	require.NoError(t, SaveToolVersions(config.Toolchain.VersionsFile, &ToolVersions{Tools: versions}))
	return config
}

func fixtureInstaller(t *testing.T, config *schema.AtmosConfiguration) *Installer {
	t.Helper()
	reg, err := NewRegistry(config)
	require.NoError(t, err)
	return toolinstaller.New(WithBinDir(filepath.Join(config.Toolchain.InstallPath, "bin")),
		WithCacheDir(t.TempDir()), WithAtmosConfig(config), WithConfiguredRegistry(reg))
}

func assertFixtureInstalled(t *testing.T, config *schema.AtmosConfiguration, owner, repo, version string) {
	t.Helper()
	name := repo
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	content, err := os.ReadFile(filepath.Join(config.Toolchain.InstallPath, "bin", owner, repo, version, name))
	require.NoError(t, err)
	assert.Equal(t, fixtureBinaryContents, string(content))
}
