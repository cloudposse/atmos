package helm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolveUpgradeChartRef(t *testing.T) {
	t.Run("explicit repository url wins", func(t *testing.T) {
		client := &action.Upgrade{}
		ref := resolveUpgradeChartRef(client, &chartSpec{
			Chart:   "nginx",
			RepoURL: "https://charts.example.com",
		})
		assert.Equal(t, "nginx", ref)
		assert.Equal(t, "https://charts.example.com", client.RepoURL)
	})

	t.Run("local and OCI refs pass through", func(t *testing.T) {
		for _, chart := range []string{"./chart", "/abs/chart", "oci://registry.example.com/acme/chart"} {
			client := &action.Upgrade{}
			assert.Equal(t, chart, resolveUpgradeChartRef(client, &chartSpec{Chart: chart}))
			assert.Empty(t, client.RepoURL)
		}
	})

	t.Run("configured repository maps repo prefix", func(t *testing.T) {
		client := &action.Upgrade{}
		ref := resolveUpgradeChartRef(client, &chartSpec{
			Chart: "bitnami/nginx",
			Repositories: []chartRepository{
				{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"},
			},
		})
		assert.Equal(t, "nginx", ref)
		assert.Equal(t, "https://charts.bitnami.com/bitnami", client.RepoURL)
	})

	t.Run("unknown repo prefix stays unchanged", func(t *testing.T) {
		client := &action.Upgrade{}
		assert.Equal(t, "unknown/nginx", resolveUpgradeChartRef(client, &chartSpec{
			Chart: "unknown/nginx",
			Repositories: []chartRepository{
				{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"},
			},
		}))
		assert.Empty(t, client.RepoURL)
	})
}

func TestConfigureReleaseLifecycleActions(t *testing.T) {
	policy := effectiveReleasePolicy{
		OnFailure:        failurePolicyRollback,
		CleanupOnFailure: true,
		WaitStrategy:     kube.LegacyStrategy,
		WaitForJobs:      true,
		Timeout:          12 * time.Minute,
		MaxHistory:       7,
		ChartHooks:       false,
		CRDs:             crdPolicySkip,
	}

	install := &action.Install{}
	installPolicy := policy
	installPolicy.OnFailure = failurePolicyUninstall
	configureInstallLifecycle(install, installPolicy)
	assert.True(t, install.RollbackOnFailure)
	assert.Equal(t, kube.LegacyStrategy, install.WaitStrategy)
	assert.True(t, install.WaitForJobs)
	assert.Equal(t, 12*time.Minute, install.Timeout)
	assert.True(t, install.DisableHooks)
	assert.True(t, install.SkipCRDs)

	upgrade := &action.Upgrade{}
	configureUpgradeLifecycle(upgrade, policy)
	assert.True(t, upgrade.RollbackOnFailure)
	assert.Equal(t, kube.LegacyStrategy, upgrade.WaitStrategy)
	assert.True(t, upgrade.WaitForJobs)
	assert.Equal(t, 12*time.Minute, upgrade.Timeout)
	assert.True(t, upgrade.CleanupOnFail)
	assert.Equal(t, 7, upgrade.MaxHistory)
	assert.True(t, upgrade.DisableHooks)

	uninstall := &action.Uninstall{}
	configureUninstallLifecycle(uninstall, policy, true)
	assert.Equal(t, kube.LegacyStrategy, uninstall.WaitStrategy)
	assert.Equal(t, 12*time.Minute, uninstall.Timeout)
	assert.True(t, uninstall.DisableHooks)
	assert.True(t, uninstall.DryRun)
}

func TestReleaseOperationErrorIncludesEffectivePolicy(t *testing.T) {
	cause := context.DeadlineExceeded
	err := releaseOperationError("upgrade", &chartSpec{
		ReleaseName: "demo",
		Namespace:   "apps",
		Lifecycle: releaseLifecycleResolution{Policy: releaseLifecycle{
			WaitStrategy: kube.StatusWatcherStrategy,
			Timeout:      7 * time.Minute,
		}},
	}, cause)

	require.ErrorIs(t, err, errUtils.ErrHelmReleaseOperation)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "operation=upgrade")
	assert.Contains(t, err.Error(), `release="demo"`)
	assert.Contains(t, err.Error(), `namespace="apps"`)
	assert.Contains(t, err.Error(), "wait_strategy=watcher")
	assert.Contains(t, err.Error(), "timeout=7m0s")
	assert.Contains(t, err.Error(), `component field "timeout"`)
}

func TestClusterOperationsReturnActionContextErrors(t *testing.T) {
	original := newActionContext
	t.Cleanup(func() { newActionContext = original })

	sentinel := errors.New("kube config unavailable")
	newActionContext = func(namespace string) (*actionContext, error) {
		assert.Equal(t, "apps", namespace)
		return nil, sentinel
	}

	spec := &chartSpec{ReleaseName: "nginx", Namespace: "apps"}

	_, err := applyRelease(context.Background(), spec, true)
	require.ErrorIs(t, err, sentinel)

	_, err = getDeployedManifest("nginx", "apps")
	require.ErrorIs(t, err, sentinel)

	err = deleteRelease(context.Background(), spec, false)
	require.ErrorIs(t, err, sentinel)
}

func TestInstallAndUpgradeReleaseLocateChartErrors(t *testing.T) {
	actx := &actionContext{
		cfg:      new(action.Configuration),
		settings: cli.New(),
	}
	spec := &chartSpec{Chart: "missing-chart", ReleaseName: "nginx", Namespace: "apps"}

	_, err := installRelease(context.Background(), actx, spec, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `failed to locate Helm chart "missing-chart"`)
	assert.ErrorIs(t, err, errUtils.ErrHelmRenderFailed)

	_, err = upgradeRelease(context.Background(), actx, spec, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `failed to locate Helm chart "missing-chart"`)
	assert.ErrorIs(t, err, errUtils.ErrHelmRenderFailed)
}
