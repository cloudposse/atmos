package aws

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	u.PrintMessage(fmt.Sprintf("Cluster name: %s", clusterName))
	assert.Equal(t, "cp-plat-ue2-dev-blue-eks-cluster", clusterName)
}

func TestExecuteAwsEksUpdateKubeconfig(t *testing.T) {
	t.Run("returns error when aws cli not available", func(t *testing.T) {
		// This test verifies the function handles missing AWS CLI gracefully.
		// We expect an error since AWS CLI is not available in test environment.
		kubeconfigContext := schema.AwsEksUpdateKubeconfigContext{
			ClusterName: "test-cluster",
			Region:      "us-east-1",
		}

		err := ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
		// We expect an error because AWS CLI is likely not installed or configured.
		// This at least exercises the code path.
		assert.Error(t, err)
	})

	t.Run("handles empty cluster name", func(t *testing.T) {
		kubeconfigContext := schema.AwsEksUpdateKubeconfigContext{
			ClusterName: "",
			Region:      "us-east-1",
		}

		err := ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
		assert.Error(t, err)
	})

	t.Run("handles empty region", func(t *testing.T) {
		kubeconfigContext := schema.AwsEksUpdateKubeconfigContext{
			ClusterName: "test-cluster",
			Region:      "",
		}

		err := ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
		assert.Error(t, err)
	})
}
