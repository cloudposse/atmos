package cloudformation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
	cfg, err := buildAWSConfig(context.Background(), &schema.ConfigAndStacksInfo{}, "us-west-2")
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", cfg.Region)
}

// buildAWSConfig must resolve nil info the same as info with no auth
// context — no AWS-managed identity, falls back to the default chain.
func TestBuildAWSConfig_NilInfo(t *testing.T) {
	cfg, err := buildAWSConfig(context.Background(), nil, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Region)
}

// A component `env` that sets the region to something other than the one the
// client resolved is ignored by design (the identity's region wins), so it must
// warn and point at settings.aws_cloudformation.region. Staying silent made the
// stack land in a different region than the user wrote.
func TestWarnIgnoredEnvRegion(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		resolved string
		want     []string
		wantNone bool
	}{
		{
			name:     "AWS_REGION differs",
			info:     &schema.ConfigAndStacksInfo{ComponentEnvSection: map[string]any{"AWS_REGION": "us-west-2"}},
			resolved: "us-east-2",
			want:     []string{"AWS_REGION=us-west-2", "us-east-2", "settings.aws_cloudformation.region"},
		},
		{
			name:     "AWS_DEFAULT_REGION differs",
			info:     &schema.ConfigAndStacksInfo{ComponentEnvSection: map[string]any{"AWS_DEFAULT_REGION": "eu-west-1"}},
			resolved: "us-east-2",
			want:     []string{"AWS_DEFAULT_REGION=eu-west-1", "settings.aws_cloudformation.region"},
		},
		{
			name:     "raw component env section is also read",
			info:     &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"env": map[string]any{"AWS_REGION": "us-west-2"}}},
			resolved: "us-east-2",
			want:     []string{"AWS_REGION=us-west-2"},
		},
		{
			name:     "same region does not warn",
			info:     &schema.ConfigAndStacksInfo{ComponentEnvSection: map[string]any{"AWS_REGION": "us-east-2"}},
			resolved: "us-east-2",
			wantNone: true,
		},
		{
			name:     "no region in env does not warn",
			info:     &schema.ConfigAndStacksInfo{ComponentEnvSection: map[string]any{"FOO": "bar"}},
			resolved: "us-east-2",
			wantNone: true,
		},
		{
			name:     "unresolved region does not warn",
			info:     &schema.ConfigAndStacksInfo{ComponentEnvSection: map[string]any{"AWS_REGION": "us-west-2"}},
			resolved: "",
			wantNone: true,
		},
		{name: "nil info does not warn", info: nil, resolved: "us-east-2", wantNone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := normalizeUIOutput(captureStderr(t, func() { warnIgnoredEnvRegion(tt.info, tt.resolved) }))
			if tt.wantNone {
				assert.Empty(t, out)
				return
			}
			for _, want := range tt.want {
				assert.Contains(t, out, want)
			}
		})
	}
}

// buildAWSConfig emits the warning when the identity-resolved region differs
// from the component env, and not when settings.aws_cloudformation.region
// (passed as the region override) already agrees with it.
func TestBuildAWSConfig_WarnsOnIgnoredEnvRegion(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: map[string]any{"AWS_REGION": "us-west-2"}}

	out := normalizeUIOutput(captureStderr(t, func() {
		_, err := buildAWSConfig(context.Background(), info, "us-east-2")
		require.NoError(t, err)
	}))
	assert.Contains(t, out, "AWS_REGION=us-west-2")

	out = normalizeUIOutput(captureStderr(t, func() {
		_, err := buildAWSConfig(context.Background(), info, "us-west-2")
		require.NoError(t, err)
	}))
	assert.Empty(t, out)
}
