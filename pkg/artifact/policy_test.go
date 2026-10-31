package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerificationPolicyFingerprint(t *testing.T) {
	identityA := TrustedIdentity{Identity: "https://github.com/acme/a/.github/workflows/r.yml@refs/heads/main", Issuer: "https://token.actions.githubusercontent.com"}
	identityB := TrustedIdentity{Identity: "https://github.com/acme/b/.github/workflows/r.yml@refs/heads/main", Issuer: "https://token.actions.githubusercontent.com"}

	base := VerificationPolicy{
		RequireSignature: true,
		TrustSets: []TrustSet{
			{Identities: []TrustedIdentity{identityA, identityB}, Keys: []string{"b.pub", "a.pub"}},
			{Keys: []string{"c.pub"}},
		},
	}
	fingerprint, err := base.Fingerprint()
	require.NoError(t, err)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, fingerprint)

	reordered := VerificationPolicy{
		RequireSignature: true,
		TrustSets: []TrustSet{
			{Keys: []string{"c.pub"}},
			{Identities: []TrustedIdentity{identityB, identityA}, Keys: []string{"a.pub", "b.pub"}},
		},
	}
	same, err := reordered.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, fingerprint, same, "order of trust sets, identities, and keys does not matter")
	assert.Equal(t, "b.pub", base.TrustSets[0].Keys[0], "fingerprinting does not reorder the caller's policy")

	tests := []struct {
		name   string
		mutate func(p *VerificationPolicy)
	}{
		{"require signature", func(p *VerificationPolicy) { p.RequireSignature = false }},
		{"offline", func(p *VerificationPolicy) { p.Offline = true }},
		{"dropped trust set", func(p *VerificationPolicy) { p.TrustSets = p.TrustSets[:1] }},
		{"different key", func(p *VerificationPolicy) {
			p.TrustSets = []TrustSet{{Keys: []string{"x.pub"}}, {Keys: []string{"c.pub"}}}
		}},
		{"unioned instead of separate sets", func(p *VerificationPolicy) {
			p.TrustSets = []TrustSet{{Identities: []TrustedIdentity{identityA, identityB}, Keys: []string{"a.pub", "b.pub", "c.pub"}}}
		}},
	}
	for _, tt := range tests {
		t.Run("differs with "+tt.name, func(t *testing.T) {
			policy := VerificationPolicy{RequireSignature: true, TrustSets: []TrustSet{
				{Identities: []TrustedIdentity{identityA, identityB}, Keys: []string{"a.pub", "b.pub"}},
				{Keys: []string{"c.pub"}},
			}}
			tt.mutate(&policy)
			other, err := policy.Fingerprint()
			require.NoError(t, err)
			assert.NotEqual(t, fingerprint, other)
		})
	}
}

func TestHasTrustConstraints(t *testing.T) {
	assert.False(t, VerificationPolicy{}.HasTrustConstraints())
	assert.False(t, VerificationPolicy{RequireSignature: true, TrustSets: []TrustSet{{}}}.HasTrustConstraints())
	assert.True(t, VerificationPolicy{TrustSets: []TrustSet{{Keys: []string{"a.pub"}}}}.HasTrustConstraints())
	assert.True(t, VerificationPolicy{TrustSets: []TrustSet{{Identities: []TrustedIdentity{{Identity: "i", Issuer: "o"}}}}}.HasTrustConstraints())
}
