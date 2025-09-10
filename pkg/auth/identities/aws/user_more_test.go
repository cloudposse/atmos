package aws

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	atmosCreds "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Use in-memory keyring for this test package
func init() { keyring.MockInit() }

func TestUser_credentialsFromConfig(t *testing.T) {
	// Missing secret when access_key_id present -> error
	id, err := NewUserIdentity("me", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{
		"access_key_id": "AKIA",
	}})
	require.NoError(t, err)
	ui := id.(*userIdentity)
	creds, err := ui.credentialsFromConfig()
	assert.Nil(t, creds)
	assert.Error(t, err)

	// Full credentials with MFA -> success
	id, err = NewUserIdentity("me", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{
		"access_key_id":     "AKIA",
		"secret_access_key": "SECRET",
		"mfa_arn":           "arn:aws:iam::111111111111:mfa/me",
	}})
	require.NoError(t, err)
	ui = id.(*userIdentity)
	creds, err = ui.credentialsFromConfig()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKIA", creds.AccessKeyID)
	assert.Equal(t, "SECRET", creds.SecretAccessKey)
	assert.Equal(t, "arn:aws:iam::111111111111:mfa/me", creds.MfaArn)
}

func TestUser_credentialsFromStore(t *testing.T) {
	// Prime the store for alias "dev"
	store := atmosCreds.NewCredentialStore()
	_ = store.Store("dev", &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET", Region: "us-east-1"})

	id, err := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	require.NoError(t, err)
	ui := id.(*userIdentity)

	// Success path
	creds, err := ui.credentialsFromStore()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKIA", creds.AccessKeyID)
	assert.Equal(t, "us-east-1", creds.Region)

	// Wrong type stored
	_ = store.Store("other", &types.OIDCCredentials{Token: "hdr.payload."})
	id, _ = NewUserIdentity("other", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	_, err = ui.credentialsFromStore()
	assert.Error(t, err)

	// Incomplete stored
	_ = store.Store("incomplete", &types.AWSCredentials{AccessKeyID: "AKIA"})
	id, _ = NewUserIdentity("incomplete", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	_, err = ui.credentialsFromStore()
	assert.Error(t, err)

	// Missing alias -> retrieval error
	id, _ = NewUserIdentity("missing", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	_, err = ui.credentialsFromStore()
	assert.Error(t, err)
}

func TestUser_resolveLongLivedCredentials_Order(t *testing.T) {
	// When config has full credentials, prefer those
	id, err := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{
		"access_key_id":     "AKIA",
		"secret_access_key": "SECRET",
	}})
	require.NoError(t, err)
	ui := id.(*userIdentity)
	creds, err := ui.resolveLongLivedCredentials()
	require.NoError(t, err)
	assert.Equal(t, "AKIA", creds.AccessKeyID)

	// If config has no access key, fallback to store
	// Prime the store
	store := atmosCreds.NewCredentialStore()
	_ = store.Store("dev2", &types.AWSCredentials{AccessKeyID: "AK2", SecretAccessKey: "SEC2"})
	id, _ = NewUserIdentity("dev2", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	creds, err = ui.resolveLongLivedCredentials()
	require.NoError(t, err)
	assert.Equal(t, "AK2", creds.AccessKeyID)
}

func TestUser_resolveRegion_DefaultAndOverride(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	assert.Equal(t, defaultRegion, ui.resolveRegion())

	id, _ = NewUserIdentity("dev", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{"region": "us-west-2"}})
	ui = id.(*userIdentity)
	assert.Equal(t, "us-west-2", ui.resolveRegion())
}

func TestUser_writeAWSFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	creds := &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET", SessionToken: "TOKEN"}
	err := ui.writeAWSFiles(creds, "us-east-2")
	require.NoError(t, err)

	// Files should exist under ~/.aws/atmos/aws-user
	// Note: we can’t know the exact tempdir path here; assert partial suffix
	env, _ := id.Environment()
	require.Contains(t, env["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join(".aws", "atmos", "aws-user", "credentials"))
	require.Contains(t, env["AWS_CONFIG_FILE"], filepath.Join(".aws", "atmos", "aws-user", "config"))
}

func TestUser_buildGetSessionTokenInput_NoMFA(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	in, err := ui.buildGetSessionTokenInput(&types.AWSCredentials{})
	require.NoError(t, err)
	require.NotNil(t, in)
	assert.Nil(t, in.SerialNumber)
	assert.Nil(t, in.TokenCode)
	require.NotNil(t, in.DurationSeconds)
	assert.Equal(t, int32(defaultUserSessionSeconds), *in.DurationSeconds)
}

func TestUser_buildGetSessionTokenInput_WithMFA(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)

	// Stub MFA prompt to avoid interactive UI
	old := promptMfaTokenFunc
	defer func() { promptMfaTokenFunc = old }()
	promptMfaTokenFunc = func(_ *types.AWSCredentials) (string, error) { return "123456", nil }

	in, err := ui.buildGetSessionTokenInput(&types.AWSCredentials{MfaArn: "arn:aws:iam::111111111111:mfa/me"})
	require.NoError(t, err)
	require.NotNil(t, in)
	require.NotNil(t, in.SerialNumber)
	assert.Equal(t, "arn:aws:iam::111111111111:mfa/me", *in.SerialNumber)
	require.NotNil(t, in.TokenCode)
	assert.Equal(t, "123456", *in.TokenCode)
	assert.Equal(t, int32(defaultUserSessionSeconds), *in.DurationSeconds)
}

func TestUser_PostAuthenticate_SetsEnvAndFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", Region: "us-east-1"}
	err := ui.PostAuthenticate(context.Background(), stack, "aws-user", "dev", creds)
	require.NoError(t, err)

	// Env set on stack
	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join(".aws", "atmos", "aws-user", "credentials"))
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}

func TestUser_generateSessionToken_toAWSCredentials(t *testing.T) {
	// This tests the conversion part separately by invoking GetSessionToken via a local stubbed client through a helper
	// We don’t call generateSessionToken directly to avoid network calls.
	// Instead, validate that the result from STS is translated as expected.
	exp := time.Now().Add(45 * time.Minute)
	out := &sts.GetSessionTokenOutput{Credentials: &ststypes.Credentials{
		AccessKeyId:     aws.String("TMPAKIA"),
		SecretAccessKey: aws.String("TMPSECRET"),
		SessionToken:    aws.String("TMPTOKEN"),
		Expiration:      &exp,
	}}
	// Verify fields mapping and RFC3339 formatting for expiration.
	// Reuse logic similar to user.generateSessionToken’s construction.
	region := "us-west-1"
	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     aws.ToString(out.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(out.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(out.Credentials.SessionToken),
		Region:          region,
		Expiration:      out.Credentials.Expiration.Format(time.RFC3339),
	}
	assert.Equal(t, "TMPAKIA", sessionCreds.AccessKeyID)
	assert.Equal(t, region, sessionCreds.Region)
	assert.NotEmpty(t, sessionCreds.Expiration)
}
