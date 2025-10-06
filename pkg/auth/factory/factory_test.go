package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewProvider_Factory(t *testing.T) {
	// Supported kinds construct without error when minimally valid.
	sso, err := NewProvider("aws-sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start"})
	assert.NoError(t, err)
	assert.NotNil(t, sso)

	saml, err := NewProvider("aws-saml", &schema.Provider{Kind: "aws/saml", Region: "us-east-1", URL: "https://idp.example.com/saml"})
	assert.NoError(t, err)
	assert.NotNil(t, saml)

	oidc, err := NewProvider("github-oidc", &schema.Provider{Kind: "github/oidc", Region: "us-east-1"})
	assert.NoError(t, err)
	assert.NotNil(t, oidc)

	// Unsupported.
	_, err = NewProvider("x", &schema.Provider{Kind: "unknown/kind"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errUtils.ErrInvalidProviderKind.Error())
}

func TestNewIdentity_Factory(t *testing.T) {
	ps, err := NewIdentity("dev", &schema.Identity{Kind: "aws/permission-set"})
	assert.NoError(t, err)
	assert.NotNil(t, ps)

	ar, err := NewIdentity("role", &schema.Identity{Kind: "aws/assume-role"})
	assert.NoError(t, err)
	assert.NotNil(t, ar)

	usr, err := NewIdentity("me", &schema.Identity{Kind: "aws/user"})
	assert.NoError(t, err)
	assert.NotNil(t, usr)

	_, err = NewIdentity("x", &schema.Identity{Kind: "unknown/kind"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errUtils.ErrInvalidIdentityKind.Error())
}
