package config

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LockPlan is the resolved lock coordination of one repository.
type LockPlan struct {
	// Provider is the alias of the repository that provides the lock. It is the repository itself
	// unless locking is delegated.
	Provider string

	// Required is the minimum scope mutations need.
	Required artifact.LockScope

	// Chain lists the aliases visited from the repository to the provider, in order.
	Chain []string
}

// ResolveLockPlan follows `locking.repository` delegation from alias to the repository that
// provides the lock and reports the required scope. It fails on an unknown delegate, on a cycle,
// and on an invalid scope. It does not check the provider's capability; [Validator.Validate] does.
func ResolveLockPlan(cfg *schema.ArtifactConfig, alias string) (LockPlan, error) {
	defer perf.Track(nil, "artifactconfig.ResolveLockPlan")()

	repo, ok := cfg.Repositories[alias]
	if !ok {
		return LockPlan{}, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
	}
	required, err := requiredScope(repo.Locking)
	if err != nil {
		return LockPlan{}, err
	}
	plan := LockPlan{Provider: alias, Required: required, Chain: []string{alias}}
	visited := map[string]bool{alias: true}
	for {
		current := cfg.Repositories[plan.Provider]
		if current.Locking == nil || current.Locking.Repository == "" {
			return plan, nil
		}
		next := current.Locking.Repository
		if _, exists := cfg.Repositories[next]; !exists {
			return LockPlan{}, fmt.Errorf("%w: repository %q delegates locking to %q, which is not configured", errUtils.ErrArtifactConfigInvalid, plan.Provider, next)
		}
		if visited[next] {
			return LockPlan{}, fmt.Errorf("%w: lock delegation cycle: %s -> %s", errUtils.ErrArtifactConfigInvalid, strings.Join(plan.Chain, " -> "), next)
		}
		visited[next] = true
		plan.Chain = append(plan.Chain, next)
		plan.Provider = next
	}
}

// requiredScope returns the configured scope, defaulting to host.
func requiredScope(locking *schema.ArtifactLockingConfig) (artifact.LockScope, error) {
	if locking == nil || locking.RequiredScope == "" {
		return artifact.LockScopeHost, nil
	}
	switch scope := artifact.LockScope(locking.RequiredScope); scope {
	case artifact.LockScopeHost, artifact.LockScopeRepository:
		return scope, nil
	default:
		return "", fmt.Errorf("%w: locking.required_scope %q must be %q or %q", errUtils.ErrArtifactConfigInvalid, locking.RequiredScope, artifact.LockScopeHost, artifact.LockScopeRepository)
	}
}

// validateLocking checks the lock configuration: scope value, delegate existence, delegation
// cycles, and that the repository that finally provides the lock advertises the required scope.
func validateLocking(v *Validator, cfg *schema.ArtifactConfig, alias string, repo *schema.ArtifactRepositoryConfig) error {
	if repo.Locking == nil {
		return nil
	}
	plan, err := ResolveLockPlan(cfg, alias)
	if err != nil {
		return err
	}
	provider := cfg.Repositories[plan.Provider]
	desc, ok := v.registry.Lookup(provider.Kind)
	if !ok {
		// The provider's own unknown kind is reported when that repository is validated.
		return nil
	}
	if desc.Capabilities.HasLockScope(plan.Required) {
		return nil
	}
	return errors.Join(
		errUtils.ErrArtifactConfigInvalid,
		fmt.Errorf("%w: locking.required_scope %q is not provided by %q (kind %q advertises %v)%s",
			errUtils.ErrArtifactUnsupportedCapability, plan.Required, plan.Provider, provider.Kind, desc.Capabilities.LockScopes, delegateHint(plan)),
	)
}

func delegateHint(plan LockPlan) string {
	if len(plan.Chain) > 1 {
		return ""
	}
	return "; set locking.repository to a repository (for example an aws/s3 coordinator) that advertises repository scope"
}
