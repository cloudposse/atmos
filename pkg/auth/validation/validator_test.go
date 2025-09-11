package validation

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestValidateLogsConfig(t *testing.T) {
	v := &validator{}
	// Empty -> ok.
	err := v.ValidateLogsConfig(&schema.Logs{})
	assert.NoError(t, err)

	// Valid -> ok.
	err = v.ValidateLogsConfig(&schema.Logs{Level: "Info"})
	assert.NoError(t, err)

	// Invalid level.
	err = v.ValidateLogsConfig(&schema.Logs{Level: "Verbose"})
	assert.Error(t, err)
}

func TestValidateProvider(t *testing.T) {
	v := NewValidator()

	// SSO ok.
	err := v.ValidateProvider("aws-sso", &schema.Provider{Kind: "aws/iam-identity-center", StartURL: "https://example.awsapps.com/start", Region: "us-east-1"})
	assert.NoError(t, err)

	// SAML needs url and region.
	err = v.ValidateProvider("aws-saml", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com/saml", Region: "us-east-1"})
	assert.NoError(t, err)

    // Unsupported kind.
	err = v.ValidateProvider("x", &schema.Provider{Kind: "unknown/kind"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errUtils.ErrInvalidProviderKind.Error())
}

func TestValidateIdentity(t *testing.T) {
	v := NewValidator()
	providers := map[string]*schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start"},
	}

	// aws/user requires no via.
	err := v.ValidateIdentity("me", &schema.Identity{Kind: "aws/user"}, providers)
	assert.NoError(t, err)

	// assume-role requires principal.assume_role and arn format.
	err = v.ValidateIdentity("role", &schema.Identity{Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "aws-sso"}, Principal: map[string]any{"assume_role": "arn:aws:iam::123456789012:role/MyRole"}}, providers)
	assert.NoError(t, err)

	// bad arn.
	err = v.ValidateIdentity("role-bad", &schema.Identity{Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "aws-sso"}, Principal: map[string]any{"assume_role": "not-an-arn"}}, providers)
	assert.Error(t, err)

	// permission-set requires principal.name and account name/id.
	err = v.ValidateIdentity("ps", &schema.Identity{Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "aws-sso"}, Principal: map[string]any{"name": "DevAccess", "account": map[string]any{"name": "dev"}}}, providers)
	assert.NoError(t, err)
}

func TestValidateChains(t *testing.T) {
	v := NewValidator()
	identities := map[string]*schema.Identity{
		"a": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "b"}},
		"b": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "c"}},
		"c": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "aws-sso"}},
	}
	providers := map[string]*schema.Provider{"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start"}}

	err := v.ValidateChains(identities, providers)
	assert.NoError(t, err)

	// Introduce a cycle a->b->a.
	identitiesCycle := map[string]*schema.Identity{
		"a": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "b"}},
		"b": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "a"}},
	}
	err = v.ValidateChains(identitiesCycle, providers)
	assert.ErrorIs(t, err, ErrIdentityCycle)
}

func TestValidateAuthConfig(t *testing.T) {
	v := NewValidator()
	cfg := &schema.AuthConfig{
		Logs:      schema.Logs{Level: "Info"},
		Providers: map[string]schema.Provider{"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start"}},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "aws-sso"}, Principal: map[string]any{"name": "DevAccess", "account": map[string]any{"name": "dev"}}},
		},
	}
	assert.NoError(t, v.ValidateAuthConfig(cfg))

	// bad logs level.
	bad := *cfg
	bad.Logs.Level = "Verbose"
	err := v.ValidateAuthConfig(&bad)
	assert.Error(t, err)
}
