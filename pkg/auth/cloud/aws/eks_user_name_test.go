package aws

// Tests for the EKS kubeconfig exec-user names shared by the writer
// (BuildKubeClusterInfo) and EKS integration cleanup.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
)

// TestEKSKubeUserName_MatchesWrittenUser verifies that cleanup computes exactly
// the user name the kubeconfig writer used, for every identity and account shape.
func TestEKSKubeUserName_MatchesWrittenUser(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		identity string
		want     string
	}{
		{
			name:     "account and identity",
			arn:      "arn:aws:eks:us-east-2:111111111111:cluster/prod",
			identity: "prod-admin",
			want:     "atmos-eks-prod-us-east-2-111111111111-prod-admin",
		},
		{
			name: "no identity",
			arn:  "arn:aws:eks:us-east-2:111111111111:cluster/prod",
			want: "atmos-eks-prod-us-east-2-111111111111",
		},
		{
			name:     "ARN that cannot be parsed keeps the identity",
			arn:      "not-an-arn",
			identity: "prod-admin",
			want:     "atmos-eks-prod-us-east-2-prod-admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			written, err := BuildKubeClusterInfo(&EKSClusterInfo{Name: "prod", Region: "us-east-2", ARN: tt.arn}, tt.identity)
			require.NoError(t, err)

			assert.Equal(t, tt.want, kube.UserName(written))
			assert.Equal(t, kube.UserName(written), EKSKubeUserName(tt.arn, "prod", "us-east-2", tt.identity, ""))
		})
	}
}

// TestLegacyEKSKubeUserName verifies the name versions before the account ID
// and identity were added wrote.
func TestLegacyEKSKubeUserName(t *testing.T) {
	assert.Equal(t, "atmos-eks-prod-us-east-2", LegacyEKSKubeUserName("prod", "us-east-2"))
}

// TestEKSKubeUserNames verifies the cleanup candidates: the current name first,
// then the pre-realm name (only with a realm) and the legacy name.
func TestEKSKubeUserNames(t *testing.T) {
	const arn = "arn:aws:eks:us-east-2:111111111111:cluster/prod"

	assert.Equal(t, []string{
		"atmos-acme-eks-prod-us-east-2-111111111111-admin",
		"atmos-eks-prod-us-east-2-111111111111-admin",
		"atmos-eks-prod-us-east-2",
	}, EKSKubeUserNames(arn, "prod", "us-east-2", "admin", kube.RealmScope{Realm: "acme"}))
	assert.Equal(t, []string{
		"atmos-eks-prod-us-east-2-111111111111-admin",
		"atmos-eks-prod-us-east-2",
	}, EKSKubeUserNames(arn, "prod", "us-east-2", "admin", kube.RealmScope{Realm: ""}))
}

// TestBuildKubeClusterInfo_OwnedUserNamesMatchCleanup verifies that the users
// the writer treats as its own when migrating an unprefixed context are the
// same names cleanup removes, with and without a realm.
func TestBuildKubeClusterInfo_OwnedUserNamesMatchCleanup(t *testing.T) {
	const clusterARN = "arn:aws:eks:us-east-2:111111111111:cluster/prod"
	for _, realm := range []string{"", "acme"} {
		t.Run("realm="+realm, func(t *testing.T) {
			written, err := BuildKubeClusterInfo(&EKSClusterInfo{Name: "prod", Region: "us-east-2", ARN: clusterARN}, "prod-admin")
			require.NoError(t, err)
			written.Realm = realm

			assert.Equal(t, []string{LegacyEKSKubeUserName("prod", "us-east-2")}, written.LegacyUserNames)
			assert.Equal(t, EKSKubeUserNames(clusterARN, "prod", "us-east-2", "prod-admin", kube.RealmScope{Realm: realm}), kube.OwnedUserNames(written))
		})
	}
}

// TestEKSKubeUserNames_LegacyRealms verifies that names an unreleased build
// wrote under a derived realm are cleanup candidates, in the same order as
// kube.OwnedUserNames.
func TestEKSKubeUserNames_LegacyRealms(t *testing.T) {
	const clusterARN = "arn:aws:eks:us-east-2:111111111111:cluster/prod"
	const derived = "462813558259d41c"

	assert.Equal(t, []string{
		"atmos-eks-prod-us-east-2-111111111111-admin",
		"atmos-" + derived + "-eks-prod-us-east-2-111111111111-admin",
		"atmos-eks-prod-us-east-2",
	}, EKSKubeUserNames(clusterARN, "prod", "us-east-2", "admin", kube.RealmScope{LegacyRealms: []string{derived}}))

	written, err := BuildKubeClusterInfo(&EKSClusterInfo{Name: "prod", Region: "us-east-2", ARN: clusterARN}, "admin")
	require.NoError(t, err)
	written.Realm, written.LegacyRealms = kube.NameRealm(derived, false)
	assert.Equal(t, EKSKubeUserNames(clusterARN, "prod", "us-east-2", "admin", kube.RealmScope{Realm: written.Realm, LegacyRealms: written.LegacyRealms}), kube.OwnedUserNames(written))
}
