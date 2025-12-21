package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestNewAssumeRootIdentity(t *testing.T) {
	tests := []struct {
		name        string
		identName   string
		config      *schema.Identity
		expectError bool
		wantErr     error
	}{
		{
			name:        "valid config",
			identName:   "root-access",
			config:      &schema.Identity{Kind: "aws/assume-root"},
			expectError: false,
		},
		{
			name:        "wrong kind",
			identName:   "root-access",
			config:      &schema.Identity{Kind: "aws/assume-role"},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityKind,
		},
		{
			name:        "empty name",
			identName:   "",
			config:      &schema.Identity{Kind: "aws/assume-root"},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:        "nil config",
			identName:   "root-access",
			config:      nil,
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewAssumeRootIdentity(tt.identName, tt.config)
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, id)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, id)
				assert.Equal(t, "aws/assume-root", id.Kind())
			}
		})
	}
}

func TestAssumeRootIdentity_Validate(t *testing.T) {
	tests := []struct {
		name        string
		principal   map[string]any
		expectError bool
		wantErr     error
	}{
		{
			name:        "nil principal",
			principal:   nil,
			expectError: true,
			wantErr:     errUtils.ErrMissingPrincipal,
		},
		{
			name:        "empty principal",
			principal:   map[string]any{},
			expectError: true,
			wantErr:     errUtils.ErrMissingPrincipal,
		},
		{
			name: "missing task_policy_arn",
			principal: map[string]any{
				"target_principal": "123456789012",
			},
			expectError: true,
			wantErr:     errUtils.ErrMissingPrincipal,
		},
		{
			name: "invalid account ID format (too short)",
			principal: map[string]any{
				"target_principal": "12345678901",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "invalid account ID format (too long)",
			principal: map[string]any{
				"target_principal": "1234567890123",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "invalid account ID format (letters)",
			principal: map[string]any{
				"target_principal": "12345678901a",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "invalid task policy ARN (wrong prefix)",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::123456789012:role/MyRole",
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "valid minimal config",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			expectError: false,
		},
		{
			name: "valid config with region",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
				"region":           "us-west-2",
			},
			expectError: false,
		},
		{
			name: "valid config with all task policies",
			principal: map[string]any{
				"target_principal": "987654321098",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind:      "aws/assume-root",
					Principal: tt.principal,
				},
			}
			err := i.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssumeRootIdentity_Validate_SetsFields(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"region":           "eu-west-1",
			},
		},
	}

	require.NoError(t, i.Validate())
	assert.Equal(t, "123456789012", i.targetPrincipal)
	assert.Equal(t, "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials", i.taskPolicyArn)
	assert.Equal(t, "eu-west-1", i.region)
}

func TestAssumeRootIdentity_GetProviderName(t *testing.T) {
	tests := []struct {
		name           string
		via            *schema.IdentityVia
		expectedResult string
		expectError    bool
	}{
		{
			name:           "via provider",
			via:            &schema.IdentityVia{Provider: "aws-sso"},
			expectedResult: "aws-sso",
			expectError:    false,
		},
		{
			name:           "via identity",
			via:            &schema.IdentityVia{Identity: "root-access-permission-set"},
			expectedResult: "root-access-permission-set",
			expectError:    false,
		},
		{
			name:        "empty via",
			via:         &schema.IdentityVia{},
			expectError: true,
		},
		{
			name:        "nil via",
			via:         nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind: "aws/assume-root",
					Via:  tt.via,
				},
			}

			result, err := i.GetProviderName()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestAssumeRootIdentity_BuildAssumeRootInput(t *testing.T) {
	tests := []struct {
		name             string
		principal        map[string]any
		expectedDuration *int32
	}{
		{
			name: "without duration",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			expectedDuration: nil,
		},
		{
			name: "with valid duration",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"duration":         "10m",
			},
			expectedDuration: aws.Int32(600),
		},
		{
			name: "with max duration",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"duration":         "15m",
			},
			expectedDuration: aws.Int32(900),
		},
		{
			name: "with duration exceeding max (gets capped)",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"duration":         "1h",
			},
			expectedDuration: aws.Int32(900), // Capped to max.
		},
		{
			name: "with invalid duration (ignored)",
			principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"duration":         "invalid",
			},
			expectedDuration: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind:      "aws/assume-root",
					Principal: tt.principal,
				},
			}
			require.NoError(t, i.Validate())

			input := i.buildAssumeRootInput()

			assert.NotNil(t, input)
			assert.Equal(t, "123456789012", *input.TargetPrincipal)
			assert.NotNil(t, input.TaskPolicyArn)
			assert.Equal(t, "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials", *input.TaskPolicyArn.Arn)

			if tt.expectedDuration == nil {
				assert.Nil(t, input.DurationSeconds)
			} else {
				require.NotNil(t, input.DurationSeconds)
				assert.Equal(t, *tt.expectedDuration, *input.DurationSeconds)
			}
		})
	}
}

func TestAssumeRootIdentity_toAWSCredentials(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		result      *sts.AssumeRootOutput
		expectError bool
		expectCreds *types.AWSCredentials
	}{
		{
			name:        "nil result",
			region:      "us-east-1",
			result:      nil,
			expectError: true,
		},
		{
			name:   "nil credentials",
			region: "us-east-1",
			result: &sts.AssumeRootOutput{
				Credentials: nil,
			},
			expectError: true,
		},
		{
			name:   "valid credentials",
			region: "us-west-2",
			result: &sts.AssumeRootOutput{
				Credentials: &ststypes.Credentials{
					AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
					SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
					SessionToken:    aws.String("FwoGZXIvYXdzEBExample"),
					Expiration:      aws.Time(time.Now().Add(15 * time.Minute)),
				},
			},
			expectError: false,
			expectCreds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBExample",
				Region:          "us-west-2",
			},
		},
		{
			name:   "empty region defaults to us-east-1",
			region: "",
			result: &sts.AssumeRootOutput{
				Credentials: &ststypes.Credentials{
					AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
					SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
					SessionToken:    aws.String("FwoGZXIvYXdzEBExample"),
				},
			},
			expectError: false,
			expectCreds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBExample",
				Region:          "us-east-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &assumeRootIdentity{
				name:   "test-root",
				region: tt.region,
			}

			creds, err := i.toAWSCredentials(tt.result)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				require.NoError(t, err)
				require.NotNil(t, creds)

				awsCreds, ok := creds.(*types.AWSCredentials)
				require.True(t, ok)
				assert.Equal(t, tt.expectCreds.AccessKeyID, awsCreds.AccessKeyID)
				assert.Equal(t, tt.expectCreds.SecretAccessKey, awsCreds.SecretAccessKey)
				assert.Equal(t, tt.expectCreds.SessionToken, awsCreds.SessionToken)
				assert.Equal(t, tt.expectCreds.Region, awsCreds.Region)
			}
		})
	}
}

func TestAssumeRootIdentity_Environment(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			Via: &schema.IdentityVia{Provider: "test-provider"},
			Env: []schema.EnvironmentVariable{{Key: "CUSTOM_VAR", Value: "custom-value"}},
		},
	}

	env, err := i.Environment()
	assert.NoError(t, err)

	// Should include custom env vars from config.
	assert.Equal(t, "custom-value", env["CUSTOM_VAR"])
	// Should include AWS file environment variables.
	assert.NotEmpty(t, env["AWS_SHARED_CREDENTIALS_FILE"])
	assert.NotEmpty(t, env["AWS_CONFIG_FILE"])
	assert.NotEmpty(t, env["AWS_PROFILE"])
	assert.Equal(t, "test-root", env["AWS_PROFILE"])
}

func TestAssumeRootIdentity_PostAuthenticate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
		},
	}

	authContext := &schema.AuthContext{}
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBExample",
		Region:          "us-east-1",
	}

	err := i.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stack,
		ProviderName: "aws-sso",
		IdentityName: "test-root",
		Credentials:  creds,
	})

	require.NoError(t, err)
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "test-root", authContext.AWS.Profile)
	require.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], "aws-sso")
}

func TestAssumeRootIdentity_PostAuthenticate_Errors(t *testing.T) {
	tests := []struct {
		name        string
		params      *types.PostAuthenticateParams
		expectError bool
		wantErr     error
	}{
		{
			name:        "nil params",
			params:      nil,
			expectError: true,
			wantErr:     errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "nil credentials",
			params: &types.PostAuthenticateParams{
				AuthContext:  &schema.AuthContext{},
				StackInfo:    &schema.ConfigAndStacksInfo{},
				ProviderName: "aws-sso",
				IdentityName: "test-root",
				Credentials:  nil,
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidAuthConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &assumeRootIdentity{
				name:   "test-root",
				config: &schema.Identity{Kind: "aws/assume-root"},
			}

			err := i.PostAuthenticate(context.Background(), tt.params)
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssumeRootIdentity_Authenticate_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		identity    *assumeRootIdentity
		inputCreds  types.ICredentials
		expectError bool
		wantErr     error
	}{
		{
			name: "nil input credentials",
			identity: &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind: "aws/assume-root",
					Principal: map[string]any{
						"target_principal": "123456789012",
						"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
					},
				},
			},
			inputCreds:  nil,
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "OIDC credentials not supported",
			identity: &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind: "aws/assume-root",
					Principal: map[string]any{
						"target_principal": "123456789012",
						"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
					},
				},
			},
			inputCreds: &types.OIDCCredentials{
				Token:    "test-token",
				Provider: "github",
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.identity.Authenticate(context.Background(), tt.inputCreds)
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssumeRootIdentity_Authenticate_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		principal   map[string]any
		expectError bool
		wantErr     error
	}{
		{
			name:        "nil principal",
			principal:   nil,
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:        "missing target_principal",
			principal:   map[string]any{"task_policy_arn": "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "missing task_policy_arn",
			principal: map[string]any{
				"target_principal": "123456789012",
			},
			expectError: true,
			wantErr:     errUtils.ErrInvalidIdentityConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind:      "aws/assume-root",
					Principal: tt.principal,
				},
			}

			inputCreds := &types.AWSCredentials{
				AccessKeyID:     "AKIAEXAMPLE",
				SecretAccessKey: "basesecret",
				SessionToken:    "basetoken",
				Region:          "us-east-1",
			}

			_, err := identity.Authenticate(context.Background(), inputCreds)
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssumeRootIdentity_newSTSClient_RegionResolution(t *testing.T) {
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	testCases := []struct {
		name           string
		identityRegion string
		baseRegion     string
		expectedRegion string
	}{
		{
			name:           "uses identity region when set",
			identityRegion: "eu-west-1",
			baseRegion:     "us-east-1",
			expectedRegion: "eu-west-1",
		},
		{
			name:           "falls back to base region",
			identityRegion: "",
			baseRegion:     "ap-south-1",
			expectedRegion: "ap-south-1",
		},
		{
			name:           "defaults to us-east-1",
			identityRegion: "",
			baseRegion:     "",
			expectedRegion: "us-east-1",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.Identity{
				Kind: "aws/assume-root",
				Via:  &schema.IdentityVia{Provider: "test-provider"},
				Principal: map[string]any{
					"target_principal": "123456789012",
					"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				},
			}

			if tt.identityRegion != "" {
				config.Principal["region"] = tt.identityRegion
			}

			identity, err := NewAssumeRootIdentity("test-root", config)
			require.NoError(t, err)

			ari, ok := identity.(*assumeRootIdentity)
			require.True(t, ok)

			err = ari.Validate()
			require.NoError(t, err)

			baseCreds := &types.AWSCredentials{
				AccessKeyID:     "AKIAEXAMPLE",
				SecretAccessKey: "secret",
				SessionToken:    "token",
				Region:          tt.baseRegion,
			}

			client, err := ari.newSTSClient(context.Background(), baseCreds)
			assert.NoError(t, err)
			assert.NotNil(t, client)
			assert.Equal(t, tt.expectedRegion, ari.region)
		})
	}
}

func TestAssumeRootIdentity_CredentialsExist(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		setupFiles     bool
		expectedExists bool
	}{
		{
			name:           "credentials file exists",
			setupFiles:     true,
			expectedExists: true,
		},
		{
			name:           "credentials file does not exist",
			setupFiles:     false,
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
				Kind: "aws/assume-root",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]any{
					"target_principal": "123456789012",
					"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				},
			})
			require.NoError(t, err)

			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				require.NoError(t, os.WriteFile(credPath, []byte("[test-root]\naws_access_key_id=test\n"), 0o600))
			} else {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			exists, err := identity.CredentialsExist()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedExists, exists)
		})
	}
}

func TestAssumeRootIdentity_LoadCredentials(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		setupFiles    bool
		expectedError bool
	}{
		{
			name:          "successfully loads credentials from files",
			setupFiles:    true,
			expectedError: false,
		},
		{
			name:          "fails when credentials file does not exist",
			setupFiles:    false,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", "")
			t.Setenv("AWS_DEFAULT_REGION", "")

			identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
				Kind: "aws/assume-root",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]any{
					"target_principal": "123456789012",
					"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				},
			})
			require.NoError(t, err)

			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				credContent := `[test-root]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = FwoGZXIvYXdzEBExample
`
				require.NoError(t, os.WriteFile(credPath, []byte(credContent), 0o600))

				configPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "config")
				configContent := `[profile test-root]
region = ap-south-1
output = json
`
				require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))
			} else {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			ctx := context.Background()
			creds, err := identity.LoadCredentials(ctx)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				require.NoError(t, err)
				require.NotNil(t, creds)

				awsCreds, ok := creds.(*types.AWSCredentials)
				require.True(t, ok, "credentials should be AWSCredentials type")
				assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
				assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", awsCreds.SecretAccessKey)
				assert.Equal(t, "FwoGZXIvYXdzEBExample", awsCreds.SessionToken)
				assert.Equal(t, "ap-south-1", awsCreds.Region)
			}
		})
	}
}

func TestAssumeRootIdentity_Logout(t *testing.T) {
	identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
		Kind: "aws/assume-root",
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
	})
	require.NoError(t, err)

	// Set root provider name (required for logout to resolve the provider).
	assumeRoot := identity.(*assumeRootIdentity)
	assumeRoot.SetManagerAndProvider(nil, "test-provider")

	ctx := context.Background()
	err = identity.Logout(ctx)

	// Should succeed (no credentials to delete).
	assert.NoError(t, err)
}

func TestAssumeRootIdentity_SetManagerAndProvider(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
		},
	}

	assert.Nil(t, i.manager)
	assert.Empty(t, i.rootProviderName)

	// Set manager and provider.
	i.SetManagerAndProvider(nil, "test-provider")

	assert.Nil(t, i.manager) // Manager is nil in this test.
	assert.Equal(t, "test-provider", i.rootProviderName)
}

func TestIsSupportedTaskPolicy(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		expected bool
	}{
		{
			name:     "IAMAuditRootUserCredentials",
			arn:      "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			expected: true,
		},
		{
			name:     "IAMCreateRootUserPassword",
			arn:      "arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword",
			expected: true,
		},
		{
			name:     "IAMDeleteRootUserCredentials",
			arn:      "arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
			expected: true,
		},
		{
			name:     "S3UnlockBucketPolicy",
			arn:      "arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
			expected: true,
		},
		{
			name:     "SQSUnlockQueuePolicy",
			arn:      "arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy",
			expected: true,
		},
		{
			name:     "unsupported policy",
			arn:      "arn:aws:iam::aws:policy/root-task/UnsupportedPolicy",
			expected: false,
		},
		{
			name:     "regular IAM policy",
			arn:      "arn:aws:iam::123456789012:policy/MyPolicy",
			expected: false,
		},
		{
			name:     "empty string",
			arn:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSupportedTaskPolicy(tt.arn)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSupportedTaskPolicies(t *testing.T) {
	policies := GetSupportedTaskPolicies()

	// Verify expected policies are present.
	expected := []string{
		"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		"arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword",
		"arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
		"arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
		"arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy",
	}

	assert.Equal(t, len(expected), len(policies))
	for _, exp := range expected {
		assert.Contains(t, policies, exp)
	}

	// Verify it returns a copy, not the original slice.
	policies[0] = "modified"
	assert.NotEqual(t, "modified", GetSupportedTaskPolicies()[0])
}

func TestAssumeRootIdentity_PrepareEnvironment(t *testing.T) {
	i := &assumeRootIdentity{
		name:   "test-root",
		region: "us-west-2",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "test-provider"},
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
		},
	}

	environ := map[string]string{
		"EXISTING_VAR": "existing-value",
	}

	result, err := i.PrepareEnvironment(context.Background(), environ)
	assert.NoError(t, err)

	// Should preserve existing vars.
	assert.Equal(t, "existing-value", result["EXISTING_VAR"])

	// Should set AWS vars.
	assert.NotEmpty(t, result["AWS_SHARED_CREDENTIALS_FILE"])
	assert.NotEmpty(t, result["AWS_CONFIG_FILE"])
	assert.Equal(t, "test-root", result["AWS_PROFILE"])
	assert.Equal(t, "us-west-2", result["AWS_REGION"])
	assert.Equal(t, "us-west-2", result["AWS_DEFAULT_REGION"])
}

func TestAssumeRootIdentity_WithCustomResolver(t *testing.T) {
	config := &schema.Identity{
		Kind: "aws/assume-root",
		Via:  &schema.IdentityVia{Provider: "test-provider"},
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			"region":           "us-east-1",
		},
		Credentials: map[string]any{
			"aws": map[string]any{
				"resolver": map[string]any{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	identity, err := NewAssumeRootIdentity("test-root", config)
	require.NoError(t, err)
	assert.NotNil(t, identity)

	ari, ok := identity.(*assumeRootIdentity)
	require.True(t, ok)
	assert.Equal(t, "test-root", ari.name)
	assert.NotNil(t, ari.config)
	assert.NotNil(t, ari.config.Credentials)

	// Verify resolver config exists.
	awsCreds, ok := ari.config.Credentials["aws"]
	assert.True(t, ok)
	assert.NotNil(t, awsCreds)
}

func TestAssumeRootIdentity_AllTaskPoliciesValidate(t *testing.T) {
	// Verify that all supported task policies pass validation.
	policies := GetSupportedTaskPolicies()

	for _, policy := range policies {
		t.Run(policy, func(t *testing.T) {
			i := &assumeRootIdentity{
				name: "test-root",
				config: &schema.Identity{
					Kind: "aws/assume-root",
					Principal: map[string]any{
						"target_principal": "123456789012",
						"task_policy_arn":  policy,
					},
				},
			}
			err := i.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestAssumeRootIdentity_Paths(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
		},
	}

	paths, err := i.Paths()
	assert.NoError(t, err)
	assert.Empty(t, paths)
}

func TestAssumeRootIdentity_resolveRootProviderName_WithManager(t *testing.T) {
	mockManager := &mockAuthManager{
		providerForIdentity: map[string]string{
			"test-root": "sso-provider",
		},
	}

	i := &assumeRootIdentity{
		name:    "test-root",
		manager: mockManager,
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "fallback-provider"},
		},
	}

	// When manager returns a provider, use it.
	provider, err := i.resolveRootProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "sso-provider", provider)
}

func TestAssumeRootIdentity_resolveRootProviderName_ManagerReturnsEmpty(t *testing.T) {
	mockManager := &mockAuthManager{
		providerForIdentity: map[string]string{},
	}

	i := &assumeRootIdentity{
		name:             "test-root",
		manager:          mockManager,
		rootProviderName: "cached-provider",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "fallback-provider"},
		},
	}

	// When manager returns empty, fall back to cached value.
	provider, err := i.resolveRootProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "cached-provider", provider)
}

func TestAssumeRootIdentity_getRootProviderFromVia_CachedValue(t *testing.T) {
	i := &assumeRootIdentity{
		name:             "test-root",
		rootProviderName: "cached-provider",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "config-provider"},
		},
	}

	// Cached value takes precedence over config.
	provider, err := i.getRootProviderFromVia()
	assert.NoError(t, err)
	assert.Equal(t, "cached-provider", provider)
}

func TestAssumeRootIdentity_getRootProviderFromVia_ViaProvider(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "config-provider"},
		},
	}

	// Falls back to Via.Provider from config.
	provider, err := i.getRootProviderFromVia()
	assert.NoError(t, err)
	assert.Equal(t, "config-provider", provider)
}

func TestAssumeRootIdentity_getRootProviderFromVia_NoProvider(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Identity: "parent-identity"},
		},
	}

	// Cannot determine root provider without Via.Provider.
	_, err := i.getRootProviderFromVia()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestAssumeRootIdentity_CredentialsExist_EmptyAccessKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	// Create credentials file with empty access key.
	credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	require.NoError(t, os.WriteFile(credPath, []byte("[test-root]\naws_access_key_id=\n"), 0o600))

	identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
		Kind: "aws/assume-root",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
	})
	require.NoError(t, err)

	exists, err := identity.CredentialsExist()
	assert.NoError(t, err)
	assert.False(t, exists, "empty access key should return false")
}

func TestAssumeRootIdentity_CredentialsExist_MissingSection(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	// Create credentials file without the identity section.
	credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	require.NoError(t, os.WriteFile(credPath, []byte("[other-identity]\naws_access_key_id=test\n"), 0o600))

	identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
		Kind: "aws/assume-root",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
	})
	require.NoError(t, err)

	exists, err := identity.CredentialsExist()
	assert.NoError(t, err)
	assert.False(t, exists, "missing section should return false")
}

func TestAssumeRootIdentity_Logout_NoProvider(t *testing.T) {
	identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
		Kind: "aws/assume-root",
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
		// No Via configured.
	})
	require.NoError(t, err)

	err = identity.Logout(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestAssumeRootIdentity_Environment_NoViaProvider(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			// No Via configured - should fail to resolve provider.
		},
	}

	_, err := i.Environment()
	assert.Error(t, err)
}

func TestAssumeRootIdentity_PrepareEnvironment_NoProvider(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			// No Via configured.
		},
	}

	environ := map[string]string{}
	_, err := i.PrepareEnvironment(context.Background(), environ)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get provider name")
}

func TestAssumeRootIdentity_parseDurationSeconds_EmptyString(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"duration":         "",
			},
		},
	}
	require.NoError(t, i.Validate())

	duration := i.parseDurationSeconds()
	assert.Nil(t, duration)
}

func TestAssumeRootIdentity_parseDurationSeconds_NoDurationKey(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
		},
	}
	require.NoError(t, i.Validate())

	duration := i.parseDurationSeconds()
	assert.Nil(t, duration)
}

func TestAssumeRootIdentity_parseDurationSeconds_ShortDuration(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"duration":         "5m",
			},
		},
	}
	require.NoError(t, i.Validate())

	duration := i.parseDurationSeconds()
	require.NotNil(t, duration)
	assert.Equal(t, int32(300), *duration)
}

func TestAssumeRootIdentity_toAWSCredentials_NilExpiration(t *testing.T) {
	i := &assumeRootIdentity{
		name:   "test-root",
		region: "us-east-1",
	}

	result := &sts.AssumeRootOutput{
		Credentials: &ststypes.Credentials{
			AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
			SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			SessionToken:    aws.String("FwoGZXIvYXdzEBExample"),
			Expiration:      nil, // Nil expiration.
		},
	}

	creds, err := i.toAWSCredentials(result)
	require.NoError(t, err)
	require.NotNil(t, creds)

	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Empty(t, awsCreds.Expiration)
}

// mockAuthManager implements types.AuthManager for testing.
type mockAuthManager struct {
	providerForIdentity map[string]string
}

func (m *mockAuthManager) GetProviderForIdentity(identityName string) string {
	return m.providerForIdentity[identityName]
}

// Implement other AuthManager methods as no-ops for the mock.
func (m *mockAuthManager) GetCachedCredentials(_ context.Context, _ string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (m *mockAuthManager) Authenticate(_ context.Context, _ string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (m *mockAuthManager) AuthenticateProvider(_ context.Context, _ string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (m *mockAuthManager) Whoami(_ context.Context, _ string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (m *mockAuthManager) Validate() error {
	return nil
}

func (m *mockAuthManager) GetDefaultIdentity(_ bool) (string, error) {
	return "", nil
}

func (m *mockAuthManager) ListIdentities() []string {
	return nil
}

func (m *mockAuthManager) GetFilesDisplayPath(_ string) string {
	return ""
}

func (m *mockAuthManager) GetProviderKindForIdentity(_ string) (string, error) {
	return "", nil
}

func (m *mockAuthManager) GetChain() []string {
	return nil
}

func (m *mockAuthManager) GetStackInfo() *schema.ConfigAndStacksInfo {
	return nil
}

func (m *mockAuthManager) ListProviders() []string {
	return nil
}

func (m *mockAuthManager) GetIdentities() map[string]schema.Identity {
	return nil
}

func (m *mockAuthManager) GetProviders() map[string]schema.Provider {
	return nil
}

func (m *mockAuthManager) Logout(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockAuthManager) LogoutProvider(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockAuthManager) LogoutAll(_ context.Context, _ bool) error {
	return nil
}

func (m *mockAuthManager) GetEnvironmentVariables(_ string) (map[string]string, error) {
	return nil, nil
}

func (m *mockAuthManager) PrepareShellEnvironment(_ context.Context, _ string, _ []string) ([]string, error) {
	return nil, nil
}

func TestAssumeRootIdentity_CredentialsExist_ProviderResolutionError(t *testing.T) {
	// Test when we can't resolve the provider name.
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			// No Via configured - can't resolve provider.
		},
	}

	exists, err := i.CredentialsExist()
	assert.Error(t, err)
	assert.False(t, exists)
}

func TestAssumeRootIdentity_LoadCredentials_ProviderResolutionError(t *testing.T) {
	// Test when we can't resolve the provider name.
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			// No Via configured.
		},
	}

	_, err := i.LoadCredentials(context.Background())
	assert.Error(t, err)
}

func TestAssumeRootIdentity_Validate_RegionFromPrincipal(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"region":           "ap-southeast-1",
			},
		},
	}

	err := i.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "ap-southeast-1", i.region)
}

func TestAssumeRootIdentity_Validate_EmptyTargetPrincipal(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
		},
	}

	err := i.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingPrincipal)
}

func TestAssumeRootIdentity_Validate_EmptyTaskPolicyArn(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Principal: map[string]any{
				"target_principal": "123456789012",
				"task_policy_arn":  "",
			},
		},
	}

	err := i.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingPrincipal)
}

func TestAssumeRootIdentity_PrepareEnvironment_EmptyRegion(t *testing.T) {
	i := &assumeRootIdentity{
		name:   "test-root",
		region: "", // Empty region.
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "test-provider"},
		},
	}

	environ := map[string]string{}
	result, err := i.PrepareEnvironment(context.Background(), environ)
	assert.NoError(t, err)
	// Even with empty region, should still set AWS vars.
	assert.NotEmpty(t, result["AWS_SHARED_CREDENTIALS_FILE"])
	assert.NotEmpty(t, result["AWS_CONFIG_FILE"])
	assert.Equal(t, "test-root", result["AWS_PROFILE"])
}

func TestAssumeRootIdentity_Logout_WithCachedProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
		Kind: "aws/assume-root",
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
	})
	require.NoError(t, err)

	// Set cached provider name.
	assumeRoot := identity.(*assumeRootIdentity)
	assumeRoot.rootProviderName = "cached-provider"

	// Create the credentials directory.
	credPath := filepath.Join(tmpDir, "atmos", "aws", "cached-provider")
	require.NoError(t, os.MkdirAll(credPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(credPath, "credentials"), []byte("[test-root]\naws_access_key_id=test\n"), 0o600))

	err = identity.Logout(context.Background())
	// Should succeed (cleans up files).
	assert.NoError(t, err)
}

func TestAssumeRootIdentity_PostAuthenticate_StoresManagerAndProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
		},
	}

	mockManager := &mockAuthManager{
		providerForIdentity: map[string]string{
			"test-root": "sso-provider",
		},
	}

	authContext := &schema.AuthContext{}
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBExample",
		Region:          "us-east-1",
	}

	err := i.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stack,
		ProviderName: "aws-sso",
		IdentityName: "test-root",
		Credentials:  creds,
		Manager:      mockManager,
	})

	require.NoError(t, err)
	// Verify manager and provider name are stored.
	assert.Equal(t, mockManager, i.manager)
	assert.Equal(t, "aws-sso", i.rootProviderName)
}

func TestAssumeRootIdentity_Environment_WithEnvFromConfig(t *testing.T) {
	i := &assumeRootIdentity{
		name: "test-root",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Provider: "test-provider"},
			Env: []schema.EnvironmentVariable{
				{Key: "CUSTOM_VAR1", Value: "value1"},
				{Key: "CUSTOM_VAR2", Value: "value2"},
			},
		},
	}

	env, err := i.Environment()
	assert.NoError(t, err)

	// Should include custom env vars from config.
	assert.Equal(t, "value1", env["CUSTOM_VAR1"])
	assert.Equal(t, "value2", env["CUSTOM_VAR2"])
}

func TestAssumeRootIdentity_Logout_ViaProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	identity, err := NewAssumeRootIdentity("test-root", &schema.Identity{
		Kind: "aws/assume-root",
		Via:  &schema.IdentityVia{Provider: "sso-provider"},
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
	})
	require.NoError(t, err)

	// Create the credentials directory.
	credPath := filepath.Join(tmpDir, "atmos", "aws", "sso-provider")
	require.NoError(t, os.MkdirAll(credPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(credPath, "credentials"), []byte("[test-root]\naws_access_key_id=test\n"), 0o600))

	err = identity.Logout(context.Background())
	assert.NoError(t, err)
}
