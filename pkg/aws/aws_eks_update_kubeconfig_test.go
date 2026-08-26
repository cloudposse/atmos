package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestClusterNamePattern(t *testing.T) {
	// InitCliConfig finds and processes `atmos.yaml` CLI config
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	// Define variables for a component in a stack
	componentVars := map[string]any{
		"namespace":   "cp",
		"tenant":      "plat",
		"environment": "ue2",
		"stage":       "dev",
		"attributes":  []any{"blue"},
	}

	// Build `Context` from the variables
	context := cfg.GetContextFromVars(componentVars)

	// Build EKS cluster name using the `components.helmfile.cluster_name_pattern` config from `atmos.yaml`
	// cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-{attributes}-eks-cluster"
	clusterName := cfg.ReplaceContextTokens(context, atmosConfig.Components.Helmfile.ClusterNamePattern)
	assert.Equal(t, "cp-plat-ue2-dev-blue-eks-cluster", clusterName)
}

// TestExecuteAwsEksUpdateKubeconfig_ProfileAndRoleArnConflict verifies that the public
// wrapper propagates the validation error from the underlying implementation when both
// `profile` and `role_arn` are provided. This exercises the wrapper without requiring AWS
// credentials or network access, since the conflict is rejected before any AWS call.
func TestExecuteAwsEksUpdateKubeconfig_ProfileAndRoleArnConflict(t *testing.T) {
	err := ExecuteAwsEksUpdateKubeconfig(schema.AwsEksUpdateKubeconfigContext{
		ClusterName: "test-cluster",
		Profile:     "test-profile",
		RoleArn:     "arn:aws:iam::123456789012:role/test-role",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either `profile` or `role-arn`")
}
