package auth

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	assert.Equal(t, []string{"p", "root", "child"}, chain)

	// aws/user without via produces identity-only chain
	only, err := m.buildAuthenticationChain("orphan")
	assert.NoError(t, err)
	assert.Equal(t, []string{"orphan"}, only)
}

func TestGetProviderKindForIdentity(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"p": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev":   {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			"me":    {Kind: "aws/user"},
			"alias": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}, Alias: "developer"},
		},
	}}
	// Populate identities map so getProviderForIdentity can resolve alias using GetProviderName()
	m.identities = map[string]types.Identity{"alias": stubIdentity{provider: "p"}}

	kind, err := m.GetProviderKindForIdentity("dev")
	assert.NoError(t, err)
	assert.Equal(t, "aws/iam-identity-center", kind)

	// For aws/user chain root is the identity itself
	kind, err = m.GetProviderKindForIdentity("me")
	assert.NoError(t, err)
	assert.Equal(t, "aws/user", kind)

	// Alias resolution in getProviderForIdentity
	prov := m.getProviderForIdentity("developer")
	assert.Equal(t, "p", prov)
}
