package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	gcpIntegration "github.com/cloudposse/atmos/pkg/auth/integrations/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

type fakeManagerGKEIntegration struct {
	config     *integrations.IntegrationConfig
	executions *int
}

func (*fakeManagerGKEIntegration) Kind() string { return integrations.KindGCPGKE }

func (f *fakeManagerGKEIntegration) Execute(_ context.Context, creds types.ICredentials) error {
	*f.executions++
	cluster := f.config.Config.Spec.Cluster
	mgr, err := kube.NewKubeconfigManager(cluster.Kubeconfig.Path, cluster.Kubeconfig.Mode)
	if err != nil {
		return err
	}
	info := &gcpCloud.GKEClusterInfo{
		Name:                     cluster.Name,
		ProjectID:                cluster.ProjectID,
		Location:                 cluster.Location,
		ID:                       gcpCloud.ClusterResourceName(cluster.ProjectID, cluster.Location, cluster.Name),
		Endpoint:                 "https://gke.example.invalid",
		CertificateAuthorityData: "dGVzdC1jYQ==",
	}
	identity := f.config.Config.Via.Identity
	_, err = mgr.WriteClusterConfig(gcpCloud.BuildKubeClusterInfo(info, identity), cluster.Alias, cluster.Kubeconfig.Update)
	return err
}

func (*fakeManagerGKEIntegration) Cleanup(context.Context) error { return nil }

func (f *fakeManagerGKEIntegration) Environment() (map[string]string, error) {
	path := f.config.Config.Spec.Cluster.Kubeconfig.Path
	return map[string]string{"KUBECONFIG": path, "KUBE_CONFIG_PATH": path}, nil
}

func TestEnsureIdentityEnvironmentProvisionsGKEAndReturnsKubeconfig(t *testing.T) {
	resetProcessIntegrationCache()
	t.Cleanup(resetProcessIntegrationCache)

	path := filepath.Join(t.TempDir(), "example-kubeconfig")
	executions := 0
	integrations.Register(integrations.KindGCPGKE, func(config *integrations.IntegrationConfig) (integrations.Integration, error) {
		return &fakeManagerGKEIntegration{config: config, executions: &executions}, nil
	})
	t.Cleanup(func() { integrations.Register(integrations.KindGCPGKE, gcpIntegration.NewGKEIntegration) })

	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"example-deployer": {Kind: "gcp/service-account"},
		},
		Integrations: map[string]schema.Integration{
			"example-gke": {
				Kind: integrations.KindGCPGKE,
				Via:  &schema.IntegrationVia{Identity: "example-deployer"},
				Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
					Name:      "example-cluster",
					ProjectID: "example-project",
					Location:  "us-central1",
					Alias:     "example",
					Kubeconfig: &schema.KubeconfigSettings{
						Path:   path,
						Update: "replace",
					},
				}},
			},
		},
	}
	creds := &types.GCPCredentials{AccessToken: "example-access-token", TokenExpiry: time.Now().Add(time.Hour)}
	m := &manager{
		config: authConfig,
		identities: map[string]types.Identity{
			"example-deployer": &stubEnvIdentity{},
		},
		credentialStore: &testStore{data: map[string]any{"example-deployer": creds}},
	}

	env, err := m.EnsureIdentityEnvironment(t.Context(), "example-deployer")
	require.NoError(t, err)
	assert.Equal(t, 1, executions)
	assert.Equal(t, path, env["KUBECONFIG"])
	assert.Equal(t, path, env["KUBE_CONFIG_PATH"])

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "example", config.CurrentContext)
	assert.Contains(t, config.Clusters, "projects/example-project/locations/us-central1/clusters/example-cluster")
	assert.Empty(t, config.AuthInfos["atmos-gke-example-cluster-example-project-us-central1"].Token)

	// The process-level canonical target cache suppresses duplicate provisioning.
	_, err = m.EnsureIdentityEnvironment(t.Context(), "example-deployer")
	require.NoError(t, err)
	assert.Equal(t, 1, executions)
}
