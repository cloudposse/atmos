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

func TestExecuteAwsEksUpdateKubeconfig_FailsWithoutConfig(t *testing.T) {
	// When no atmos.yaml config is available, the command should fail during config initialization.
	ctx := schema.AwsEksUpdateKubeconfigContext{
		Stack: "",
	}

	err := ExecuteAwsEksUpdateKubeconfig(ctx)
	assert.Error(t, err)
}

func TestExecuteAwsEksUpdateKubeconfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		ctx           schema.AwsEksUpdateKubeconfigContext
		expectError   bool
		errorContains string
	}{
		{
			name: "profile and role-arn both set",
			ctx: schema.AwsEksUpdateKubeconfigContext{
				Profile:     "my-profile",
				RoleArn:     "arn:aws:iam::123456789012:role/my-role",
				ClusterName: "cluster",
			},
			expectError:   true,
			errorContains: "either `profile` or `role-arn` can be specified, but not both",
		},
		{
			name: "profile only is valid input",
			ctx: schema.AwsEksUpdateKubeconfigContext{
				Profile:     "dev-profile",
				ClusterName: "dev-cluster",
				Region:      "us-east-1",
			},
			// This will fail at AWS CLI execution, not validation.
			expectError:   true,
			errorContains: "", // Error comes from AWS CLI not being able to connect.
		},
		{
			name: "role-arn only is valid input",
			ctx: schema.AwsEksUpdateKubeconfigContext{
				RoleArn:     "arn:aws:iam::123456789012:role/EKSAdmin",
				ClusterName: "prod-cluster",
				Region:      "us-west-2",
			},
			// This will fail at AWS CLI execution, not validation.
			expectError:   true,
			errorContains: "", // Error comes from AWS CLI not being able to connect.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecuteAwsEksUpdateKubeconfig(tt.ctx)
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

func TestExecuteAwsEksUpdateKubeconfig_WithRequiredParams(t *testing.T) {
	// Test with all required parameters provided (profile and cluster name).
	// With DryRun=true, the function should succeed without actually calling AWS CLI.
	ctx := schema.AwsEksUpdateKubeconfigContext{
		Profile:     "my-profile",
		ClusterName: "my-cluster",
		Region:      "us-east-1",
		DryRun:      true, // Use dry-run to avoid actual AWS calls.
	}

	err := ExecuteAwsEksUpdateKubeconfig(ctx)
	// DryRun mode should complete successfully without calling AWS CLI.
	assert.NoError(t, err)
}

func TestExecuteAwsEksUpdateKubeconfig_WithRoleArn(t *testing.T) {
	// Test with role-arn instead of profile.
	// With DryRun=true, the function should succeed without actually calling AWS CLI.
	ctx := schema.AwsEksUpdateKubeconfigContext{
		RoleArn:     "arn:aws:iam::123456789012:role/EKSRole",
		ClusterName: "my-cluster",
		Region:      "us-west-2",
		DryRun:      true,
	}

	err := ExecuteAwsEksUpdateKubeconfig(ctx)
	// DryRun mode should complete successfully.
	assert.NoError(t, err)
}

func TestExecuteAwsEksUpdateKubeconfig_WithAllOptionalParams(t *testing.T) {
	// Test with all optional parameters set.
	// With DryRun=true, the function should succeed without actually calling AWS CLI.
	ctx := schema.AwsEksUpdateKubeconfigContext{
		Profile:     "my-profile",
		ClusterName: "my-cluster",
		Region:      "us-east-1",
		Kubeconfig:  "/tmp/kubeconfig",
		Alias:       "my-alias",
		DryRun:      true,
		Verbose:     true,
	}

	err := ExecuteAwsEksUpdateKubeconfig(ctx)
	// DryRun mode should complete successfully.
	assert.NoError(t, err)
}
