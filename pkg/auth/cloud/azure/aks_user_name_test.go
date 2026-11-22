package azure

// Tests for the AKS kubeconfig exec-user names shared by the writer
// (BuildKubeClusterInfo) and AKS integration cleanup.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
)

// TestAKSKubeUserName_MatchesWrittenUser verifies that cleanup computes exactly
// the user name the kubeconfig writer used, for every identity and subscription shape.
func TestAKSKubeUserName_MatchesWrittenUser(t *testing.T) {
	tests := []struct {
		name           string
		subscriptionID string
		identity       string
		want           string
	}{
		{name: "subscription and identity", subscriptionID: "sub-123", identity: "azure-admin", want: "atmos-aks-dev-cluster-dev-rg-sub-123-azure-admin"},
		{name: "no identity", subscriptionID: "sub-123", want: "atmos-aks-dev-cluster-dev-rg-sub-123"},
		{name: "no subscription keeps the identity", identity: "azure-admin", want: "atmos-aks-dev-cluster-dev-rg-azure-admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			written := BuildKubeClusterInfo(&AKSClusterInfo{Name: "dev-cluster", ResourceGroup: "dev-rg", SubscriptionID: tt.subscriptionID}, tt.identity)

			assert.Equal(t, tt.identity, written.Identity)
			assert.Equal(t, tt.want, kube.UserName(written))
			assert.Equal(t, kube.UserName(written), AKSKubeUserName("dev-cluster", "dev-rg", tt.subscriptionID, tt.identity, ""))
		})
	}
}

// TestLegacyAKSKubeUserName verifies the name versions before the identity was added wrote.
func TestLegacyAKSKubeUserName(t *testing.T) {
	assert.Equal(t, "atmos-aks-dev-cluster-dev-rg-sub-123", LegacyAKSKubeUserName("dev-cluster", "dev-rg", "sub-123"))
}

// TestAKSKubeUserNames verifies the cleanup candidates: the current name first,
// then the pre-realm name (only with a realm) and the legacy name.
func TestAKSKubeUserNames(t *testing.T) {
	assert.Equal(t, []string{
		"atmos-acme-aks-dev-cluster-dev-rg-sub-123-azure-admin",
		"atmos-aks-dev-cluster-dev-rg-sub-123-azure-admin",
		"atmos-aks-dev-cluster-dev-rg-sub-123",
	}, AKSKubeUserNames("dev-cluster", "dev-rg", "sub-123", "azure-admin", "acme"))
	assert.Equal(t, []string{
		"atmos-aks-dev-cluster-dev-rg-sub-123-azure-admin",
		"atmos-aks-dev-cluster-dev-rg-sub-123",
	}, AKSKubeUserNames("dev-cluster", "dev-rg", "sub-123", "azure-admin", ""))
}

// TestBuildKubeClusterInfo_OwnedUserNamesMatchCleanup verifies that the users
// the writer treats as its own when migrating an unprefixed context are the
// same names cleanup removes, with and without a realm.
func TestBuildKubeClusterInfo_OwnedUserNamesMatchCleanup(t *testing.T) {
	for _, realm := range []string{"", "acme"} {
		t.Run("realm="+realm, func(t *testing.T) {
			written := BuildKubeClusterInfo(&AKSClusterInfo{Name: "dev-cluster", ResourceGroup: "dev-rg", SubscriptionID: "sub-123"}, "azure-admin")
			written.Realm = realm

			assert.Equal(t, []string{LegacyAKSKubeUserName("dev-cluster", "dev-rg", "sub-123")}, written.LegacyUserNames)
			assert.Equal(t, AKSKubeUserNames("dev-cluster", "dev-rg", "sub-123", "azure-admin", realm), kube.OwnedUserNames(written))
		})
	}
}
