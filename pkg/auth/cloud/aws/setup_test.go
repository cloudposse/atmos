package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ini "gopkg.in/ini.v1"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetupFiles_WritesCredentialsAndConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	homedir.Reset() // Clear homedir cache to pick up the test HOME

	creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret", SessionToken: "token", Region: "us-east-2"}
	err := SetupFiles("prov", "dev", creds, "")
	require.NoError(t, err)

	credPath := filepath.Join(tmp, ".aws", "atmos", "prov", "credentials")
	cfgPath := filepath.Join(tmp, ".aws", "atmos", "prov", "config")

	// Verify credentials file.
	cfg, err := ini.Load(credPath)
	require.NoError(t, err)
	_, err = os.Stat(credPath)
	require.NoError(t, err)
	sec := cfg.Section("dev")
	assert.Equal(t, "AKIA123", sec.Key("aws_access_key_id").String())
	assert.Equal(t, "secret", sec.Key("aws_secret_access_key").String())
	assert.Equal(t, "token", sec.Key("aws_session_token").String())

	// Verify config file.
	cfg2, err := ini.Load(cfgPath)
	require.NoError(t, err)
	_, err = os.Stat(cfgPath)
	require.NoError(t, err)
	sec = cfg2.Section("profile dev")
	assert.Equal(t, "us-east-2", sec.Key("region").String())
}

func TestSetEnvironmentVariables_SetsStackEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	homedir.Reset() // Clear homedir cache to pick up the test HOME

	// Create auth context with AWS credentials.
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			CredentialsFile: filepath.Join(tmp, ".aws", "atmos", "prov", "credentials"),
			ConfigFile:      filepath.Join(tmp, ".aws", "atmos", "prov", "config"),
			Profile:         "dev",
		},
	}

	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(authContext, stack)
	require.NoError(t, err)

	credPath := filepath.Join(".aws", "atmos", "prov", "credentials")
	cfgPath := filepath.Join(".aws", "atmos", "prov", "config")

	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], credPath)
	assert.Contains(t, stack.ComponentEnvSection["AWS_CONFIG_FILE"], cfgPath)
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}

func TestSetAuthContext_PopulatesAuthContext(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	homedir.Reset() // Clear homedir cache to pick up the test HOME

	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA123",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-west-2",
	}

	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	err := SetAuthContext(&SetAuthContextParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "test-provider",
		IdentityName: "test-identity",
		Credentials:  creds,
		BasePath:     "",
	})
	require.NoError(t, err)

	// Verify auth context was populated.
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "test-identity", authContext.AWS.Profile)
	assert.Equal(t, "us-west-2", authContext.AWS.Region)
	assert.Contains(t, authContext.AWS.CredentialsFile, "test-provider/credentials")
	assert.Contains(t, authContext.AWS.ConfigFile, "test-provider/config")
}

func TestSetAuthContext_NilParams(t *testing.T) {
	err := SetAuthContext(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SetAuthContext parameters cannot be nil")
}

func TestSetAuthContext_NilAuthContext(t *testing.T) {
	creds := &types.AWSCredentials{Region: "us-east-1"}
	stackInfo := &schema.ConfigAndStacksInfo{}

	err := SetAuthContext(&SetAuthContextParams{
		AuthContext:  nil,
		StackInfo:    stackInfo,
		ProviderName: "test-provider",
		IdentityName: "test-identity",
		Credentials:  creds,
		BasePath:     "",
	})
	require.NoError(t, err) // No auth context to populate, should succeed.
}

func TestSetAuthContext_NonAWSCredentials(t *testing.T) {
	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	// Pass non-AWS credentials (nil implements ICredentials).
	err := SetAuthContext(&SetAuthContextParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "test-provider",
		IdentityName: "test-identity",
		Credentials:  nil,
		BasePath:     "",
	})
	require.NoError(t, err) // Should succeed, just doesn't populate AWS context.
}

func TestSetAuthContext_WithComponentRegionOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	homedir.Reset() // Clear homedir cache to pick up the test HOME

	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA123",
		SecretAccessKey: "secret",
		Region:          "us-west-2",
	}

	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentAuthSection: map[string]any{
			"identities": map[string]any{
				"test-identity": map[string]any{
					"region": "eu-west-1",
				},
			},
		},
	}

	err := SetAuthContext(&SetAuthContextParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "test-provider",
		IdentityName: "test-identity",
		Credentials:  creds,
		BasePath:     "",
	})
	require.NoError(t, err)

	// Verify region override was applied.
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "eu-west-1", authContext.AWS.Region)
}

func TestGetComponentRegionOverride_WithValidOverride(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentAuthSection: map[string]any{
			"identities": map[string]any{
				"dev-identity": map[string]any{
					"region": "ap-southeast-1",
				},
			},
		},
	}

	region := getComponentRegionOverride(stackInfo, "dev-identity")
	assert.Equal(t, "ap-southeast-1", region)
}

func TestGetComponentRegionOverride_NoOverride(t *testing.T) {
	tests := []struct {
		name      string
		stackInfo *schema.ConfigAndStacksInfo
	}{
		{
			name:      "nil stack info",
			stackInfo: nil,
		},
		{
			name: "nil component auth section",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: nil,
			},
		},
		{
			name: "missing identities",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{},
			},
		},
		{
			name: "identities not a map",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": "invalid",
				},
			},
		},
		{
			name: "identity not found",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"other-identity": map[string]any{
							"region": "us-east-1",
						},
					},
				},
			},
		},
		{
			name: "identity config not a map",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"test-identity": "invalid",
					},
				},
			},
		},
		{
			name: "region not a string",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"test-identity": map[string]any{
							"region": 123,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region := getComponentRegionOverride(tt.stackInfo, "test-identity")
			assert.Equal(t, "", region)
		})
	}
}
