package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubEKSDescribe makes Execute describe a fixed cluster without AWS calls.
func stubEKSDescribe(t *testing.T, info *awsCloud.EKSClusterInfo) {
	t.Helper()
	origClientFactory, origDescribe := eksClientFactory, eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})
	eksClientFactory = func(context.Context, types.ICredentials, string) (awsCloud.EKSClient, error) {
		return nil, nil // Unused: DescribeCluster is stubbed too.
	}
	eksDescribeCluster = func(context.Context, awsCloud.EKSClient, string, string) (*awsCloud.EKSClusterInfo, error) {
		return info, nil
	}
}

// userOwnedKubeconfig returns a kubeconfig with entries Atmos did not create.
func userOwnedKubeconfig() *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["minikube"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:8443"}
	cfg.AuthInfos["minikube"] = &clientcmdapi.AuthInfo{Token: "user-token"}
	cfg.Contexts["minikube"] = &clientcmdapi.Context{Cluster: "minikube", AuthInfo: "minikube", Namespace: "apps"}
	cfg.CurrentContext = "minikube"
	return cfg
}

func TestEKSIntegration_ExecuteAndCleanup_PreserveUserEntries(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeKubeconfig(t, kubeconfigPath, userOwnedKubeconfig())

	clusterARN := "arn:aws:eks:eu-west-1:123456789012:cluster/platform"
	stubEKSDescribe(t, &awsCloud.EKSClusterInfo{
		Name:                     "platform",
		Endpoint:                 "https://platform.eks.amazonaws.com",
		CertificateAuthorityData: "dGVzdA==",
		ARN:                      clusterARN,
		Region:                   "eu-west-1",
	})

	integration := &EKSIntegration{
		name:     "platform-eks",
		identity: "platform-admin",
		cluster: &schema.Cluster{
			Name:       "platform",
			Region:     "eu-west-1",
			Alias:      "platform",
			Kubeconfig: &schema.KubeconfigSettings{Path: kubeconfigPath, Update: "merge"},
		},
	}
	workingDir, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, integration.Execute(t.Context(), nil))

	merged, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)

	// User-owned entries survive the merge untouched.
	require.Contains(t, merged.Clusters, "minikube")
	assert.Equal(t, "https://127.0.0.1:8443", merged.Clusters["minikube"].Server)
	require.Contains(t, merged.AuthInfos, "minikube")
	assert.Equal(t, "user-token", merged.AuthInfos["minikube"].Token)
	require.Contains(t, merged.Contexts, "minikube")
	assert.Equal(t, "apps", merged.Contexts["minikube"].Namespace)

	// The Atmos context points at the cluster ARN and an exec user that requests
	// tokens for the right cluster, region, identity, and project directory.
	require.Contains(t, merged.Contexts, "platform")
	assert.Equal(t, "platform", merged.CurrentContext)
	atmosCtx := merged.Contexts["platform"]
	assert.Equal(t, clusterARN, atmosCtx.Cluster)
	require.Contains(t, merged.AuthInfos, atmosCtx.AuthInfo)
	exec := merged.AuthInfos[atmosCtx.AuthInfo].Exec
	require.NotNil(t, exec)
	assert.Equal(t, []string{
		"aws", "eks", "token",
		"--cluster-name", "platform",
		"--region", "eu-west-1",
		"--identity=platform-admin",
	}, exec.Args)
	execEnv := map[string]string{}
	for _, env := range exec.Env {
		execEnv[env.Name] = env.Value
	}
	assert.Equal(t, "platform-admin", execEnv["ATMOS_IDENTITY"])
	assert.Equal(t, workingDir, execEnv["ATMOS_CHDIR"])
	assert.Equal(t, clientcmdapi.NeverExecInteractiveMode, exec.InteractiveMode)

	// Cleanup removes only what Atmos wrote.
	require.NoError(t, integration.Cleanup(t.Context()))

	remaining, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, remaining.Clusters, clusterARN)
	assert.NotContains(t, remaining.Contexts, "platform")
	assert.NotContains(t, remaining.AuthInfos, atmosCtx.AuthInfo)
	assert.Contains(t, remaining.Clusters, "minikube")
	assert.Contains(t, remaining.AuthInfos, "minikube")
	assert.Contains(t, remaining.Contexts, "minikube")
	// The current context pointed at the removed Atmos context, so it is cleared.
	assert.Empty(t, remaining.CurrentContext)
}

func TestEKSIntegration_FindClusterARN_RequiresWholeClusterName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		existing    []string
		wantARN     string
		wantErr     bool
	}{
		{
			name:        "exact cluster name matches",
			clusterName: "prod",
			existing:    []string{"arn:aws:eks:us-east-1:123456789012:cluster/prod"},
			wantARN:     "arn:aws:eks:us-east-1:123456789012:cluster/prod",
		},
		{
			name:        "cluster whose name ends with the configured name does not match",
			clusterName: "prod",
			existing:    []string{"arn:aws:eks:us-east-1:123456789012:cluster/my-prod"},
			wantErr:     true,
		},
		{
			name:        "cluster whose name starts with the configured name does not match",
			clusterName: "prod",
			existing:    []string{"arn:aws:eks:us-east-1:123456789012:cluster/prod-blue"},
			wantErr:     true,
		},
		{
			name:        "empty kubeconfig has no match",
			clusterName: "prod",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			cfg := clientcmdapi.NewConfig()
			for _, arn := range tt.existing {
				cfg.Clusters[arn] = &clientcmdapi.Cluster{Server: "https://example.eks.amazonaws.com"}
			}
			if len(tt.existing) > 0 {
				writeKubeconfig(t, kubeconfigPath, cfg)
			}

			mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
			require.NoError(t, err)
			integration := &EKSIntegration{cluster: &schema.Cluster{Name: tt.clusterName, Region: "us-east-1"}}

			arn, err := integration.findClusterARN(mgr)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, arn)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantARN, arn)
		})
	}
}
