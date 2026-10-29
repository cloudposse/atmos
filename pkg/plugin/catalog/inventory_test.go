package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgit "github.com/cloudposse/atmos/pkg/git"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_inventory_test.go -package=catalog github.com/cloudposse/atmos/pkg/git Runner

var (
	_ = pkgit.RunResult{Stdout: ""}
	_ = ReleaseSelection{Name: "", Version: "", Repository: ""}
)

const catalogInventory = `packages:
  fixture:
    path: plugins/fixture
    repository: oci://registry.example.com/team/fixture
`

func inventoryRunner(t *testing.T, files map[string]string, failure string) *MockRunner {
	t.Helper()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), pkgit.RunOptions{}).DoAndReturn(
		func(_ context.Context, _ string, args []string, _ pkgit.RunOptions) (pkgit.RunResult, error) {
			key := strings.Join(args, " ")
			if key == failure {
				return pkgit.RunResult{}, context.Canceled
			}
			raw, ok := files[key]
			require.True(t, ok, "unexpected Git operation: %s", key)
			return pkgit.RunResult{Stdout: raw}, nil
		},
	).AnyTimes()
	return runner
}

func TestReadInventoryWithoutReleasePlanning(t *testing.T) {
	files := map[string]string{
		"rev-parse --verify --end-of-options accepted^{commit}": "commit-sha\n",
		"show commit-sha:catalog.yaml":                          catalogInventory + "  another:\n    path: plugins/another\n    repository: oci://registry.example.com/team/another\n",
		"show commit-sha:plugins/fixture/plugin.yaml":           syncManifest,
		"show commit-sha:plugins/another/plugin.yaml":           strings.ReplaceAll(syncManifest, "fixture", "another"),
	}
	// The runner rejects any build-history or diff command. Both packages have no CI runners.
	releases, err := ReadInventory(t.Context(), WithInventory("catalog.yaml"), WithHead("accepted"), WithInventoryRunner(inventoryRunner(t, files, "")))
	require.NoError(t, err)
	require.Equal(t, []ReleaseSelection{
		{Name: "another", Version: "1.2.3", Repository: "oci://registry.example.com/team/another"},
		{Name: "fixture", Version: "1.2.3", Repository: "oci://registry.example.com/team/fixture"},
	}, releases)
}

func TestReadInventoryRejectsInvalidSources(t *testing.T) {
	for _, tc := range []struct{ name, inventory, manifest, fail string }{
		{name: "invalid YAML", inventory: "["},
		{name: "empty inventory", inventory: "{}"},
		{name: "unknown field", inventory: catalogInventory + "typo: true\n"},
		{name: "multiple documents", inventory: catalogInventory + "---\n{}"},
		{name: "unsafe path", inventory: strings.ReplaceAll(catalogInventory, "plugins/fixture", "../fixture")},
		{name: "invalid name", inventory: strings.ReplaceAll(catalogInventory, "fixture:", "Bad_Name:")},
		{name: "missing OCI scheme", inventory: strings.ReplaceAll(catalogInventory, "oci://", "")},
		{name: "invalid repository", inventory: strings.ReplaceAll(catalogInventory, "registry.example.com/team/fixture", "bad repository")},
		{name: "malformed manifest", manifest: "["},
		{name: "wrong package", manifest: strings.ReplaceAll(syncManifest, "fixture", "another")},
		{name: "prerelease", manifest: strings.ReplaceAll(syncManifest, "1.2.3", "1.2.3-rc.1")},
		{name: "build metadata", manifest: strings.ReplaceAll(syncManifest, "1.2.3", "1.2.3+build")},
		{name: "revision read failure", fail: "rev-parse --verify --end-of-options HEAD^{commit}"},
		{name: "inventory read failure", fail: "show commit-sha:plugins/releases.yaml"},
		{name: "manifest read failure", fail: "show commit-sha:plugins/fixture/plugin.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"rev-parse --verify --end-of-options HEAD^{commit}": "commit-sha",
				"show commit-sha:plugins/releases.yaml":             catalogInventory,
				"show commit-sha:plugins/fixture/plugin.yaml":       syncManifest,
			}
			if tc.inventory != "" {
				files["show commit-sha:plugins/releases.yaml"] = tc.inventory
			}
			if tc.manifest != "" {
				files["show commit-sha:plugins/fixture/plugin.yaml"] = tc.manifest
			}
			releases, err := ReadInventory(t.Context(), WithInventoryRunner(inventoryRunner(t, files, tc.fail)))
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Nil(t, releases)
			if tc.fail != "" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestReadInventoryRejectsInvalidOptions(t *testing.T) {
	for _, option := range []InventoryOption{WithHead(""), WithInventory("../inventory.yaml"), WithInventoryRunner(nil)} {
		_, err := ReadInventory(t.Context(), option)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	runner := inventoryRunner(t, map[string]string{"rev-parse --verify --end-of-options HEAD^{commit}": ""}, "")
	_, err := ReadInventory(t.Context(), WithInventoryRunner(runner))
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}
