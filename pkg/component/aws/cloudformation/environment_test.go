package cloudformation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// isolateAWSDefaultChain prevents ambient shared profiles from affecting config construction.
func isolateAWSDefaultChain(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// awsAuthContextFrom must extract the active identity's AWS auth context, and
// return nil (not panic) when info or its AuthContext is unset.
func TestAwsAuthContextFrom(t *testing.T) {
	tests := []struct {
		name string
		info *schema.ConfigAndStacksInfo
		want *schema.AWSAuthContext
	}{
		{"nil info", nil, nil},
		{"nil auth context", &schema.ConfigAndStacksInfo{}, nil},
		{
			"populated auth context",
			&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "dev"}}},
			&schema.AWSAuthContext{Profile: "dev"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, awsAuthContextFrom(tt.info))
		})
	}
}

// resolveEndpointURL must return the active identity's endpoint override, or
// "" (real AWS) when no AWS auth context is active or no override is set.
func TestResolveEndpointURL(t *testing.T) {
	tests := []struct {
		name string
		info *schema.ConfigAndStacksInfo
		want string
	}{
		{"nil info", nil, ""},
		{"no auth context", &schema.ConfigAndStacksInfo{}, ""},
		{
			"auth context without endpoint override",
			&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "dev"}}},
			"",
		},
		{
			"emulator endpoint override",
			&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{EndpointURL: "http://localhost:4566"}}},
			"http://localhost:4566",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveEndpointURL(tt.info))
		})
	}
}

// buildAWSConfig must thread the requested region into the resolved
// aws.Config, and succeed without a live AWS identity — config construction
// (unlike an actual API call) never touches the network, so this asserts
// real, deterministic behavior rather than merely "doesn't panic".
func TestBuildAWSConfig_PropagatesRegion(t *testing.T) {
	isolateAWSDefaultChain(t)
	cfg, err := buildAWSConfig(context.Background(), &schema.ConfigAndStacksInfo{}, "us-west-2")
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", cfg.Region)
}

// buildAWSConfig must resolve nil info the same as info with no auth
// context — no AWS-managed identity, falls back to the default chain.
func TestBuildAWSConfig_NilInfo(t *testing.T) {
	isolateAWSDefaultChain(t)
	cfg, err := buildAWSConfig(context.Background(), nil, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Region)
}
