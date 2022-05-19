package aws

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestClusterNamePattern(t *testing.T) {
	// InitConfig finds and processes `atmos.yaml` CLI config
	err := c.InitConfig()
	assert.Nil(t, err)

	// Define variables for a component in a stack
	componentVars := map[interface{}]interface{}{
		"namespace":   "eg",
		"tenant":      "plat",
		"environment": "ue2",
		"stage":       "dev",
		"attributes":  []string{"blue"},
	}

	// Build `Context` from the variables
	context := c.GetContextFromVars(componentVars)

	// Build EKS cluster name using the `components.helmfile.cluster_name_pattern` config from `atmos.yaml`
	// cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-{attributes}-eks-cluster"
	clusterName := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.ClusterNamePattern)
	u.PrintInfo(fmt.Sprintf("Cluster name: %s", clusterName))
	assert.Equal(t, "eg-plat-ue2-dev-blue-eks-cluster", clusterName)
}
