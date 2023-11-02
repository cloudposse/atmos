package aws

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestClusterNamePattern(t *testing.T) {
	// InitCliConfig finds and processes `atmos.yaml` CLI config
	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	// Define variables for a component in a stack
	componentVars := map[any]any{
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
	clusterName := cfg.ReplaceContextTokens(context, cliConfig.Components.Helmfile.ClusterNamePattern)
	u.PrintMessage(fmt.Sprintf("Cluster name: %s", clusterName))
	assert.Equal(t, "cp-plat-ue2-dev-blue-eks-cluster", clusterName)
}
