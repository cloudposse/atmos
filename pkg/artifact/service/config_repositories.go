package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
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
	opened   map[string]*openedRepository
}

// An openedRepository is the construction state of one alias. Its done channel closes when
// construction ends, and the repository and error are valid after that. The shared mutex guards only
// the map of these entries, never a construction: building a repository may resolve credentials and
// wait on a user.
type openedRepository struct {
	done chan struct{}
	repo artifact.Repository
	err  error
}

var _ Repositories = (*ConfigRepositories)(nil)

// FromConfig returns a [Repositories] backed by configuration. A nil registry uses the default
// repository registry.
func FromConfig(cfg *schema.ArtifactConfig, registry *artifact.RepositoryRegistry, opts ...artifact.RepositoryOption) *ConfigRepositories {
	defer perf.Track(nil, "service.FromConfig")()

	if registry == nil {
		registry = artifact.DefaultRepositoryRegistry()
	}
	return &ConfigRepositories{cfg: cfg, registry: registry, options: opts, opened: map[string]*openedRepository{}}
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
		Capabilities:      desc.Capabilities.Clone(),
		Packaging:         desc.EffectivePackaging(repoCfg.Packaging),
		RequiredLockScope: plan.Required,
		Policy:            policy,
	}
	if repoCfg.Defaults != nil && repoCfg.Defaults.Metadata != nil {
		info.Defaults = artifact.Metadata{Description: repoCfg.Defaults.Metadata.Description, Labels: maps.Clone(repoCfg.Defaults.Metadata.Labels)}
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

// Open implements [Repositories]. Each repository is constructed once, and constructing one does not
// block opening another: callers waiting for the same alias share its construction and stop waiting
// when their own context ends. A failed construction is not cached, so a later Open retries it.
func (c *ConfigRepositories) Open(ctx context.Context, alias string) (artifact.Repository, error) {
	defer perf.Track(nil, "service.ConfigRepositories.Open")()

	for {
		c.mu.Lock()
		if entry, ok := c.opened[alias]; ok {
			c.mu.Unlock()
			repo, retry, err := c.await(ctx, entry)
			if retry {
				continue
			}
			return repo, err
		}
		repoCfg, ok := c.cfg.Repositories[alias]
		if !ok {
			c.mu.Unlock()
			return nil, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
		}
		entry := &openedRepository{done: make(chan struct{})}
		c.opened[alias] = entry
		c.mu.Unlock()
		return c.construct(ctx, alias, &repoCfg, entry)
	}
}

// construct builds the repository outside the shared mutex and publishes the outcome to every caller
// waiting on entry. A failure removes the entry so a later Open can try again.
func (c *ConfigRepositories) construct(ctx context.Context, alias string, repoCfg *schema.ArtifactRepositoryConfig, entry *openedRepository) (artifact.Repository, error) {
	entry.repo, entry.err = c.registry.New(ctx, alias, repoCfg, c.options...)
	if entry.err != nil {
		c.mu.Lock()
		if c.opened[alias] == entry {
			delete(c.opened, alias)
		}
		c.mu.Unlock()
	}
	close(entry.done)
	return entry.repo, entry.err
}

// await waits for another caller's construction of entry, or for ctx to end. It reports retry when that
// construction ended because the other caller's own context did, which says nothing about this caller.
func (c *ConfigRepositories) await(ctx context.Context, entry *openedRepository) (artifact.Repository, bool, error) {
	select {
	case <-entry.done:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	if entry.err != nil && ctx.Err() == nil && (errors.Is(entry.err, context.Canceled) || errors.Is(entry.err, context.DeadlineExceeded)) {
		return nil, true, nil
	}
	return entry.repo, false, entry.err
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
