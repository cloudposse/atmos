package aws

// Tests that EKS kubeconfig exec users are scoped to the auth realm, so two
// repositories (or worktrees) with different realms never share, and
// overwrite, one kube user in the shared kubeconfig (issue #3720, #3675).

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	realmRepoA = "repo-a"
	realmRepoB = "repo-b"
)

// execEnvValue returns the value of the named exec-plugin env var of user, or "".
func execEnvValue(t *testing.T, user *clientcmdapi.AuthInfo, name string) string {
	t.Helper()

	require.NotNil(t, user)
	require.NotNil(t, user.Exec)
	for _, env := range user.Exec.Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

// stubProdEKSCluster makes Execute describe the prod cluster without AWS calls.
func stubProdEKSCluster(t *testing.T) {
	t.Helper()

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
}

// newRealmEKSIntegration builds an EKS integration through the factory, as the
// auth manager does, with the given realm.
func newRealmEKSIntegration(t *testing.T, path, alias, realm string) integrations.Integration {
	t.Helper()

	integration, err := NewEKSIntegration(&integrations.IntegrationConfig{
		Name:  "prod",
		Realm: realm,
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Via:  &schema.IntegrationVia{Identity: "admin"},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name: "prod", Region: "us-east-2", Alias: alias,
				Kubeconfig: &schema.KubeconfigSettings{Path: path},
			}},
		},
	})
	require.NoError(t, err)
	return integration
}

// resolvedDir returns dir with symlinks resolved, so macOS /var and /private/var compare equal.
func resolvedDir(t *testing.T, dir string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return resolved
}

// TestEKSIntegration_Execute_DistinctUserPerRealm runs the same integration
// (same cluster and identity) from two checkouts with different realms and
// verifies that each context keeps its own exec user, working directory and
// realm instead of the last writer's.
func TestEKSIntegration_Execute_DistinctUserPerRealm(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	repoA, repoB := t.TempDir(), t.TempDir()

	t.Chdir(repoA)
	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "prod-a", realmRepoA).Execute(t.Context(), nil))
	t.Chdir(repoB)
	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "prod-b", realmRepoB).Execute(t.Context(), nil))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	userA := config.Contexts["prod-a"].AuthInfo
	userB := config.Contexts["prod-b"].AuthInfo
	require.NotEqual(t, userA, userB, "realms must not share a kube user")
	assert.Equal(t, "atmos-repo-a-eks-prod-us-east-2-111111111111-admin-70ac3e65", userA)
	assert.Equal(t, "atmos-repo-b-eks-prod-us-east-2-111111111111-admin-299cbf1c", userB)

	assert.Equal(t, resolvedDir(t, repoA), resolvedDir(t, execEnvValue(t, config.AuthInfos[userA], "ATMOS_CHDIR")))
	assert.Equal(t, resolvedDir(t, repoB), resolvedDir(t, execEnvValue(t, config.AuthInfos[userB], "ATMOS_CHDIR")))
	assert.Equal(t, realmRepoA, execEnvValue(t, config.AuthInfos[userA], "ATMOS_AUTH_REALM"))
	assert.Equal(t, realmRepoB, execEnvValue(t, config.AuthInfos[userB], "ATMOS_AUTH_REALM"))
}

// TestEKSIntegration_Execute_EmptyRealmKeepsUserName verifies that without a
// realm the user name and exec env stay as before.
func TestEKSIntegration_Execute_EmptyRealmKeepsUserName(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "prod", "").Execute(t.Context(), nil))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	user := config.Contexts["prod"].AuthInfo
	assert.Equal(t, "atmos-eks-prod-us-east-2-111111111111-admin-48b9c2fe", user)
	assert.Empty(t, execEnvValue(t, config.AuthInfos[user], "ATMOS_AUTH_REALM"))
}

// TestEKSIntegration_Cleanup_KeepsOtherRealmUser verifies that cleaning up one
// realm's integration leaves the other realm's context and user in place.
func TestEKSIntegration_Cleanup_KeepsOtherRealmUser(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	integrationA := newRealmEKSIntegration(t, kubeconfigPath, "prod-a", realmRepoA)
	require.NoError(t, integrationA.Execute(t.Context(), nil))
	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "prod-b", realmRepoB).Execute(t.Context(), nil))

	require.NoError(t, integrationA.Cleanup(t.Context()))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "prod-a")
	assert.NotContains(t, config.AuthInfos, "atmos-repo-a-eks-prod-us-east-2-111111111111-admin-70ac3e65")
	assert.Contains(t, config.Contexts, "prod-b")
	assert.Contains(t, config.AuthInfos, "atmos-repo-b-eks-prod-us-east-2-111111111111-admin-299cbf1c")
	assert.Contains(t, config.Clusters, prodClusterARN, "the cluster is still used by the prod-b context")
}

// TestEKSIntegration_Cleanup_RemovesPreRealmUser verifies that a realm's
// cleanup still removes the user written before realms were part of the name
// (atmos-eks-<name>-<region>-<account>-<identity>).
func TestEKSIntegration_Cleanup_RemovesPreRealmUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", prodAdminContextName, "admin")
	// An unrelated cluster keeps the file from being deleted, so the result can be inspected.
	writeEKSEntry(t, kubeconfigPath, "arn:aws:eks:eu-west-1:111111111111:cluster/other", "other", "eu-west-1", "other", "admin")

	before, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	preRealmUser := "atmos-eks-prod-us-east-2-111111111111-admin-48b9c2fe"
	require.Equal(t, preRealmUser, before.Contexts[prodAdminContextName].AuthInfo)

	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, prodAdminContextName, realmRepoA).Cleanup(t.Context()))

	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, after.Contexts, prodAdminContextName)
	assert.NotContains(t, after.AuthInfos, preRealmUser)
	assert.NotContains(t, after.Clusters, prodClusterARN)
	assert.Contains(t, after.Contexts, "other")
}

// TestEKSIntegration_Cleanup_PicksOwnAccountByPreRealmUser verifies that, with
// same-named clusters of two accounts written before realms were part of the
// name, a realm's cleanup still attributes the cluster by its pre-realm user
// and removes only that one.
func TestEKSIntegration_Cleanup_PicksOwnAccountByPreRealmUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", "", "prod-admin")
	writeEKSEntry(t, kubeconfigPath, prodClusterARNOther, "prod", "us-east-2", "", "other-admin")

	integration := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", "", "other-admin")
	integration.realm = realmRepoA
	require.NoError(t, integration.Cleanup(t.Context()))

	after, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, after.Clusters, prodClusterARNOther)
	assert.NotContains(t, after.Contexts, prodClusterARNOther)
	assert.Contains(t, after.Clusters, prodClusterARN)
	assert.Equal(t, "prod-admin", contextIdentity(t, after, prodClusterARN))
}
