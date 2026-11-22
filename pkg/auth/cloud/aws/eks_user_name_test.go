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
			assert.Equal(t, kube.UserName(written), EKSKubeUserName(tt.arn, "prod", "us-east-2", tt.identity))
		})
	}
}

// TestLegacyEKSKubeUserName verifies the name versions before the account ID
// and identity were added wrote.
func TestLegacyEKSKubeUserName(t *testing.T) {
	assert.Equal(t, "atmos-eks-prod-us-east-2", LegacyEKSKubeUserName("prod", "us-east-2"))
}
