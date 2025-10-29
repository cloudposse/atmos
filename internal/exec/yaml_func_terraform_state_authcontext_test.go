package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAuthContextReachesGetTerraformState is the IDEAL test that verifies
// authContext actually flows through to GetTerraformState when processing
// !terraform.state YAML tags.
//
// This test uses a mock to verify the complete flow:
//  1. stackInfo with authContext is passed to ProcessCustomYamlTags.
//  2. It flows through processNodesWithContext.
//  3. It reaches processCustomTagsWithContext.
//  4. It gets passed to processTagTerraformStateWithContext.
//  5. authContext is extracted and passed to GetTerraformState.
//
// This would have caught the bug where stackInfo was accepted but not used.
func TestAuthContextReachesGetTerraformState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock state getter.
	mockStateGetter := NewMockTerraformStateGetter(ctrl)

	// Save original and replace with mock.
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	// Setup test data.
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
			Region:  "us-west-2",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test-stack",
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// The key assertion: mock expects GetState to be called with the authContext.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			"!terraform.state vpc test-stack bucket_name",
			"test-stack",
			"vpc",
			"bucket_name",
			false,
			gomock.Eq(expectedAuthContext), // This is the critical assertion!
		).
		Return("test-bucket-name", nil).
		Times(1)

	// Create input with !terraform.state tag.
	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.state vpc test-stack bucket_name",
	}

	// Process the YAML tags - this should call our mock with authContext.
	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, stackInfo)

	// Verify no error and result is correct.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-bucket-name", result["backend_config"])

	// The mock automatically verifies that GetState was called with the
	// expected authContext. If stackInfo wasn't threaded through correctly,
	// the mock would receive nil instead and this test would fail.
}

// TestAuthContextNilWhenStackInfoNil verifies backward compatibility -
// when stackInfo is nil, authContext should also be nil.
func TestAuthContextNilWhenStackInfoNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)

	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Expect GetState to be called with nil authContext.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			"!terraform.state vpc test-stack bucket_name",
			"test-stack",
			"vpc",
			"bucket_name",
			false,
			gomock.Nil(), // Should be nil when stackInfo is nil
		).
		Return("test-bucket-name", nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.state vpc test-stack bucket_name",
	}

	// Process with nil stackInfo.
	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-bucket-name", result["backend_config"])
}

// TestAuthContextWithDifferentConfigurations verifies authContext works with
// different AWS configurations.
func TestAuthContextWithDifferentConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		authContext *schema.AuthContext
	}{
		{
			name: "AWS authContext with profile",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "aws-profile",
					Region:  "us-east-1",
				},
			},
		},
		{
			name: "AWS authContext with different region",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "aws-profile",
					Region:  "eu-west-1",
				},
			},
		},
		{
			name: "AWS authContext with credentials file",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					CredentialsFile: "/tmp/test-credentials",
					ConfigFile:      "/tmp/test-config",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStateGetter := NewMockTerraformStateGetter(ctrl)

			originalGetter := stateGetter
			stateGetter = mockStateGetter
			defer func() { stateGetter = originalGetter }()

			stackInfo := &schema.ConfigAndStacksInfo{
				AuthContext: tt.authContext,
				Stack:       "test-stack",
			}

			atmosConfig := &schema.AtmosConfiguration{
				BasePath: t.TempDir(),
			}

			// Verify the exact authContext is passed through.
			mockStateGetter.EXPECT().
				GetState(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Eq(tt.authContext),
				).
				Return("result", nil).
				Times(1)

			input := schema.AtmosSectionMapType{
				"value": "!terraform.state component stack output",
			}

			result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, stackInfo)

			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}
