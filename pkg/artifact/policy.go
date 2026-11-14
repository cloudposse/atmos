package artifact

import (
	"fmt"
	"sort"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TrustedIdentity is a certificate identity and issuer pair that may satisfy a trust set.
type TrustedIdentity struct {
	// Identity is the expected certificate identity.
	Identity string `json:"identity"`

	// Issuer is the expected OIDC issuer.
	Issuer string `json:"issuer"`
}

// TrustSet is one set of acceptable signers. Within a trust set any one signer satisfies it.
type TrustSet struct {
	// Identities are trusted certificate identities.
	Identities []TrustedIdentity `json:"identities,omitempty"`

	// Keys are operator-managed public-key file paths.
	Keys []string `json:"keys,omitempty"`
}

// VerificationPolicy is the effective policy passed to Verify. When a global and a repository
// trust restriction both exist, the policy lists both TrustSets and every one must be satisfied;
// they are never unioned into a weaker policy.
type VerificationPolicy struct {
	// RequireSignature makes a missing or invalid signature a failure.
	RequireSignature bool `json:"require_signature"`

	// TrustSets are the sets that must each be satisfied by at least one valid permitted signature.
	TrustSets []TrustSet `json:"trust_sets,omitempty"`

	// Offline forbids network access; only preserved evidence and provisioned trust material are used.
	Offline bool `json:"offline"`
}

// HasTrustConstraints reports whether the policy supplies any trust material. Without it a
// verifier reports unchecked rather than verified.
func (p VerificationPolicy) HasTrustConstraints() bool {
	defer perf.Track(nil, "artifact.VerificationPolicy.HasTrustConstraints")()

	for _, set := range p.TrustSets {
		if len(set.Identities) > 0 || len(set.Keys) > 0 {
			return true
		}
	}
	return false
}

// Fingerprint returns a stable SHA-256 fingerprint of the policy, independent of the order of
// trust sets, identities, and keys, for recording in verification results.
func (p VerificationPolicy) Fingerprint() (string, error) {
	defer perf.Track(nil, "artifact.VerificationPolicy.Fingerprint")()

	normalized := VerificationPolicy{RequireSignature: p.RequireSignature, Offline: p.Offline}
	for _, set := range p.TrustSets {
		next := TrustSet{
			Identities: append([]TrustedIdentity(nil), set.Identities...),
			Keys:       append([]string(nil), set.Keys...),
		}
		sort.Slice(next.Identities, func(i, j int) bool {
			if next.Identities[i].Identity != next.Identities[j].Identity {
				return next.Identities[i].Identity < next.Identities[j].Identity
			}
			return next.Identities[i].Issuer < next.Identities[j].Issuer
		})
		sort.Strings(next.Keys)
		normalized.TrustSets = append(normalized.TrustSets, next)
	}
	sort.Slice(normalized.TrustSets, func(i, j int) bool {
		return trustSetKey(normalized.TrustSets[i]) < trustSetKey(normalized.TrustSets[j])
	})
	canonical, err := canonicalJSON(normalized)
	if err != nil {
		return "", fmt.Errorf("%w: policy fingerprint: %w", errUtils.ErrArtifactConfigInvalid, err)
	}
	return digest.FromBytes(canonical).String(), nil
}

// trustSetKey is a deterministic sort key for a trust set.
func trustSetKey(set TrustSet) string {
	key := ""
	for _, identity := range set.Identities {
		key += identity.Identity + "\x00" + identity.Issuer + "\x00"
	}
	for _, path := range set.Keys {
		key += path + "\x01"
	}
	return key
}
