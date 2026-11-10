package service

import (
	"context"
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	artifactconfig "github.com/cloudposse/atmos/pkg/artifact/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfigRepositories implements [Repositories] from the validated artifacts configuration and a
// repository kind registry. Describe reads only configuration and kind descriptors, so it never
// authenticates; Open constructs each repository once through the registry, which is where
// identities are resolved.
type ConfigRepositories struct {
	cfg      *schema.ArtifactConfig
	registry *artifact.RepositoryRegistry
	options  []artifact.RepositoryOption
	mu       sync.Mutex
	opened   map[string]artifact.Repository
}

var _ Repositories = (*ConfigRepositories)(nil)

// FromConfig returns a [Repositories] backed by configuration. A nil registry uses the default
// repository registry.
func FromConfig(cfg *schema.ArtifactConfig, registry *artifact.RepositoryRegistry, opts ...artifact.RepositoryOption) *ConfigRepositories {
	defer perf.Track(nil, "service.FromConfig")()

	if registry == nil {
		registry = artifact.DefaultRepositoryRegistry()
	}
	return &ConfigRepositories{cfg: cfg, registry: registry, options: opts, opened: map[string]artifact.Repository{}}
}

// Describe implements [Repositories]. For a repository that delegates locking, the lock scopes of
// the delegate are added to its capabilities, so the lock guarantee check sees what the delegate
// can provide.
func (c *ConfigRepositories) Describe(alias string) (RepositoryInfo, error) {
	defer perf.Track(nil, "service.ConfigRepositories.Describe")()

	repoCfg, ok := c.cfg.Repositories[alias]
	if !ok {
		return RepositoryInfo{}, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
	}
	desc, known := c.registry.Lookup(repoCfg.Kind)
	if !known {
		return RepositoryInfo{}, fmt.Errorf("%w: repository %q has unknown kind %q", errUtils.ErrArtifactConfigInvalid, alias, repoCfg.Kind)
	}
	plan, err := artifactconfig.ResolveLockPlan(c.cfg, alias)
	if err != nil {
		return RepositoryInfo{}, err
	}
	policy, err := artifactconfig.EffectivePolicy(c.cfg, alias)
	if err != nil {
		return RepositoryInfo{}, err
	}
	info := RepositoryInfo{
		Alias:             alias,
		Capabilities:      desc.Capabilities,
		Packaging:         desc.EffectivePackaging(repoCfg.Packaging),
		RequiredLockScope: plan.Required,
		Policy:            policy,
	}
	if repoCfg.Defaults != nil && repoCfg.Defaults.Metadata != nil {
		info.Defaults = artifact.Metadata{Description: repoCfg.Defaults.Metadata.Description, Labels: repoCfg.Defaults.Metadata.Labels}
	}
	if plan.Provider != alias {
		info.LockProvider = plan.Provider
		info.Capabilities.LockScopes = c.delegatedScopes(&info.Capabilities, plan.Provider)
	}
	return info, nil
}

// delegatedScopes adds the delegate repository's lock scopes to the repository's own.
func (c *ConfigRepositories) delegatedScopes(caps *artifact.Capabilities, provider string) []artifact.LockScope {
	scopes := append([]artifact.LockScope(nil), caps.LockScopes...)
	delegate, ok := c.cfg.Repositories[provider]
	if !ok {
		return scopes
	}
	desc, known := c.registry.Lookup(delegate.Kind)
	if !known {
		return scopes
	}
	for _, scope := range desc.Capabilities.LockScopes {
		if !containsScope(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func containsScope(scopes []artifact.LockScope, scope artifact.LockScope) bool {
	for _, have := range scopes {
		if have == scope {
			return true
		}
	}
	return false
}

// Open implements [Repositories]. Each repository is constructed once.
func (c *ConfigRepositories) Open(ctx context.Context, alias string) (artifact.Repository, error) {
	defer perf.Track(nil, "service.ConfigRepositories.Open")()

	c.mu.Lock()
	defer c.mu.Unlock()
	if repo, ok := c.opened[alias]; ok {
		return repo, nil
	}
	repoCfg, ok := c.cfg.Repositories[alias]
	if !ok {
		return nil, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
	}
	repo, err := c.registry.New(ctx, alias, &repoCfg, c.options...)
	if err != nil {
		return nil, err
	}
	c.opened[alias] = repo
	return repo, nil
}

// Aliases implements [Repositories].
func (c *ConfigRepositories) Aliases() []string {
	defer perf.Track(nil, "service.ConfigRepositories.Aliases")()

	aliases := make([]string, 0, len(c.cfg.Repositories))
	for alias := range c.cfg.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
