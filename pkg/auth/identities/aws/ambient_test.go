package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
