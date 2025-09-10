package auth

import (
	"context"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubIdentity implements types.Identity minimally for provider lookups.
type stubIdentity struct{ provider string }

func (s stubIdentity) Kind() string                     { return "aws/permission-set" }
func (s stubIdentity) GetProviderName() (string, error) { return s.provider, nil }
func (s stubIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return nil, nil
}
func (s stubIdentity) Validate() error                         { return nil }
func (s stubIdentity) Environment() (map[string]string, error) { return nil, nil }
func (s stubIdentity) PostAuthenticate(_ context.Context, _ *schema.ConfigAndStacksInfo, _ string, _ string, _ types.ICredentials) error {
	return nil
}

func TestBuildAuthenticationChain_Basic(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"aws-sso": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "aws-sso"}},
		},
	}}

	chain, err := m.buildAuthenticationChain("dev")
	assert.NoError(t, err)
	assert.Equal(t, []string{"aws-sso", "dev"}, chain)
}

func TestBuildAuthenticationChain_NestedIdentity(t *testing.T) {
	t.Parallel()

	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"p": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"child":  {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "root"}},
			"root":   {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			"orphan": {Kind: "aws/user"},
		},
	}}

	chain, err := m.buildAuthenticationChain("child")
	require.NoError(t, err)
	assert.Equal(t, []string{"p", "root", "child"}, chain)

	// aws/user without via produces identity-only chain.
	only, err := m.buildAuthenticationChain("orphan")
	require.NoError(t, err)
	assert.Equal(t, []string{"orphan"}, only)
}

func TestGetProviderKindForIdentity(t *testing.T) {
	t.Parallel()

	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"p": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			"me":  {Kind: "aws/user"},
		},
	}}
	// Populate identities map so alias resolution can use GetProviderName().
	m.identities = map[string]types.Identity{"alias": stubIdentity{provider: "p"}}

	kind, err := m.GetProviderKindForIdentity("dev")
	require.NoError(t, err)
	assert.Equal(t, "aws/iam-identity-center", kind)

	// For aws/user chain root is the identity itself.
	kind, err = m.GetProviderKindForIdentity("me")
	require.NoError(t, err)
	assert.Equal(t, "aws/user", kind)

	_, err = m.GetProviderKindForIdentity("developer")
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}
