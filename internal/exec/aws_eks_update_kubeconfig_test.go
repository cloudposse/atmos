package exec

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// createTestEksCommand creates a Cobra command with all EKS update-kubeconfig flags registered.
// This allows testing ExecuteAwsEksUpdateKubeconfigCommand without importing the eks package.
func createTestEksCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "update-kubeconfig"}
	cmd.Flags().StringP("stack", "s", "", "Stack name")
	cmd.Flags().String("profile", "", "AWS CLI profile")
	cmd.Flags().String("name", "", "EKS cluster name")
	cmd.Flags().String("region", "", "AWS region")
	cmd.Flags().String("kubeconfig", "", "Kubeconfig path")
	cmd.Flags().String("role-arn", "", "IAM role ARN")
	cmd.Flags().Bool("dry-run", false, "Dry run mode")
	cmd.Flags().Bool("verbose", false, "Verbose output")
	cmd.Flags().String("alias", "", "Cluster context alias")
	return cmd
}

// TestExecuteAwsEksUpdateKubeconfigCommand_FlagParsing tests that the command
// correctly reads all flags from Cobra and passes them to the context.
func TestExecuteAwsEksUpdateKubeconfigCommand_FlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name: "all required flags with dry-run",
			flags: map[string]string{
				"profile": "test-profile",
				"name":    "test-cluster",
				"region":  "us-east-1",
				"dry-run": "true",
			},
			wantErr: false,
		},
		{
			name: "role-arn with dry-run",
			flags: map[string]string{
				"role-arn": "arn:aws:iam::123456789012:role/TestRole",
				"name":     "test-cluster",
				"region":   "us-west-2",
				"dry-run":  "true",
			},
			wantErr: false,
		},
		{
			name: "all flags with dry-run",
			flags: map[string]string{
				"profile":    "test-profile",
				"name":       "test-cluster",
				"region":     "ap-southeast-1",
				"kubeconfig": "/tmp/test-kubeconfig",
				"alias":      "test-alias",
				"dry-run":    "true",
				"verbose":    "true",
			},
			wantErr: false,
		},
		{
			name: "with component arg and dry-run",
			flags: map[string]string{
				"stack":   "dev",
				"profile": "dev-profile",
				"name":    "dev-cluster",
				"dry-run": "true",
			},
			args:    []string{"eks-cluster"},
			wantErr: false,
		},
		{
			name: "profile and role-arn conflict",
			flags: map[string]string{
				"profile":  "test-profile",
				"role-arn": "arn:aws:iam::123456789012:role/TestRole",
				"name":     "test-cluster",
			},
			wantErr: true,
			errMsg:  "either `profile` or `role-arn` can be specified, but not both",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestEksCommand()
			for k, v := range tt.flags {
				err := cmd.Flags().Set(k, v)
				require.NoError(t, err, "failed to set flag %s", k)
			}

			err := ExecuteAwsEksUpdateKubeconfigCommand(cmd, tt.args)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestExecuteAwsEksUpdateKubeconfigCommand_MissingFlags tests error handling
// when required Cobra flags are not registered on the command.
func TestExecuteAwsEksUpdateKubeconfigCommand_MissingFlags(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func() *cobra.Command
		expectError bool
	}{
		{
			name: "missing stack flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				// Intentionally NOT adding "stack" flag.
				cmd.Flags().String("profile", "", "")
				cmd.Flags().String("name", "", "")
				cmd.Flags().String("region", "", "")
				cmd.Flags().String("kubeconfig", "", "")
				cmd.Flags().String("role-arn", "", "")
				cmd.Flags().Bool("dry-run", false, "")
				cmd.Flags().Bool("verbose", false, "")
				cmd.Flags().String("alias", "", "")
				return cmd
			},
			expectError: true,
		},
		{
			name: "missing profile flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("stack", "", "")
				// Intentionally NOT adding "profile" flag.
				cmd.Flags().String("name", "", "")
				cmd.Flags().String("region", "", "")
				cmd.Flags().String("kubeconfig", "", "")
				cmd.Flags().String("role-arn", "", "")
				cmd.Flags().Bool("dry-run", false, "")
				cmd.Flags().Bool("verbose", false, "")
				cmd.Flags().String("alias", "", "")
				return cmd
			},
			expectError: true,
		},
		{
			name: "missing dry-run flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("stack", "", "")
				cmd.Flags().String("profile", "", "")
				cmd.Flags().String("name", "", "")
				cmd.Flags().String("region", "", "")
				cmd.Flags().String("kubeconfig", "", "")
				cmd.Flags().String("role-arn", "", "")
				// Intentionally NOT adding "dry-run" flag.
				cmd.Flags().Bool("verbose", false, "")
				cmd.Flags().String("alias", "", "")
				return cmd
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			err := ExecuteAwsEksUpdateKubeconfigCommand(cmd, []string{})
			if tt.expectError {
				assert.Error(t, err, "should error when flag is missing")
			}
		})
	}
}

// TestExecuteAwsEksUpdateKubeconfigCommand_ComponentArg tests that the component
// positional argument is correctly extracted from args.
func TestExecuteAwsEksUpdateKubeconfigCommand_ComponentArg(t *testing.T) {
	// With profile + cluster name + dry-run, the command should succeed.
	// This tests that the component arg is correctly read from args[0].
	cmd := createTestEksCommand()
	require.NoError(t, cmd.Flags().Set("profile", "test-profile"))
	require.NoError(t, cmd.Flags().Set("name", "test-cluster"))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))

	// With empty args - component should be empty.
	err := ExecuteAwsEksUpdateKubeconfigCommand(cmd, []string{})
	assert.NoError(t, err)

	// With component arg.
	cmd2 := createTestEksCommand()
	require.NoError(t, cmd2.Flags().Set("profile", "test-profile"))
	require.NoError(t, cmd2.Flags().Set("name", "test-cluster"))
	require.NoError(t, cmd2.Flags().Set("dry-run", "true"))

	err = ExecuteAwsEksUpdateKubeconfigCommand(cmd2, []string{"my-component"})
	assert.NoError(t, err)
}
