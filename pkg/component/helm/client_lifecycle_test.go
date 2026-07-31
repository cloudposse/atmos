package helm

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

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

	errUtils "github.com/cloudposse/atmos/errors"
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
		Release:     releasePolicyInput{},
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
	result, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)
	assert.Equal(t, "install", result.Operation)
	assert.Contains(t, result.Manifest, "kind: ConfigMap")
	assert.Contains(t, result.Manifest, "name: lifecycle")
	assert.Contains(t, result.Manifest, `name: "lifecycle-settings"`)

	// The installed release is now the diff baseline.
	deployed, err := getDeployedManifest("lifecycle", "testns")
	require.NoError(t, err)
	assert.Contains(t, deployed, "kind: ConfigMap")
	assert.Contains(t, deployed, `name: "lifecycle-settings"`)

	// Release exists -> applyRelease takes the upgrade branch.
	upgraded, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)
	assert.Equal(t, "upgrade", upgraded.Operation)
	assert.Contains(t, upgraded.Manifest, "kind: ConfigMap")

	// Delete removes it; deleting an absent release is a no-op (idempotent).
	require.NoError(t, deleteRelease(context.Background(), spec, false))
	require.NoError(t, deleteRelease(context.Background(), spec, false))

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

	result, err := applyRelease(context.Background(), spec, true)
	require.NoError(t, err)
	assert.Contains(t, result.Manifest, "kind: ConfigMap")

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

	result, err := applyRelease(context.Background(), spec, true)
	require.NoError(t, err)
	assert.Contains(t, result.Manifest, "kind: ConfigMap")
}

func TestDeleteReleaseDryRunPreservesRelease(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "delete-preview")

	_, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)
	require.NoError(t, deleteRelease(context.Background(), spec, true))

	deployed, err := getDeployedManifest(spec.ReleaseName, spec.Namespace)
	require.NoError(t, err)
	assert.Contains(t, deployed, "kind: ConfigMap")
}

func TestApplyReleaseHonorsCanceledContext(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "canceled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := applyRelease(ctx, spec, false)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, errUtils.ErrHelmReleaseUpgrade)

	deployed, getErr := getDeployedManifest(spec.ReleaseName, spec.Namespace)
	require.NoError(t, getErr)
	assert.Empty(t, deployed)
}

func TestDeleteReleaseHonorsCanceledContext(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "delete-canceled")
	_, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, deleteRelease(ctx, spec, false), context.Canceled)

	deployed, getErr := getDeployedManifest(spec.ReleaseName, spec.Namespace)
	require.NoError(t, getErr)
	assert.NotEmpty(t, deployed)
}

func TestApplyReleaseUsesLifecycleTimeoutAndWaitContext(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "bounded-upgrade")

	_, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)

	kubeClient, ok := actx.cfg.KubeClient.(*kubefake.FailingKubeClient)
	require.True(t, ok)
	kubeClient.RecordedWaitOptions = nil
	kubeClient.WaitDuration = 2 * time.Second
	timeout := "25ms"
	spec.Release.Upgrade.Timeout = &timeout

	started := time.Now()
	result, err := applyRelease(context.Background(), spec, false)
	elapsed := time.Since(started)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, releaseOperationUpgrade, result.Operation)
	assert.Less(t, elapsed, time.Second, "upgrade must return at the lifecycle deadline")
	assert.NotEmpty(t, kubeClient.RecordedWaitOptions, "Helm waiters must receive the operation context")
}

func TestReleaseOperationContextPreservesZeroTimeout(t *testing.T) {
	parent := context.Background()
	ctx, cancel := releaseOperationContext(parent, 0)
	defer cancel()

	assert.Equal(t, parent, ctx)
	_, hasDeadline := ctx.Deadline()
	assert.False(t, hasDeadline)
}

func TestUpgradeReleasePrunesDefaultHistory(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "history")

	for revision := 0; revision < defaultHelmMaxHistory+3; revision++ {
		spec.Values["replicaCount"] = revision + 1
		_, err := applyRelease(context.Background(), spec, false)
		require.NoError(t, err)
	}

	history, err := actx.cfg.Releases.History(spec.ReleaseName)
	require.NoError(t, err)
	assert.Len(t, history, defaultHelmMaxHistory)
}

func TestUpgradeReleaseUnlimitedHistory(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "unlimited-history")
	spec.Lifecycle.Policy.MaxHistory = 0

	const revisions = defaultHelmMaxHistory + 3
	for revision := 0; revision < revisions; revision++ {
		spec.Values["replicaCount"] = revision + 1
		_, err := applyRelease(context.Background(), spec, false)
		require.NoError(t, err)
	}

	history, err := actx.cfg.Releases.History(spec.ReleaseName)
	require.NoError(t, err)
	assert.Len(t, history, revisions)
}
