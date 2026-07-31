package helm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
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

// recordingKubeClient wraps the fake kube client to record every Build input, so a
// test can assert whether Helm's CreateNamespace path ran: the install action only
// builds a `kind: Namespace` object (and then creates it) when CreateNamespace is
// true. The fake's Build returns an empty resource list, so inspecting Create's
// arguments cannot distinguish the two — the Build input is the observable signal.
type recordingKubeClient struct {
	*kubefake.FailingKubeClient
	BuiltDocs []string
}

type failNextUpdateKubeClient struct {
	*kubefake.FailingKubeClient
	failNext bool
}

func (f *failNextUpdateKubeClient) Update(current, target kube.ResourceList, options ...kube.ClientUpdateOption) (*kube.Result, error) {
	if f.failNext {
		f.failNext = false
		return &kube.Result{}, errors.New("forced upgrade failure")
	}
	return f.FailingKubeClient.Update(current, target, options...)
}

func (r *recordingKubeClient) Build(reader io.Reader, strict bool) (kube.ResourceList, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	r.BuiltDocs = append(r.BuiltDocs, string(data))
	return r.FailingKubeClient.Build(bytes.NewReader(data), strict)
}

// recordingActionContext returns an in-memory action context whose kube client
// records Build inputs, plus the recorder itself for assertions.
func recordingActionContext(t *testing.T) (*actionContext, *recordingKubeClient) {
	t.Helper()
	actx := memoryActionContext(t)
	rec := &recordingKubeClient{FailingKubeClient: actx.cfg.KubeClient.(*kubefake.FailingKubeClient)}
	actx.cfg.KubeClient = rec
	return actx, rec
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

	// The installed release is now the diff baseline.
	deployed, err := getDeployedManifest("lifecycle", "testns")
	require.NoError(t, err)
	assert.Contains(t, deployed, "kind: ConfigMap")

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

func TestUpgradeRollbackPreservesHistoryLimit(t *testing.T) {
	maxHistory := 3

	actx := memoryActionContext(t)
	kubeClient := &failNextUpdateKubeClient{
		FailingKubeClient: actx.cfg.KubeClient.(*kubefake.FailingKubeClient),
	}
	actx.cfg.KubeClient = kubeClient
	stubActionContext(t, actx)

	spec := testdataChartSpec(t, "rollback-history")
	onFailure := string(failurePolicyRollback)
	spec.Release.History.Max = &maxHistory
	spec.Release.Upgrade.OnFailure = &onFailure

	for revision := 1; revision <= maxHistory; revision++ {
		spec.Values["replicaCount"] = revision
		_, err := applyRelease(context.Background(), spec, false)
		require.NoError(t, err)
	}

	kubeClient.failNext = true
	spec.Values["replicaCount"] = maxHistory + 1
	_, err := applyRelease(context.Background(), spec, false)
	require.ErrorContains(t, err, "forced upgrade failure")

	history, err := actx.cfg.Releases.History(spec.ReleaseName)
	require.NoError(t, err)
	assert.Len(t, history, maxHistory)
	assert.Equal(t, maxHistory, actx.cfg.Releases.MaxHistory)
}

func TestApplyReleaseHonorsCanceledContext(t *testing.T) {
	actx := memoryActionContext(t)
	stubActionContext(t, actx)
	spec := testdataChartSpec(t, "canceled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := applyRelease(ctx, spec, false)
	require.ErrorIs(t, err, context.Canceled)
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
