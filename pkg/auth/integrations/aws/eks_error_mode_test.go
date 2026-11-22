package aws

// Tests that an aws/eks integration with kubeconfig update "error" can re-run,
// as auto-provisioned integrations do on every login (issue #3720, #3675).

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newErrorModeEKSIntegration builds the prod integration with update "error".
func newErrorModeEKSIntegration(t *testing.T, path, alias string) integrations.Integration {
	t.Helper()

	integration, err := NewEKSIntegration(&integrations.IntegrationConfig{
		Name: "prod",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Via:  &schema.IntegrationVia{Identity: "admin"},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name: "prod", Region: "us-east-2", Alias: alias,
				Kubeconfig: &schema.KubeconfigSettings{Path: path, Update: "error"},
			}},
		},
	})
	require.NoError(t, err)
	return integration
}

// TestEKSIntegration_Execute_ErrorModeRerun verifies that re-running the same
// integration in error mode, from the same or another checkout, succeeds.
func TestEKSIntegration_Execute_ErrorModeRerun(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	t.Chdir(t.TempDir())
	require.NoError(t, newErrorModeEKSIntegration(t, kubeconfigPath, "prod").Execute(t.Context(), nil))
	require.NoError(t, newErrorModeEKSIntegration(t, kubeconfigPath, "prod").Execute(t.Context(), nil), "an identical re-run must not conflict")

	t.Chdir(t.TempDir())
	require.NoError(t, newErrorModeEKSIntegration(t, kubeconfigPath, "prod").Execute(t.Context(), nil), "a different ATMOS_CHDIR must not conflict")
}

// TestEKSIntegration_Execute_ErrorModeConflict verifies that error mode still
// refuses to repoint an existing context at another cluster.
func TestEKSIntegration_Execute_ErrorModeConflict(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeEKSEntry(t, kubeconfigPath, "arn:aws:eks:eu-west-1:111111111111:cluster/other", "other", "eu-west-1", "prod", "admin")

	err := newErrorModeEKSIntegration(t, kubeconfigPath, "prod").Execute(t.Context(), nil)
	require.ErrorIs(t, err, errUtils.ErrEKSIntegrationFailed)
	require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
}
