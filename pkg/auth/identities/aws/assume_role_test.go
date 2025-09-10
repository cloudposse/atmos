package aws

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewAssumeRoleIdentity(t *testing.T) {
	// Wrong kind should error
	_, err := NewAssumeRoleIdentity("role", &schema.Identity{Kind: "aws/permission-set"})
	assert.Error(t, err)

	// Correct kind succeeds
	id, err := NewAssumeRoleIdentity("role", &schema.Identity{Kind: "aws/assume-role"})
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Equal(t, "aws/assume-role", id.Kind())
}

func TestAssumeRoleIdentity_ValidateAndProviderName(t *testing.T) {
	// Missing principal -> error
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{Kind: "aws/assume-role"}}
	assert.Error(t, i.Validate())

	// Missing assume_role -> error
	i = &assumeRoleIdentity{name: "role", config: &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{}}}
	assert.Error(t, i.Validate())

	// Valid minimal config with provider via
	i = &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"region":      "us-west-2",
		},
	}}
	require.NoError(t, i.Validate())
	// Provider name resolves from Via.Provider
	prov, err := i.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "aws-sso", prov)

	// Via.Identity fallback
	i.config.Via = &schema.IdentityVia{Identity: "base"}
	prov, err = i.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "base", prov)

	// Neither set -> error
	i.config.Via = &schema.IdentityVia{}
	_, err = i.GetProviderName()
	assert.Error(t, err)
}

func TestAssumeRoleIdentity_Environment(t *testing.T) {
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind:      "aws/assume-role",
		Principal: map[string]any{"assume_role": "arn:aws:iam::123:role/x"},
		Env:       []schema.EnvironmentVariable{{Key: "FOO", Value: "BAR"}},
	}}
	env, err := i.Environment()
	assert.NoError(t, err)
	assert.Equal(t, "BAR", env["FOO"])
}

func TestAssumeRoleIdentity_BuildAssumeRoleInput(t *testing.T) {
	// External ID and duration should be set when provided
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"external_id": "abc-123",
			"duration":    "15m",
		},
	}}
	// Validate populates role arn
	require.NoError(t, i.Validate())
	in := i.buildAssumeRoleInput()
	require.NotNil(t, in)
	assert.NotNil(t, in.ExternalId)
	assert.Equal(t, int32(900), *in.DurationSeconds)

	// Invalid duration -> no DurationSeconds set
	i = &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"duration":    "bogus",
		},
	}}
	require.NoError(t, i.Validate())
	in = i.buildAssumeRoleInput()
	assert.Nil(t, in.DurationSeconds)
}

func TestAssumeRoleIdentity_toAWSCredentials(t *testing.T) {
	i := &assumeRoleIdentity{name: "role", region: "us-east-2"}

	// Nil result -> error
	_, err := i.toAWSCredentials(nil)
	assert.Error(t, err)

	// Valid conversion
	exp := time.Now().Add(time.Hour)
	out := &sts.AssumeRoleOutput{Credentials: &ststypes.Credentials{
		AccessKeyId:     aws.String("AKIA123"),
		SecretAccessKey: aws.String("secret"),
		SessionToken:    aws.String("token"),
		Expiration:      &exp,
	}}
	creds, err := i.toAWSCredentials(out)
	require.NoError(t, err)
	assert.Equal(t, "us-east-2", creds.(*types.AWSCredentials).Region)
}
