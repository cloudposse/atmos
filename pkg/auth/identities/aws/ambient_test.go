package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setEmptyAWSConfigFiles creates empty temp files for AWS config and credentials,
// replacing /dev/null usage for cross-platform compatibility.
func setEmptyAWSConfigFiles(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	creds := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(cfg, []byte(""), 0o600))
	require.NoError(t, os.WriteFile(creds, []byte(""), 0o600))
	t.Setenv("AWS_CONFIG_FILE", cfg)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", creds)
}

func TestNewAWSAmbientIdentity(t *testing.T) {
	tests := []struct {
		name      string
		idName    string
		config    *schema.Identity
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "valid aws/ambient identity",
			idName: "eks-deployer",
			config: &schema.Identity{Kind: "aws/ambient"},
		},
		{
			name:   "valid with principal region",
			idName: "eks-deployer",
			config: &schema.Identity{
				Kind:      "aws/ambient",
				Principal: map[string]interface{}{"region": "us-west-2"},
			},
		},
		{
			name:      "nil config",
			idName:    "bad",
			config:    nil,
			wantErr:   true,
			errSubstr: "nil config",
		},
		{
			name:      "wrong kind",
			idName:    "bad",
			config:    &schema.Identity{Kind: "aws/user"},
			wantErr:   true,
			errSubstr: "invalid identity kind",
		},
		{
			name:   "via is rejected",
			idName: "bad",
			config: &schema.Identity{
				Kind: "aws/ambient",
				Via:  &schema.IdentityVia{Identity: "base-identity"},
			},
			wantErr:   true,
			errSubstr: "must not define via",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewAWSAmbientIdentity(tt.idName, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, identity)
			} else {
				require.NoError(t, err)
				require.NotNil(t, identity)
			}
		})
	}
}

func TestAWSAmbientIdentityKind(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)
	assert.Equal(t, "aws/ambient", identity.Kind())
}

func TestAWSAmbientIdentityGetProviderName(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)
	name, err := identity.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "aws-ambient", name)
}

func TestAWSAmbientIdentityEnvironment(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	env, err := identity.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestAWSAmbientIdentityPaths(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	paths, err := identity.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestAWSAmbientIdentityPrepareEnvironment(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	input := map[string]string{
		"AWS_ACCESS_KEY_ID":           "AKIAIOSFODNN7EXAMPLE",
		"AWS_SECRET_ACCESS_KEY":       "secret",
		"AWS_SESSION_TOKEN":           "token",
		"AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/secrets/eks.amazonaws.com/serviceaccount/token",
		"AWS_ROLE_ARN":                "arn:aws:iam::123456789012:role/my-role",
		"AWS_ROLE_SESSION_NAME":       "my-session",
		"CUSTOM_VAR":                  "value",
	}

	result, err := identity.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	// All credential vars should be preserved — ambient does NOT clear them.
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", result["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "secret", result["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "token", result["AWS_SESSION_TOKEN"])
	assert.Equal(t, "/var/run/secrets/eks.amazonaws.com/serviceaccount/token", result["AWS_WEB_IDENTITY_TOKEN_FILE"])
	assert.Equal(t, "arn:aws:iam::123456789012:role/my-role", result["AWS_ROLE_ARN"])
	assert.Equal(t, "my-session", result["AWS_ROLE_SESSION_NAME"])
	assert.Equal(t, "value", result["CUSTOM_VAR"])

	// IMDS should NOT be disabled.
	_, hasIMDSDisabled := result["AWS_EC2_METADATA_DISABLED"]
	assert.False(t, hasIMDSDisabled, "AWS_EC2_METADATA_DISABLED should NOT be set by ambient identity")
}

func TestAWSAmbientIdentityPrepareEnvironmentWithRegion(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{
		Kind:      "aws/ambient",
		Principal: map[string]interface{}{"region": "eu-west-1"},
	})
	require.NoError(t, err)

	result, err := identity.PrepareEnvironment(context.Background(), map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, "eu-west-1", result["AWS_REGION"])
	assert.Equal(t, "eu-west-1", result["AWS_DEFAULT_REGION"])
}

func TestAWSAmbientIdentityPrepareEnvironmentWithoutRegion(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	result, err := identity.PrepareEnvironment(context.Background(), map[string]string{})
	require.NoError(t, err)

	_, hasRegion := result["AWS_REGION"]
	assert.False(t, hasRegion, "AWS_REGION should not be set when no region configured")
}

func TestAWSAmbientIdentityPrepareEnvironmentDoesNotMutateInput(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{
		Kind:      "aws/ambient",
		Principal: map[string]interface{}{"region": "us-east-1"},
	})
	require.NoError(t, err)

	input := map[string]string{
		"EXISTING": "value",
	}

	result, err := identity.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	// Mutate result and verify input is unchanged.
	result["EXISTING"] = "modified"
	result["NEW_KEY"] = "new"

	assert.Equal(t, "value", input["EXISTING"], "input should not be mutated")
	_, exists := input["NEW_KEY"]
	assert.False(t, exists, "new keys should not appear in input")
	_, exists = input["AWS_REGION"]
	assert.False(t, exists, "region should not appear in input")
}

func TestIsStandaloneAWSAmbientChain(t *testing.T) {
	tests := []struct {
		name       string
		chain      []string
		identities map[string]schema.Identity
		want       bool
	}{
		{
			name:  "single aws/ambient identity",
			chain: []string{"eks-deployer"},
			identities: map[string]schema.Identity{
				"eks-deployer": {Kind: "aws/ambient"},
			},
			want: true,
		},
		{
			name:  "single non-ambient identity",
			chain: []string{"my-user"},
			identities: map[string]schema.Identity{
				"my-user": {Kind: "aws/user"},
			},
			want: false,
		},
		{
			name:  "generic ambient (not aws/ambient)",
			chain: []string{"passthrough"},
			identities: map[string]schema.Identity{
				"passthrough": {Kind: "ambient"},
			},
			want: false,
		},
		{
			name:  "multi-element chain with aws/ambient base",
			chain: []string{"deployer", "eks-pod"},
			identities: map[string]schema.Identity{
				"eks-pod":  {Kind: "aws/ambient"},
				"deployer": {Kind: "aws/assume-role"},
			},
			want: false,
		},
		{
			name:       "empty chain",
			chain:      []string{},
			identities: map[string]schema.Identity{},
			want:       false,
		},
		{
			name:  "identity not found",
			chain: []string{"missing"},
			identities: map[string]schema.Identity{
				"other": {Kind: "aws/ambient"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStandaloneAWSAmbientChain(tt.chain, tt.identities)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAWSAmbientIdentitySetRealm(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	// SetRealm stores the realm. Verify it doesn't panic.
	identity.SetRealm("test-realm")
}

func TestAWSAmbientIdentityValidate(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	err = identity.Validate()
	assert.NoError(t, err)
}

func TestAWSAmbientIdentityPostAuthenticate(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	err = identity.PostAuthenticate(context.Background(), nil)
	assert.NoError(t, err)
}

func TestAWSAmbientIdentityLogout(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	err = identity.Logout(context.Background())
	assert.NoError(t, err)
}

func TestAWSAmbientIdentityAuthenticate(t *testing.T) {
	// Use environment variables to provide fake credentials to the AWS SDK.
	// The SDK resolves from env vars without making network calls.
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")
	// Prevent SDK from trying IMDS or other sources.
	setEmptyAWSConfigFiles(t)

	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{
		Kind:      "aws/ambient",
		Principal: map[string]interface{}{"region": "us-west-2"},
	})
	require.NoError(t, err)

	creds, err := identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, creds)

	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", awsCreds.SecretAccessKey)
	assert.Equal(t, "test-session-token", awsCreds.SessionToken)
	assert.Equal(t, "us-west-2", awsCreds.Region)
}

func TestAWSAmbientIdentityAuthenticateWithoutRegion(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	setEmptyAWSConfigFiles(t)

	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	creds, err := identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, creds)

	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
	// No explicit region and /dev/null config means SDK resolves no region either.
	assert.Empty(t, awsCreds.Region)
}

func TestAWSAmbientIdentityAuthenticateSDKResolvedRegion(t *testing.T) {
	// Set credentials via env vars but region only via AWS_DEFAULT_REGION
	// (not in principal config). The SDK resolves this region, and it should
	// be preserved in the returned credentials.
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")
	setEmptyAWSConfigFiles(t)

	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	creds, err := identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, creds)

	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "ap-southeast-1", awsCreds.Region, "SDK-resolved region should be preserved")
}

func TestAWSAmbientIdentityCredentialsExist(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	setEmptyAWSConfigFiles(t)

	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	exists, err := identity.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAWSAmbientIdentityCredentialsExistNoCreds(t *testing.T) {
	// Clear all AWS credential sources.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	setEmptyAWSConfigFiles(t)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	exists, err := identity.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestAWSAmbientIdentityLoadCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	setEmptyAWSConfigFiles(t)

	identity, err := NewAWSAmbientIdentity("test", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)

	creds, err := identity.LoadCredentials(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)

	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
}

func TestAWSAmbientIdentityResolveRegion(t *testing.T) {
	tests := []struct {
		name      string
		principal map[string]interface{}
		want      string
	}{
		{
			name:      "with region",
			principal: map[string]interface{}{"region": "eu-central-1"},
			want:      "eu-central-1",
		},
		{
			name:      "nil principal",
			principal: nil,
			want:      "",
		},
		{
			name:      "empty region",
			principal: map[string]interface{}{"region": ""},
			want:      "",
		},
		{
			name:      "region not a string",
			principal: map[string]interface{}{"region": 42},
			want:      "",
		},
		{
			name:      "no region key",
			principal: map[string]interface{}{"account": "123456789"},
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := &awsAmbientIdentity{
				name:   "test",
				config: &schema.Identity{Kind: "aws/ambient", Principal: tt.principal},
			}
			assert.Equal(t, tt.want, id.resolveRegion())
		})
	}
}

// mockAWSIdentity is a test double for types.Identity used by AuthenticateStandaloneAWSAmbient tests.
type mockAWSIdentity struct {
	kind      string
	authCreds types.ICredentials
	authErr   error
}

func (m *mockAWSIdentity) Kind() string                            { return m.kind }
func (m *mockAWSIdentity) GetProviderName() (string, error)        { return "mock", nil }
func (m *mockAWSIdentity) Validate() error                         { return nil }
func (m *mockAWSIdentity) Environment() (map[string]string, error) { return nil, nil }
func (m *mockAWSIdentity) Paths() ([]types.Path, error)            { return nil, nil }
func (m *mockAWSIdentity) SetRealm(_ string)                       {}
func (m *mockAWSIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (m *mockAWSIdentity) Logout(_ context.Context) error  { return nil }
func (m *mockAWSIdentity) CredentialsExist() (bool, error) { return true, nil }
func (m *mockAWSIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

func (m *mockAWSIdentity) PrepareEnvironment(_ context.Context, env map[string]string) (map[string]string, error) {
	return env, nil
}

func (m *mockAWSIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return m.authCreds, m.authErr
}

func TestAuthenticateStandaloneAWSAmbient(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		identities   map[string]types.Identity
		wantErr      bool
		errSubstr    string
		wantCreds    bool
	}{
		{
			name:         "success",
			identityName: "eks-deployer",
			identities: map[string]types.Identity{
				"eks-deployer": &mockAWSIdentity{
					kind: "aws/ambient",
					authCreds: &types.AWSCredentials{
						AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
						SecretAccessKey: "secret",
					},
				},
			},
			wantCreds: true,
		},
		{
			name:         "identity not found",
			identityName: "missing",
			identities:   map[string]types.Identity{},
			wantErr:      true,
			errSubstr:    "not found",
		},
		{
			name:         "authentication fails",
			identityName: "broken",
			identities: map[string]types.Identity{
				"broken": &mockAWSIdentity{
					kind:    "aws/ambient",
					authErr: fmt.Errorf("no credentials available"),
				},
			},
			wantErr:   true,
			errSubstr: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := AuthenticateStandaloneAWSAmbient(context.Background(), tt.identityName, tt.identities)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
				if tt.wantCreds {
					require.NotNil(t, creds)
				} else {
					assert.Nil(t, creds)
				}
			}
		})
	}
}
