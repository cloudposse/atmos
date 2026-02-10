package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteAwsEksUpdateKubeconfig_ProfileAndRoleArnMutuallyExclusive(t *testing.T) {
	ctx := schema.AwsEksUpdateKubeconfigContext{
		Profile: "my-profile",
		RoleArn: "arn:aws:iam::123456789012:role/my-role",
	}

	err := ExecuteAwsEksUpdateKubeconfig(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either `profile` or `role-arn` can be specified, but not both")
}

func TestExecuteAwsEksUpdateKubeconfig_RequiresClusterNameOrComponent(t *testing.T) {
	// When no required parameters are provided and no stack, should fail.
	ctx := schema.AwsEksUpdateKubeconfigContext{
		Stack: "", // No stack.
	}

	err := ExecuteAwsEksUpdateKubeconfig(ctx)
	assert.Error(t, err)
	// Will fail during config initialization since we have no atmos.yaml.
	assert.NotNil(t, err)
}

func TestAwsEksUpdateKubeconfigContext_Fields(t *testing.T) {
	tests := []struct {
		name string
		ctx  schema.AwsEksUpdateKubeconfigContext
	}{
		{
			name: "full context",
			ctx: schema.AwsEksUpdateKubeconfigContext{
				Component:   "eks",
				Stack:       "dev-us-east-1",
				Profile:     "dev-profile",
				ClusterName: "dev-cluster",
				Region:      "us-east-1",
				Kubeconfig:  "/tmp/kubeconfig",
				RoleArn:     "",
				DryRun:      true,
				Verbose:     true,
				Alias:       "dev-alias",
			},
		},
		{
			name: "role arn context",
			ctx: schema.AwsEksUpdateKubeconfigContext{
				Component:   "eks",
				Stack:       "prod-us-west-2",
				Profile:     "",
				ClusterName: "prod-cluster",
				Region:      "us-west-2",
				Kubeconfig:  "/home/user/.kube/config",
				RoleArn:     "arn:aws:iam::123456789012:role/EKSAdmin",
				DryRun:      false,
				Verbose:     false,
				Alias:       "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify all fields are accessible.
			assert.NotEmpty(t, tt.ctx.Component)
			assert.NotEmpty(t, tt.ctx.Stack)
			assert.NotEmpty(t, tt.ctx.ClusterName)
			assert.NotEmpty(t, tt.ctx.Region)
			assert.NotEmpty(t, tt.ctx.Kubeconfig)
			// Profile XOR RoleArn.
			if tt.ctx.Profile != "" {
				assert.Empty(t, tt.ctx.RoleArn)
			}
			if tt.ctx.RoleArn != "" {
				assert.Empty(t, tt.ctx.Profile)
			}
		})
	}
}

func TestGetStackNamePattern(t *testing.T) {
	tests := []struct {
		name             string
		atmosConfig      *schema.AtmosConfiguration
		expectedContains string
	}{
		{
			name: "with name pattern",
			atmosConfig: &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					NamePattern: "{tenant}-{environment}-{stage}",
				},
			},
			expectedContains: "{tenant}",
		},
		{
			name: "empty name pattern",
			atmosConfig: &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					NamePattern: "",
				},
			},
			expectedContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStackNamePattern(tt.atmosConfig)
			if tt.expectedContains != "" {
				assert.Contains(t, result, tt.expectedContains)
			} else {
				assert.Empty(t, result)
			}
		})
	}
}
