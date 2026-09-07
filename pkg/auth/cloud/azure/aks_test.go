package azure

import (
	"context"
	"testing"
	"time"

	armcontainerservice "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// buildExecFormatKubeconfig builds the raw bytes of an exec-format AKS
// credential kubeconfig, mirroring what ListClusterUserCredentials(Format:
// exec) returns for an AAD-enabled cluster: one cluster entry, one user
// entry whose exec plugin is kubelogin with --server-id/--tenant-id args.
func buildExecFormatKubeconfig(t *testing.T, serverID, tenantID string) []byte {
	t.Helper()

	config := clientcmdapi.NewConfig()
	config.Clusters["mycluster"] = &clientcmdapi.Cluster{
		Server:                   "https://mycluster-dns-12345.hcp.eastus.azmk8s.io:443",
		CertificateAuthorityData: []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n"),
	}
	config.AuthInfos["clusterUser_myRG_mycluster"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Command:    "kubelogin",
			Args: []string{
				"get-token",
				"--environment", "AzurePublicCloud",
				"--server-id", serverID,
				"--client-id", "04b07795-8ddb-461a-bbee-02f9e1bf7b46",
				"--tenant-id", tenantID,
				"--login", "devicecode",
			},
		},
	}
	config.Contexts["mycluster"] = &clientcmdapi.Context{
		Cluster:  "mycluster",
		AuthInfo: "clusterUser_myRG_mycluster",
	}
	config.CurrentContext = "mycluster"

	data, err := clientcmd.Write(*config)
	require.NoError(t, err)
	return data
}

// buildLocalAccountKubeconfig builds a non-AAD (local account) kubeconfig,
// as ListClusterUserCredentials(Format: exec) returns when the cluster isn't
// AAD-enabled: no exec block on the auth-info entry.
func buildLocalAccountKubeconfig(t *testing.T) []byte {
	t.Helper()

	config := clientcmdapi.NewConfig()
	config.Clusters["mycluster"] = &clientcmdapi.Cluster{
		Server:                   "https://mycluster-dns-12345.hcp.eastus.azmk8s.io:443",
		CertificateAuthorityData: []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n"),
	}
	config.AuthInfos["clusterUser_myRG_mycluster"] = &clientcmdapi.AuthInfo{
		Token: "some-static-token",
	}
	config.Contexts["mycluster"] = &clientcmdapi.Context{
		Cluster:  "mycluster",
		AuthInfo: "clusterUser_myRG_mycluster",
	}
	config.CurrentContext = "mycluster"

	data, err := clientcmd.Write(*config)
	require.NoError(t, err)
	return data
}

func TestNewAKSClient_InvalidCredentials(t *testing.T) {
	_, err := NewAKSClient(t.Context(), nil, "sub-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSDescribeCluster)
}

func TestNewAKSClient_NoSubscriptionID(t *testing.T) {
	creds := &types.AzureCredentials{AccessToken: "token"}
	_, err := NewAKSClient(t.Context(), creds, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSDescribeCluster)
}

func TestNewAKSClient_Success(t *testing.T) {
	creds := &types.AzureCredentials{AccessToken: "token", SubscriptionID: "sub-from-creds"}
	client, err := NewAKSClient(t.Context(), creds, "")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestDescribeCluster_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	resourceID := "/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.ContainerService/managedClusters/mycluster"
	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{
			ManagedCluster: armcontainerservice.ManagedCluster{ID: &resourceID},
		}, nil,
	)

	kubeconfigBytes := buildExecFormatKubeconfig(t, "aad-server-id-123", "tenant-abc")
	client.EXPECT().ListClusterUserCredentials(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientListClusterUserCredentialsResponse{
			CredentialResults: armcontainerservice.CredentialResults{
				Kubeconfigs: []*armcontainerservice.CredentialResult{{Value: kubeconfigBytes}},
			},
		}, nil,
	)

	info, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.NoError(t, err)
	assert.Equal(t, "mycluster", info.Name)
	assert.Equal(t, "myRG", info.ResourceGroup)
	assert.Equal(t, "sub-123", info.SubscriptionID)
	assert.Equal(t, resourceID, info.ID)
	assert.Equal(t, "https://mycluster-dns-12345.hcp.eastus.azmk8s.io:443", info.Endpoint)
	assert.Equal(t, "aad-server-id-123", info.ServerID)
	assert.Equal(t, "tenant-abc", info.TenantID)
	assert.NotEmpty(t, info.CertificateAuthorityData)
}

func TestDescribeCluster_GetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{}, assert.AnError,
	)

	_, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSDescribeCluster)
}

func TestDescribeCluster_NilID(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{}, nil,
	)

	_, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSClusterNotFound)
}

func TestDescribeCluster_ListCredentialsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	resourceID := "/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.ContainerService/managedClusters/mycluster"
	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{
			ManagedCluster: armcontainerservice.ManagedCluster{ID: &resourceID},
		}, nil,
	)
	client.EXPECT().ListClusterUserCredentials(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientListClusterUserCredentialsResponse{}, assert.AnError,
	)

	_, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSDescribeCluster)
}

func TestDescribeCluster_NoKubeconfigsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	resourceID := "/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.ContainerService/managedClusters/mycluster"
	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{
			ManagedCluster: armcontainerservice.ManagedCluster{ID: &resourceID},
		}, nil,
	)
	client.EXPECT().ListClusterUserCredentials(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientListClusterUserCredentialsResponse{}, nil,
	)

	_, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSDescribeCluster)
}

func TestDescribeCluster_MalformedKubeconfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	resourceID := "/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.ContainerService/managedClusters/mycluster"
	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{
			ManagedCluster: armcontainerservice.ManagedCluster{ID: &resourceID},
		}, nil,
	)
	client.EXPECT().ListClusterUserCredentials(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientListClusterUserCredentialsResponse{
			CredentialResults: armcontainerservice.CredentialResults{
				Kubeconfigs: []*armcontainerservice.CredentialResult{{Value: []byte("not: valid: yaml: [")}},
			},
		}, nil,
	)

	_, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSDescribeCluster)
}

func TestDescribeCluster_NotAADEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockAKSClient(ctrl)

	resourceID := "/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.ContainerService/managedClusters/mycluster"
	client.EXPECT().Get(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientGetResponse{
			ManagedCluster: armcontainerservice.ManagedCluster{ID: &resourceID},
		}, nil,
	)

	kubeconfigBytes := buildLocalAccountKubeconfig(t)
	client.EXPECT().ListClusterUserCredentials(gomock.Any(), "myRG", "mycluster", gomock.Any()).Return(
		armcontainerservice.ManagedClustersClientListClusterUserCredentialsResponse{
			CredentialResults: armcontainerservice.CredentialResults{
				Kubeconfigs: []*armcontainerservice.CredentialResult{{Value: kubeconfigBytes}},
			},
		}, nil,
	)

	_, err := DescribeCluster(t.Context(), client, "sub-123", "myRG", "mycluster")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSClusterNotAADEnabled)
}

func TestParseKubeloginExecArgs(t *testing.T) {
	args := []string{
		"get-token",
		"--environment", "AzurePublicCloud",
		"--server-id", "server-123",
		"--tenant-id", "tenant-456",
		"--login", "devicecode",
	}

	serverID, tenantID := parseKubeloginExecArgs(args)
	assert.Equal(t, "server-123", serverID)
	assert.Equal(t, "tenant-456", tenantID)
}

func TestParseKubeloginExecArgs_Missing(t *testing.T) {
	serverID, tenantID := parseKubeloginExecArgs([]string{"get-token", "--login", "devicecode"})
	assert.Empty(t, serverID)
	assert.Empty(t, tenantID)
}

func TestParseKubeloginExecArgs_TrailingFlagNoValue(t *testing.T) {
	serverID, tenantID := parseKubeloginExecArgs([]string{"get-token", "--server-id"})
	assert.Empty(t, serverID)
	assert.Empty(t, tenantID)
}

func TestBuildKubeClusterInfo_WithIdentity(t *testing.T) {
	info := &AKSClusterInfo{
		Name:                     "mycluster",
		ResourceGroup:            "myRG",
		SubscriptionID:           "sub-123",
		ID:                       "/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.ContainerService/managedClusters/mycluster",
		Endpoint:                 "https://mycluster.hcp.eastus.azmk8s.io:443",
		CertificateAuthorityData: "dGVzdA==",
		ServerID:                 "custom-server-id",
	}

	clusterInfo := BuildKubeClusterInfo(info, "azure-dev")

	assert.Equal(t, "mycluster", clusterInfo.Name)
	assert.Equal(t, "https://mycluster.hcp.eastus.azmk8s.io:443", clusterInfo.Endpoint)
	assert.Equal(t, "dGVzdA==", clusterInfo.CertificateAuthorityData)
	assert.Equal(t, info.ID, clusterInfo.ID)
	assert.Equal(t, "myRG", clusterInfo.Region)
	assert.Equal(t, "aks", clusterInfo.UserPrefix)
	assert.Equal(t, []string{"azure", "aks", "token", "--cluster-name", "mycluster", "--resource-group", "myRG", "--server-id", "custom-server-id", "--subscription-id", "sub-123", "--identity=azure-dev"}, clusterInfo.ExecArgs)
	require.Len(t, clusterInfo.ExecEnv, 1)
	assert.Equal(t, "ATMOS_IDENTITY", clusterInfo.ExecEnv[0].Name)
	assert.Equal(t, "azure-dev", clusterInfo.ExecEnv[0].Value)
}

func TestAKSServerScopeFromContext(t *testing.T) {
	assert.Equal(t, AKSServerScope, AKSServerScopeFromContext(context.Background()))

	ctx := ContextWithAKSServerID(context.Background(), "custom-server-id")
	assert.Equal(t, "custom-server-id/.default", AKSServerScopeFromContext(ctx))
}

func TestBuildKubeClusterInfo_WithoutIdentity(t *testing.T) {
	info := &AKSClusterInfo{
		Name:          "mycluster",
		ResourceGroup: "myRG",
	}

	clusterInfo := BuildKubeClusterInfo(info, "")

	assert.Equal(t, []string{"azure", "aks", "token", "--cluster-name", "mycluster", "--resource-group", "myRG"}, clusterInfo.ExecArgs)
	assert.Empty(t, clusterInfo.ExecEnv)
}

func TestGetToken_Success(t *testing.T) {
	expiresAt := time.Now().Add(1 * time.Hour)
	creds := &types.AzureCredentials{
		AKSToken:           "aks-jwt-token",
		AKSTokenExpiration: expiresAt.Format(time.RFC3339),
	}

	token, expires, err := GetToken(creds)
	require.NoError(t, err)
	assert.Equal(t, "aks-jwt-token", token)
	assert.WithinDuration(t, expiresAt, expires, time.Second)
}

func TestGetToken_NoTokenAvailable(t *testing.T) {
	creds := &types.AzureCredentials{}

	_, _, err := GetToken(creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
}

func TestGetToken_InvalidCredentialType(t *testing.T) {
	_, _, err := GetToken(&types.AWSCredentials{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
}

func TestGetToken_InvalidExpiration(t *testing.T) {
	creds := &types.AzureCredentials{
		AKSToken:           "aks-jwt-token",
		AKSTokenExpiration: "not-a-timestamp",
	}

	_, _, err := GetToken(creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
}

func TestGetToken_NoExpiration(t *testing.T) {
	creds := &types.AzureCredentials{AKSToken: "aks-jwt-token"}

	token, expires, err := GetToken(creds)
	require.NoError(t, err)
	assert.Equal(t, "aks-jwt-token", token)
	assert.True(t, expires.IsZero())
}
