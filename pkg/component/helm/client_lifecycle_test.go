package helm

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/cli"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/registry"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

// memoryActionContext builds an actionContext backed by Helm's in-memory storage
// driver and a printing (no-op) Kubernetes client, so install/upgrade/get/delete
// run end-to-end without a cluster. It mirrors Helm's own actionConfigFixture.
func memoryActionContext(t *testing.T) *actionContext {
	t.Helper()

	registryClient, err := registry.NewClient()
	require.NoError(t, err)

	return &actionContext{
		cfg: &action.Configuration{
			Releases:       storage.Init(driver.NewMemory()),
			KubeClient:     &kubefake.FailingKubeClient{PrintingKubeClient: kubefake.PrintingKubeClient{Out: io.Discard}},
			Capabilities:   common.DefaultCapabilities,
			RegistryClient: registryClient,
		},
		settings: cli.New(),
	}
}

// stubActionContext makes newActionContext return the given context for the test.
func stubActionContext(t *testing.T, actx *actionContext) {
	t.Helper()
	original := newActionContext
	t.Cleanup(func() { newActionContext = original })
	newActionContext = func(string) (*actionContext, error) { return actx, nil }
}

func testdataChartSpec(t *testing.T, releaseName string) *chartSpec {
	t.Helper()
	chartPath, err := filepath.Abs(filepath.Join("testdata", "chart"))
	require.NoError(t, err)
	return &chartSpec{
		Chart:       chartPath,
		ReleaseName: releaseName,
		Namespace:   "testns",
		Values:      map[string]any{"replicaCount": 2, "image": map[string]any{"tag": "1.0"}},
	}
}

// TestClientReleaseLifecycleInMemory exercises applyRelease (install then
// upgrade), getDeployedManifest (found then empty), and deleteRelease
// (success then idempotent) against the in-memory storage driver.
func TestClientReleaseLifecycleInMemory(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "lifecycle")

	// No release yet -> applyRelease takes the install branch.
	manifest, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)
	assert.Contains(t, manifest, "kind: ConfigMap")
	assert.Contains(t, manifest, "name: lifecycle")

	// The installed release is now the diff baseline.
	deployed, err := getDeployedManifest("lifecycle", "testns")
	require.NoError(t, err)
	assert.Contains(t, deployed, "kind: ConfigMap")

	// Release exists -> applyRelease takes the upgrade branch.
	upgraded, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)
	assert.Contains(t, upgraded, "kind: ConfigMap")

	// Delete removes it; deleting an absent release is a no-op (idempotent).
	require.NoError(t, deleteRelease("lifecycle", "testns"))
	require.NoError(t, deleteRelease("lifecycle", "testns"))

	// After delete the baseline is empty (release not found), not an error.
	deployed, err = getDeployedManifest("lifecycle", "testns")
	require.NoError(t, err)
	assert.Empty(t, deployed)
}

// TestApplyReleaseDryRunInstall covers the dry-run install branch: the manifest
// is rendered for preview but the release is not persisted.
func TestApplyReleaseDryRunInstall(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "preview")

	manifest, err := applyRelease(context.Background(), spec, true)
	require.NoError(t, err)
	assert.Contains(t, manifest, "kind: ConfigMap")

	// A dry run must not persist a release.
	deployed, err := getDeployedManifest("preview", "testns")
	require.NoError(t, err)
	assert.Empty(t, deployed)
}

// TestUpgradeReleaseDryRun seeds an existing release then takes the dry-run
// upgrade branch.
func TestUpgradeReleaseDryRun(t *testing.T) {
	actx := memoryActionContext(t)
	require.NoError(t, actx.cfg.Releases.Create(release.Mock(&release.MockReleaseOptions{
		Name:      "seeded",
		Namespace: "testns",
	})))
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "seeded")

	manifest, err := applyRelease(context.Background(), spec, true)
	require.NoError(t, err)
	assert.Contains(t, manifest, "kind: ConfigMap")
}
