package vender

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

func TestVendorComponentPullCommand(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	commonFiles := []string{"main.tf", "outputs.tf", "providers.tf", "variables.tf", "versions.tf"}
	for _, tc := range []struct {
		component string
		included  []string
		excluded  []string
	}{
		{
			component: "infra/vpc-flow-logs-bucket",
			included:  commonFiles,
			excluded:  []string{"context.tf", "README.md", "default.auto.tfvars", "ignored.txt"},
		},
		{
			component: "infra/account-map",
			included: append(
				append([]string{}, commonFiles...),
				"dynamic-roles.tf", "README.md", "remote-state.tf", "default.auto.tfvars",
				"modules/iam-roles/context.tf", "modules/iam-roles/main.tf",
				"modules/iam-roles/outputs.tf", "modules/iam-roles/variables.tf",
				"modules/roles-to-principals/context.tf", "modules/roles-to-principals/main.tf",
				"modules/roles-to-principals/outputs.tf", "modules/roles-to-principals/variables.tf",
			),
			excluded: []string{"ignored.txt"},
		},
	} {
		t.Run(tc.component, func(t *testing.T) {
			// Retain the real manifest's inclusion/exclusion rules and version template,
			// but fetch a controlled archive instead of depending on GitHub availability.
			componentConfig, _, err := e.ReadAndProcessComponentVendorConfigFile(&atmosConfig, tc.component, "terraform")
			require.NoError(t, err)
			files := append(append([]string{}, tc.included...), tc.excluded...)
			componentConfig.Spec.Source.Uri = serveComponentArchive(t, files, componentConfig.Spec.Source.Version) + "/{{.Version}}/source.zip"

			// Never materialize or delete files in the shared repository fixture.
			isolatedConfig := atmosConfig
			isolatedConfig.BasePath = t.TempDir()
			// InitCliConfig caches the resolved root; move it with the declared root.
			isolatedConfig.BasePathAbsolute = isolatedConfig.BasePath
			isolatedConfig.Vendor.LockFile = lockfile.DefaultFileName
			componentPath := filepath.Join(isolatedConfig.BasePath, "components", "terraform", tc.component)
			require.NoError(t, os.MkdirAll(componentPath, 0o755))
			err = e.ExecuteComponentVendorInternal(&isolatedConfig, &componentConfig.Spec, tc.component, componentPath, install.InstallOptions{})
			require.NoError(t, err, "%+v", err)

			for _, file := range tc.included {
				content, err := os.ReadFile(filepath.Join(componentPath, file))
				require.NoError(t, err)
				assert.Equal(t, "# vendored "+file+"\n", string(content))
			}
			for _, file := range tc.excluded {
				assert.NoFileExists(t, filepath.Join(componentPath, file))
			}
			assertIsolatedComponentReceipt(t, &isolatedConfig, tc.component, tc.included)
		})
	}
}

func assertIsolatedComponentReceipt(t *testing.T, config *schema.AtmosConfiguration, component string, included []string) {
	t.Helper()
	expectedPath := filepath.Join(config.BasePath, lockfile.DefaultFileName)
	require.Equal(t, expectedPath, lockfile.Path(config))
	require.FileExists(t, expectedPath)
	receipt, err := lockfile.Load(config)
	require.NoError(t, err)
	require.Len(t, receipt.Artifacts, 1)
	for id := range receipt.Artifacts {
		assert.Equal(t, component, receipt.Artifacts[id].Name)
		assert.Equal(t, "components/terraform/"+component, receipt.Artifacts[id].Target)
		paths := make([]string, 0, len(receipt.Artifacts[id].Files))
		for _, file := range receipt.Artifacts[id].Files {
			paths = append(paths, file.Path)
		}
		assert.ElementsMatch(t, included, paths, "receipt must own exactly the included files")
	}
	drift, err := lockfile.Verify(config, receipt)
	require.NoError(t, err)
	assert.Empty(t, drift)
}

// serveComponentArchive serves a versioned ZIP so the test exercises HTTP fetching,
// archive extraction, and template expansion without external network dependencies.
func serveComponentArchive(t *testing.T, files []string, version string) string {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, file := range files {
		entry, err := writer.Create(file)
		require.NoError(t, err)
		_, err = entry.Write([]byte("# vendored " + file + "\n"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+version+"/source.zip" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive.Bytes())
	}))
	t.Cleanup(server.Close)
	return server.URL
}
