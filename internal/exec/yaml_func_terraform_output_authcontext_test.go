package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAuthContextReachesGetTerraformOutput is the IDEAL test that verifies
// authContext actually flows through to GetTerraformOutput when processing
// !terraform.output YAML tags.
//
// This test uses a mock to verify the complete flow:
//  1. stackInfo with authContext is passed to ProcessCustomYamlTags.
//  2. It flows through processNodesWithContext.
//  3. It reaches processCustomTagsWithContext.
//  4. It gets passed to processTagTerraformOutputWithContext.
//  5. authContext is extracted and passed to GetTerraformOutput.
//
// This verifies that authContext (containing AWS credentials paths) is properly
// passed to terraform output execution, enabling access to remote state.
func TestAuthContextReachesGetTerraformOutput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock output getter.
	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	// Save original and replace with mock.
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	// Setup test data with authContext containing AWS credentials.
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-profile",
			Region:          "us-west-2",
			CredentialsFile: "/home/user/.atmos/auth/aws-sso/credentials",
			ConfigFile:      "/home/user/.atmos/auth/aws-sso/config",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test-stack",
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// The key assertion: mock expects GetOutput to be called with the authContext.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			"bucket_name",
			false,
			gomock.Eq(expectedAuthContext), // This is the critical assertion!
		).
		Return("test-bucket-name", true, nil).
		Times(1)

	// Create input with !terraform.output tag.
	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.output vpc test-stack bucket_name",
	}

	// Process the YAML tags - this should call our mock with authContext.
	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, stackInfo)

	// Verify no error and result is correct.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-bucket-name", result["backend_config"])

	// The mock automatically verifies that GetOutput was called with the
	// expected authContext. If stackInfo wasn't threaded through correctly,
	// the mock would receive nil instead and this test would fail.
}

// TestAuthContextNilWhenStackInfoNil verifies backward compatibility -
// when stackInfo is nil, authContext should also be nil.
func TestAuthContextNilWhenStackInfoNilForOutput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Expect GetOutput to be called with nil authContext.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			"bucket_name",
			false,
			gomock.Nil(), // Should be nil when stackInfo is nil
		).
		Return("test-bucket-name", true, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.output vpc test-stack bucket_name",
	}

	// Process with nil stackInfo.
	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-bucket-name", result["backend_config"])
}

// TestAuthContextWithDifferentAWSConfigurations verifies authContext works with
// different AWS configurations for terraform.output.
func TestAuthContextWithDifferentAWSConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		authContext *schema.AuthContext
	}{
		{
			name: "AWS authContext with profile and region",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile:         "aws-profile",
					Region:          "us-east-1",
					CredentialsFile: "/tmp/test-credentials",
					ConfigFile:      "/tmp/test-config",
				},
			},
		},
		{
			name: "AWS authContext with different region",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile:         "prod-profile",
					Region:          "eu-west-1",
					CredentialsFile: "/home/user/.atmos/auth/prod/credentials",
					ConfigFile:      "/home/user/.atmos/auth/prod/config",
				},
			},
		},
		{
			name: "AWS authContext without region",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile:         "default",
					CredentialsFile: "/tmp/credentials",
					ConfigFile:      "/tmp/config",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

			originalGetter := outputGetter
			outputGetter = mockOutputGetter
			defer func() { outputGetter = originalGetter }()

			stackInfo := &schema.ConfigAndStacksInfo{
				AuthContext: tt.authContext,
				Stack:       "test-stack",
			}

			atmosConfig := &schema.AtmosConfiguration{
				BasePath: t.TempDir(),
			}

			// Verify the exact authContext is passed through.
			mockOutputGetter.EXPECT().
				GetOutput(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Eq(tt.authContext),
				).
				Return("result", true, nil).
				Times(1)

			input := schema.AtmosSectionMapType{
				"value": "!terraform.output component stack output",
			}

			result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, stackInfo)

			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// TestAuthContextWithOutputNotExists verifies authContext flows through even when
// the terraform output doesn't exist (returns exists=false).
func TestAuthContextWithOutputNotExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test-stack",
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Mock returns exists=false (output doesn't exist yet).
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"component",
			"missing_output",
			false,
			gomock.Eq(expectedAuthContext),
		).
		Return(nil, false, nil). // exists=false
		Times(1)

	input := schema.AtmosSectionMapType{
		"value": "!terraform.output component test-stack missing_output",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, stackInfo)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// When output doesn't exist, the function returns nil.
	assert.Nil(t, result["value"])
}
