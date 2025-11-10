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
	// InitCliConfig finds and processes `atmos.yaml` CLI config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	// Define variables for a component in a stack.
	componentVars := map[string]any{
		"namespace":   "cp",
		"tenant":      "plat",
		"environment": "ue2",
		"stage":       "dev",
		"attributes":  []any{"blue"},
	}

	// Build `Context` from the variables.
	context := cfg.GetContextFromVars(componentVars)

	// Build EKS cluster name using the `components.helmfile.cluster_name_pattern` config from `atmos.yaml`.
	// cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-{attributes}-eks-cluster"
	clusterName := cfg.ReplaceContextTokens(context, atmosConfig.Components.Helmfile.ClusterNamePattern)
	u.PrintMessage(fmt.Sprintf("Cluster name: %s", clusterName))
	assert.Equal(t, "cp-plat-ue2-dev-blue-eks-cluster", clusterName)
}

func TestExecuteAwsEksUpdateKubeconfig(t *testing.T) {
	tests := []struct {
		name              string
		kubeconfigContext schema.AwsEksUpdateKubeconfigContext
		expectError       bool
		errorContains     string
		skip              bool
		skipReason        string
	}{
		{
			name: "happy path - with profile",
			kubeconfigContext: schema.AwsEksUpdateKubeconfigContext{
				ClusterName: "test-cluster",
				Region:      "us-east-1",
				Profile:     "test-profile",
			},
			expectError: false,
			skip:        true,
			skipReason:  "Requires AWS CLI and valid credentials - integration test only",
		},
		{
			name: "happy path - with role ARN",
			kubeconfigContext: schema.AwsEksUpdateKubeconfigContext{
				ClusterName: "test-cluster",
				Region:      "us-east-1",
				RoleArn:     "arn:aws:iam::123456789012:role/test-role",
			},
			expectError: false,
			skip:        true,
			skipReason:  "Requires AWS CLI and valid credentials - integration test only",
		},
		{
			name: "error when both profile and role ARN provided",
			kubeconfigContext: schema.AwsEksUpdateKubeconfigContext{
				ClusterName: "test-cluster",
				Region:      "us-east-1",
				Profile:     "test-profile",
				RoleArn:     "arn:aws:iam::123456789012:role/test-role",
			},
			expectError:   true,
			errorContains: "either `profile` or `role-arn` can be specified, but not both",
		},
		{
			name: "loads cluster name from atmos config when empty",
			kubeconfigContext: schema.AwsEksUpdateKubeconfigContext{
				ClusterName: "",
				Region:      "us-east-1",
				Component:   "eks",
				Stack:       "test-stack",
			},
			expectError: true,
			skip:        true,
			skipReason:  "Requires atmos.yaml config and valid stack - would load from config in real execution",
		},
		{
			name: "loads region from context when empty",
			kubeconfigContext: schema.AwsEksUpdateKubeconfigContext{
				ClusterName: "test-cluster",
				Region:      "",
				Component:   "eks",
				Stack:       "test-stack",
				Profile:     "test-profile",
			},
			expectError: false,
			skip:        true,
			skipReason:  "Requires atmos.yaml config and valid stack - would load region from context in real execution",
		},
		{
			name: "error when AWS CLI not available",
			kubeconfigContext: schema.AwsEksUpdateKubeconfigContext{
				ClusterName: "test-cluster",
				Region:      "us-east-1",
				Profile:     "test-profile",
			},
			expectError: true,
			skip:        true,
			skipReason:  "This test would require mocking ExecuteShellCommand - needs refactoring for proper unit testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip(tt.skipReason)
			}

			err := ExecuteAwsEksUpdateKubeconfig(tt.kubeconfigContext)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestExecuteAwsEksUpdateKubeconfig_ProfileAndRoleArnMutuallyExclusive tests the validation
// that profile and role-arn cannot both be specified.
func TestExecuteAwsEksUpdateKubeconfig_ProfileAndRoleArnMutuallyExclusive(t *testing.T) {
	kubeconfigContext := schema.AwsEksUpdateKubeconfigContext{
		ClusterName: "test-cluster",
		Region:      "us-east-1",
		Profile:     "test-profile",
		RoleArn:     "arn:aws:iam::123456789012:role/test-role",
	}

	err := ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either `profile` or `role-arn` can be specified, but not both")
	assert.Contains(t, err.Error(), "test-profile")
	assert.Contains(t, err.Error(), "arn:aws:iam::123456789012:role/test-role")
}
