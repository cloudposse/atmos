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
	t.Setenv("XDG_CONFIG_HOME", tmp) // XDG config home for AWS credentials
	homedir.Reset()                  // Clear homedir cache

	creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret", SessionToken: "token", Region: "us-east-2"}
	err := SetupFiles("prov", "dev", creds, "")
	require.NoError(t, err)

	credPath := filepath.Join(tmp, "atmos", "aws", "prov", "credentials")
	cfgPath := filepath.Join(tmp, "atmos", "aws", "prov", "config")

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

func TestSetupFiles_WithEmptyRegion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // XDG config home for AWS credentials
	homedir.Reset()                  // Clear homedir cache

	// Credentials without region - should default to us-east-1.
	creds := &types.AWSCredentials{AccessKeyID: "AKIA456", SecretAccessKey: "secret2", SessionToken: "token2", Region: ""}
	err := SetupFiles("test-prov", "test-id", creds, "")
	require.NoError(t, err)

	cfgPath := filepath.Join(tmp, "atmos", "aws", "test-prov", "config")

	// Verify default region was applied.
	cfg, err := ini.Load(cfgPath)
	require.NoError(t, err)
	sec := cfg.Section("profile test-id")
	assert.Equal(t, "us-east-1", sec.Key("region").String())
}

func TestSetupFiles_NonAWSCredentials(t *testing.T) {
	// Pass nil credentials (ICredentials interface).
	err := SetupFiles("prov", "id", nil, "")
	require.NoError(t, err) // Should succeed without doing anything.
}

func TestSetupFiles_WithBasePath(t *testing.T) {
	tmp := t.TempDir()
	// basePath replaces the entire ~/.aws/atmos path.
	basePath := filepath.Join(tmp, "custom-base")

	creds := &types.AWSCredentials{AccessKeyID: "AKIA789", SecretAccessKey: "secret3", SessionToken: "token3", Region: "eu-west-1"}
	err := SetupFiles("base-prov", "base-id", creds, basePath)
	require.NoError(t, err)

	// Files are under basePath/base-prov/, not basePath/.aws/atmos/base-prov/.
	credPath := filepath.Join(basePath, "base-prov", "credentials")
	cfgPath := filepath.Join(basePath, "base-prov", "config")

	// Verify credentials file in custom base path.
	cfg, err := ini.Load(credPath)
	require.NoError(t, err)
	_, err = os.Stat(credPath)
	require.NoError(t, err)
	sec := cfg.Section("base-id")
	assert.Equal(t, "AKIA789", sec.Key("aws_access_key_id").String())

	// Verify config file in custom base path.
	cfg2, err := ini.Load(cfgPath)
	require.NoError(t, err)
	sec = cfg2.Section("profile base-id")
	assert.Equal(t, "eu-west-1", sec.Key("region").String())
}

func TestSetEnvironmentVariables_SetsStackEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // XDG config home for AWS credentials
	homedir.Reset()                  // Clear homedir cache

	// Create auth context with AWS credentials.
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			CredentialsFile: filepath.Join(tmp, "atmos", "aws", "prov", "credentials"),
			ConfigFile:      filepath.Join(tmp, "atmos", "aws", "prov", "config"),
			Profile:         "dev",
		},
	}

	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(authContext, stack)
	require.NoError(t, err)

	credPath := filepath.Join("atmos", "aws", "prov", "credentials")
	cfgPath := filepath.Join("atmos", "aws", "prov", "config")

	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], credPath)
	assert.Contains(t, stack.ComponentEnvSection["AWS_CONFIG_FILE"], cfgPath)
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}

func TestSetEnvironmentVariables_WithRegion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // XDG config home for AWS credentials
	homedir.Reset()                  // Clear homedir cache

	// Create auth context with AWS credentials including region.
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			CredentialsFile: filepath.Join(tmp, "atmos", "aws", "prov", "credentials"),
			ConfigFile:      filepath.Join(tmp, "atmos", "aws", "prov", "config"),
			Profile:         "prod",
			Region:          "eu-central-1",
		},
	}

	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(authContext, stack)
	require.NoError(t, err)

	assert.Equal(t, "prod", stack.ComponentEnvSection["AWS_PROFILE"])
	assert.Equal(t, "eu-central-1", stack.ComponentEnvSection["AWS_REGION"])
}

func TestSetEnvironmentVariables_NilAuthContext(t *testing.T) {
	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(nil, stack)
	require.NoError(t, err) // Should succeed without setting anything.
}

func TestSetEnvironmentVariables_NilStackInfo(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test",
		},
	}
	err := SetEnvironmentVariables(authContext, nil)
	require.NoError(t, err) // Should succeed without setting anything.
}

func TestSetEnvironmentVariables_NoAWSContext(t *testing.T) {
	authContext := &schema.AuthContext{} // No AWS context.
	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(authContext, stack)
	require.NoError(t, err) // Should succeed without setting anything.
}

func TestSetAuthContext_PopulatesAuthContext(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // XDG config home for AWS credentials
	homedir.Reset()                  // Clear homedir cache

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
	assert.Contains(t, authContext.AWS.CredentialsFile, filepath.Join("test-provider", "credentials"))
	assert.Contains(t, authContext.AWS.ConfigFile, filepath.Join("test-provider", "config"))
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
	t.Setenv("XDG_CONFIG_HOME", tmp) // XDG config home for AWS credentials
	homedir.Reset()                  // Clear homedir cache

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
