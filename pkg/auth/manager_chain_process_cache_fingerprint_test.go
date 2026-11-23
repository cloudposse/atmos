package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields referenced by these tests.
var (
	_ = schema.Identity{Principal: map[string]any{}, Via: &schema.IdentityVia{Provider: ""}}
	_ = schema.Provider{Kind: "", StartURL: ""}
)

// fingerprintTestManager builds a manager for the chain prov -> role with the given
// effective identity and provider configuration.
func fingerprintTestManager(identity types.Identity, role *schema.Identity, prov schema.Provider) *manager {
	return &manager{
		config: &schema.AuthConfig{
			Providers:  map[string]schema.Provider{"prov": prov},
			Identities: map[string]schema.Identity{"role": *role},
		},
		providers:       map[string]types.Provider{"prov": &testProvider{name: "prov", creds: &testCreds{}}},
		identities:      map[string]types.Identity{"role": identity},
		credentialStore: &testStore{data: map[string]any{}},
		chain:           []string{"prov", "role"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}
}

func assumeRoleIdentity(roleARN string) *schema.Identity {
	return &schema.Identity{
		Kind:      "aws/assume-role",
		Via:       &schema.IdentityVia{Provider: "prov"},
		Principal: map[string]any{"assume_role": roleARN},
	}
}

// TestProcessCredentialCache_DifferentIdentityConfigMisses reproduces #3720 finding 2.1:
// when stack or component auth overrides give the same identity name a different role,
// a second manager in the same process must not receive the first manager's credentials.
func TestProcessCredentialCache_DifferentIdentityConfigMisses(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	prodCreds := &testCreds{exp: &exp}
	devCreds := &testCreds{exp: &exp}
	prodIdentity := &countingIdentity{provider: "prov", creds: prodCreds}
	devIdentity := &countingIdentity{provider: "prov", creds: devCreds}
	prov := schema.Provider{Kind: "aws/iam-identity-center", StartURL: "https://example.awsapps.com/start"}

	prod := fingerprintTestManager(prodIdentity, assumeRoleIdentity("arn:aws:iam::111111111111:role/admin"), prov)
	got, err := prod.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	require.Same(t, prodCreds, got)

	dev := fingerprintTestManager(devIdentity, assumeRoleIdentity("arn:aws:iam::222222222222:role/admin"), prov)
	got, err = dev.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Same(t, devCreds, got, "a differently configured identity must not reuse another manager's credentials")
	assert.Equal(t, int32(1), devIdentity.callCount.Load(), "the differently configured identity must authenticate")

	// The same effective configuration still hits the cache.
	again := fingerprintTestManager(devIdentity, assumeRoleIdentity("arn:aws:iam::222222222222:role/admin"), prov)
	got, err = again.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Same(t, devCreds, got)
	assert.Equal(t, int32(1), devIdentity.callCount.Load(), "an identically configured manager must reuse the process cache")
}

// TestProcessCredentialCache_DifferentProviderConfigMisses verifies the chain root's
// effective configuration is part of the key, not only its name.
func TestProcessCredentialCache_DifferentProviderConfigMisses(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	role := assumeRoleIdentity("arn:aws:iam::111111111111:role/admin")
	first := fingerprintTestManager(&countingIdentity{provider: "prov", creds: &testCreds{exp: &exp}}, role,
		schema.Provider{Kind: "aws/iam-identity-center", StartURL: "https://one.awsapps.com/start"})
	second := fingerprintTestManager(&countingIdentity{provider: "prov", creds: &testCreds{exp: &exp}}, role,
		schema.Provider{Kind: "aws/iam-identity-center", StartURL: "https://two.awsapps.com/start"})

	assert.NotEqual(t, first.chainCacheKey(), second.chainCacheKey())
	assert.Equal(t, first.chainCacheKey(), first.chainCacheKey(), "the key must be stable for one manager")
}

// TestProcessCredentialCache_UnserializableConfigSkipsCache verifies a configuration that
// cannot be fingerprinted is never cached, so it can never be served to another manager.
func TestProcessCredentialCache_UnserializableConfigSkipsCache(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	identity := &countingIdentity{provider: "prov", creds: &testCreds{exp: &exp}}
	role := assumeRoleIdentity("arn:aws:iam::111111111111:role/admin")
	role.Spec = map[string]any{"unsupported": func() {}}
	m := fingerprintTestManager(identity, role, schema.Provider{Kind: "aws/iam-identity-center"})

	assert.Empty(t, m.chainCacheKey())
	for range 2 {
		_, err := m.authenticateChain(context.Background(), "role")
		require.NoError(t, err)
	}
	assert.Equal(t, int32(2), identity.callCount.Load(), "an unfingerprintable chain must re-authenticate")
	entries := 0
	processCredentialCache.Range(func(_, _ any) bool {
		entries++
		return true
	})
	assert.Zero(t, entries, "an unfingerprintable chain must never be stored")
}
