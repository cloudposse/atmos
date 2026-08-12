package helm

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	authkube "github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestVerifyExpectedKubernetesEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	config := clientcmdapi.NewConfig()
	config.CurrentContext = "example"
	config.Clusters["example-cluster"] = &clientcmdapi.Cluster{Server: "https://example.invalid"}
	config.Contexts["example"] = &clientcmdapi.Context{Cluster: "example-cluster"}
	require.NoError(t, clientcmd.WriteToFile(*config, path))
	t.Setenv("KUBECONFIG", path)

	t.Run("no integration expectation allows ambient kubeconfig", func(t *testing.T) {
		t.Setenv(authkube.EndpointGuardEnv, "")
		t.Setenv(authkube.ExpectedServerEnv, "")
		require.NoError(t, verifyExpectedKubernetesEndpoint(cli.New()))
	})
	t.Run("expectation is inert without the opt-in guard", func(t *testing.T) {
		t.Setenv(authkube.EndpointGuardEnv, "")
		t.Setenv(authkube.ExpectedServerEnv, "https://other.invalid")
		require.NoError(t, verifyExpectedKubernetesEndpoint(cli.New()))
	})
	t.Run("matching endpoint proceeds", func(t *testing.T) {
		t.Setenv(authkube.EndpointGuardEnv, "true")
		t.Setenv(authkube.ExpectedServerEnv, "https://example.invalid/")
		require.NoError(t, verifyExpectedKubernetesEndpoint(cli.New()))
	})
	t.Run("mismatched endpoint fails closed", func(t *testing.T) {
		t.Setenv(authkube.EndpointGuardEnv, "true")
		t.Setenv(authkube.ExpectedServerEnv, "https://other.invalid")
		err := verifyExpectedKubernetesEndpoint(cli.New())
		require.ErrorIs(t, err, errUtils.ErrKubernetesEndpointMismatch)
		assert.Contains(t, err.Error(), "https://example.invalid")
	})
}

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

	err = deleteRelease("nginx", "apps")
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

	_, err = upgradeRelease(context.Background(), actx, spec, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `failed to locate Helm chart "missing-chart"`)
}
