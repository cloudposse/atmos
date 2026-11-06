package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validateVerification checks one verification policy level (global or repository).
func validateVerification(scope string, policy *schema.ArtifactVerificationConfig) error {
	if policy == nil {
		return nil
	}
	var errs []error
	for index, trusted := range policy.TrustedIdentities {
		if err := validateTrustedIdentity(trusted); err != nil {
			errs = append(errs, fmt.Errorf("trusted_identities[%d]: %w", index, err))
		}
	}
	for index, key := range policy.TrustedKeys {
		if err := validateTrustedKey(policy, key); err != nil {
			errs = append(errs, fmt.Errorf("trusted_keys[%d]: %w", index, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%s: %w", scope, err)
	}
	return nil
}

func validateTrustedIdentity(trusted schema.ArtifactTrustedIdentity) error {
	var errs []error
	if strings.TrimSpace(trusted.Identity) == "" || strings.IndexFunc(trusted.Identity, unicode.IsControl) >= 0 {
		errs = append(errs, fmt.Errorf("%w: identity is required and must not contain control characters", errUtils.ErrArtifactConfigInvalid))
	}
	issuer, err := url.Parse(trusted.Issuer)
	if trusted.Issuer == "" || err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil {
		errs = append(errs, fmt.Errorf("%w: issuer %q must be an https URL without credentials", errUtils.ErrArtifactConfigInvalid, trusted.Issuer))
	}
	return errors.Join(errs...)
}

func validateTrustedKey(policy *schema.ArtifactVerificationConfig, key string) error {
	if key == "" || strings.ContainsRune(key, 0) {
		return fmt.Errorf("%w: key path must be a non-empty file path", errUtils.ErrArtifactConfigInvalid)
	}
	if _, err := AnchorPath(policy.KeysOrigin, key); err != nil {
		return err
	}
	return nil
}

// validateWeakening rejects a repository that tries to relax the global policy.
func validateWeakening(global, repo *schema.ArtifactVerificationConfig) error {
	if global == nil || repo == nil || global.RequireSignature == nil || repo.RequireSignature == nil {
		return nil
	}
	if *global.RequireSignature && !*repo.RequireSignature {
		return fmt.Errorf("%w: verification.require_signature: false cannot weaken the global artifacts.verification.require_signature: true", errUtils.ErrArtifactConfigInvalid)
	}
	return nil
}

// EffectivePolicy returns the verification policy that applies to a repository.
//
// A global requirement can be strengthened but never weakened: RequireSignature is true when
// either level requires it. Within one trust set any configured signer satisfies it; when both
// the global and the repository define trust (identities or keys), the policy carries two trust
// sets and both must be satisfied, so a repository can narrow trust but never widen it. The
// global trust set comes first.
func EffectivePolicy(cfg *schema.ArtifactConfig, alias string) (artifact.VerificationPolicy, error) {
	defer perf.Track(nil, "artifactconfig.EffectivePolicy")()

	repo, ok := cfg.Repositories[alias]
	if !ok {
		return artifact.VerificationPolicy{}, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
	}
	return CombinePolicy(cfg.Verification, repo.Verification), nil
}

// CombinePolicy combines a global and a repository policy level. Either may be nil.
func CombinePolicy(global, repo *schema.ArtifactVerificationConfig) artifact.VerificationPolicy {
	defer perf.Track(nil, "artifactconfig.CombinePolicy")()

	var policy artifact.VerificationPolicy
	for _, level := range []*schema.ArtifactVerificationConfig{global, repo} {
		if level == nil {
			continue
		}
		if level.RequireSignature != nil && *level.RequireSignature {
			policy.RequireSignature = true
		}
		if set := trustSetOf(level); len(set.Identities) > 0 || len(set.Keys) > 0 {
			policy.TrustSets = append(policy.TrustSets, set)
		}
	}
	return policy
}

// trustSetOf converts one policy level to a trust set, copying slices so the policy never aliases configuration.
func trustSetOf(level *schema.ArtifactVerificationConfig) artifact.TrustSet {
	var set artifact.TrustSet
	for _, trusted := range level.TrustedIdentities {
		set.Identities = append(set.Identities, artifact.TrustedIdentity{Identity: trusted.Identity, Issuer: trusted.Issuer})
	}
	set.Keys = append(set.Keys, level.TrustedKeys...)
	return set
}
