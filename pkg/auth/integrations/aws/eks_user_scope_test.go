package aws

// Tests that EKS kubeconfig contexts written for different identities or
// accounts get their own exec user, and that Cleanup removes only its own
// entries, including users written by older versions (issue #3720, finding 2.5).

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	prodClusterARN       = "arn:aws:eks:us-east-2:111111111111:cluster/prod"
	prodClusterARNOther  = "arn:aws:eks:us-east-2:222222222222:cluster/prod"
	legacyProdUserName   = "atmos-eks-prod-us-east-2"
	prodAdminContextName = "prod-admin"
	prodReadContextName  = "prod-readonly"
)

// contextIdentity returns the --identity argument of the exec user behind contextName.
func contextIdentity(t *testing.T, config *clientcmdapi.Config, contextName string) string {
	t.Helper()

	kubeContext, ok := config.Contexts[contextName]
	require.True(t, ok, "context %s must exist", contextName)
	user, ok := config.AuthInfos[kubeContext.AuthInfo]
	require.True(t, ok, "user %s of context %s must exist", kubeContext.AuthInfo, contextName)
	require.NotNil(t, user.Exec)
	for _, arg := range user.Exec.Args {
		if len(arg) > len("--identity=") && arg[:len("--identity=")] == "--identity=" {
			return arg[len("--identity="):]
		}
	}
	return ""
}

// TestEKSIntegration_Execute_DistinctUserPerIdentity runs two integrations for
// the same cluster with different identities through Execute and verifies that
// each context keeps running as its own identity.
func TestEKSIntegration_Execute_DistinctUserPerIdentity(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	origFactory, origDescribe := eksClientFactory, eksDescribeCluster
	t.Cleanup(func() { eksClientFactory, eksDescribeCluster = origFactory, origDescribe })
	eksClientFactory = func(context.Context, types.ICredentials, string) (awsCloud.EKSClient, error) {
		return nil, nil
	}
	eksDescribeCluster = func(context.Context, awsCloud.EKSClient, string, string) (*awsCloud.EKSClusterInfo, error) {
		return &awsCloud.EKSClusterInfo{
			Name: "prod", Endpoint: "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==", ARN: prodClusterARN, Region: "us-east-2",
		}, nil
	}

	for _, integration := range []*EKSIntegration{
		newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", prodAdminContextName, "prod-admin"),
		newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", prodReadContextName, "prod-readonly"),
	} {
		require.NoError(t, integration.Execute(t.Context(), nil))
	}

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotEqual(t, config.Contexts[prodAdminContextName].AuthInfo, config.Contexts[prodReadContextName].AuthInfo,
		"contexts for different identities must not share an exec user")
	assert.Equal(t, "prod-admin", contextIdentity(t, config, prodAdminContextName))
	assert.Equal(t, "prod-readonly", contextIdentity(t, config, prodReadContextName))
}

// TestEKSIntegration_DistinctUserPerAccount verifies that same-named clusters in
// the same region of different accounts get distinct exec users.
func TestEKSIntegration_DistinctUserPerAccount(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", "", "admin")
	writeEKSEntry(t, kubeconfigPath, prodClusterARNOther, "prod", "us-east-2", "", "admin")

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	first := config.Contexts[prodClusterARN].AuthInfo
	second := config.Contexts[prodClusterARNOther].AuthInfo
	assert.NotEqual(t, first, second, "clusters in different accounts must not share an exec user")
	assert.Contains(t, first, "111111111111")
	assert.Contains(t, second, "222222222222")
}

// TestEKSIntegration_Cleanup_KeepsOtherIdentityContext verifies that cleaning up
// one identity's integration leaves the other identity's context, its user and
// the shared cluster entry in place.
func TestEKSIntegration_Cleanup_KeepsOtherIdentityContext(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", prodAdminContextName, "prod-admin")
	writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", prodReadContextName, "prod-readonly")

	before, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	readonlyUser := before.Contexts[prodReadContextName].AuthInfo

	err = newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", prodReadContextName, "prod-readonly").Cleanup(t.Context())
	require.NoError(t, err)

	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, after.Contexts, prodReadContextName)
	assert.NotContains(t, after.AuthInfos, readonlyUser)
	assert.Contains(t, after.Clusters, prodClusterARN, "the cluster is still used by the prod-admin context")
	assert.Equal(t, "prod-admin", contextIdentity(t, after, prodAdminContextName))
}

// writeLegacyEKSEntry writes an entry in the format older versions used: the
// exec user is named atmos-eks-<name>-<region>, without account or identity.
func writeLegacyEKSEntry(t *testing.T, path, clusterARN, alias, identity string) {
	t.Helper()

	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
		Name: "prod", Endpoint: "https://example.eks.amazonaws.com", ID: clusterARN,
		Region: "us-east-2", UserPrefix: "eks",
		ExecArgs: []string{"aws", "eks", "token", "--cluster-name", "prod", "--region", "us-east-2", "--identity=" + identity},
	}, alias, "merge")
	require.NoError(t, err)

	config, err := loadKubeconfig(path)
	require.NoError(t, err)
	require.Contains(t, config.AuthInfos, legacyProdUserName, "the fixture must use the legacy user name")
}

// TestEKSIntegration_Cleanup_RemovesLegacyUser verifies that Cleanup still
// removes an entry written by an older version with the legacy user name.
func TestEKSIntegration_Cleanup_RemovesLegacyUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyEKSEntry(t, kubeconfigPath, prodClusterARN, prodAdminContextName, "prod-admin")
	// An unrelated cluster keeps the file from being deleted, so the result can be inspected.
	writeEKSEntry(t, kubeconfigPath, "arn:aws:eks:eu-west-1:111111111111:cluster/other", "other", "eu-west-1", "other", "prod-admin")

	err := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", prodAdminContextName, "prod-admin").Cleanup(t.Context())
	require.NoError(t, err)

	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, after.Contexts, prodAdminContextName)
	assert.NotContains(t, after.Clusters, prodClusterARN)
	assert.NotContains(t, after.AuthInfos, legacyProdUserName)
	assert.Contains(t, after.Contexts, "other")
}

// TestEKSIntegration_Cleanup_KeepsLegacyUserStillInUse verifies that a legacy
// user shared with another context is not removed with this integration's context.
func TestEKSIntegration_Cleanup_KeepsLegacyUserStillInUse(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyEKSEntry(t, kubeconfigPath, prodClusterARN, prodAdminContextName, "prod-admin")
	writeLegacyEKSEntry(t, kubeconfigPath, prodClusterARN, prodReadContextName, "prod-readonly")

	err := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", prodReadContextName, "prod-readonly").Cleanup(t.Context())
	require.NoError(t, err)

	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, after.Contexts, prodReadContextName)
	assert.Contains(t, after.Contexts, prodAdminContextName)
	assert.Contains(t, after.AuthInfos, legacyProdUserName, "the legacy user is still used by prod-admin")
	assert.Contains(t, after.Clusters, prodClusterARN)
}

// TestEKSIntegration_Cleanup_PicksOwnAccount verifies that without an alias,
// cleanup removes the same-named cluster of the account its own user belongs
// to and keeps the other account's entry.
func TestEKSIntegration_Cleanup_PicksOwnAccount(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", "", "prod-admin")
	writeEKSEntry(t, kubeconfigPath, prodClusterARNOther, "prod", "us-east-2", "", "other-admin")

	err := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", "", "other-admin").Cleanup(t.Context())
	require.NoError(t, err)

	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, after.Clusters, prodClusterARNOther)
	assert.NotContains(t, after.Contexts, prodClusterARNOther)
	assert.Contains(t, after.Clusters, prodClusterARN)
	assert.Equal(t, "prod-admin", contextIdentity(t, after, prodClusterARN))
}

// TestEKSIntegration_FindClusterARN_AmbiguousAccounts verifies that when
// same-named clusters of two accounts both use this integration's user name,
// cleanup refuses to guess and removes neither.
func TestEKSIntegration_FindClusterARN_AmbiguousAccounts(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	// Older entries without account or identity in the user name match no candidate.
	writeLegacyEKSEntry(t, kubeconfigPath, prodClusterARN, "", "admin")
	writeLegacyEKSEntry(t, kubeconfigPath, prodClusterARNOther, "", "admin")

	integration := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", "", "admin")
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)
	_, err = integration.findClusterARN(mgr)
	require.ErrorIs(t, err, errUtils.ErrEKSClusterAmbiguous)

	require.NoError(t, integration.Cleanup(t.Context()))
	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.Contains(t, after.Clusters, prodClusterARN)
	assert.Contains(t, after.Clusters, prodClusterARNOther)
}

// Compile-time guard for the schema fields these tests rely on.
var _ = schema.Cluster{Name: "", Region: "", Alias: ""}
