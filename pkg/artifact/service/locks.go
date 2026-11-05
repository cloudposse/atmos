package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// lockTarget is one resource to lock in one repository.
type lockTarget struct {
	alias    string
	repo     artifact.Repository
	info     *RepositoryInfo
	resource string
}

// heldLock is an acquired lock together with where it came from.
type heldLock struct {
	target lockTarget
	handle artifact.LockHandle
}

// lockSet is a group of held locks released together in reverse acquisition order.
type lockSet struct {
	held []heldLock
}

// requiredScope returns the guarantee a repository requires, defaulting to host scope.
func requiredScope(info *RepositoryInfo) artifact.LockScope {
	if info.RequiredLockScope == "" {
		return artifact.LockScopeHost
	}
	return info.RequiredLockScope
}

// checkLockGuarantee rejects an insufficient lock guarantee before anything is mutated.
func checkLockGuarantee(info *RepositoryInfo, operation string) error {
	required := requiredScope(info)
	if info.Capabilities.HasLockScope(required) {
		return nil
	}
	return artifact.NewUnsupportedCapability(info.Alias, operation, "lock/"+string(required),
		fmt.Sprintf("the repository cannot provide %s-scope write coordination; configure locking.repository to delegate to a repository that can, or relax locking.required_scope", required))
}

// acquireLocks checks every guarantee first, then acquires the locks in canonical repository alias
// then resource order. A repository that delegates locking (locking.repository) is locked through
// its delegate, which is opened with its own identity. Any failure (including cancellation)
// releases whatever was already taken.
func (s *Service) acquireLocks(ctx context.Context, op *operation, operation string, targets []lockTarget) (*lockSet, error) {
	for index := range targets {
		if err := checkLockGuarantee(targets[index].info, operation); err != nil {
			return nil, err
		}
	}
	ordered := canonicalOrder(targets)
	set := &lockSet{}
	for index := range ordered {
		target := ordered[index]
		provider, resource, err := s.lockProviderFor(ctx, &target)
		if err != nil {
			releaseErr := set.release(ctx, op)
			return nil, errors.Join(err, releaseErr)
		}
		handle, err := provider.Lock(ctx, artifact.LockRequest{
			Resource:    resource,
			Owner:       s.owner,
			OperationID: op.id,
			Required:    requiredScope(target.info),
		})
		if err != nil {
			releaseErr := set.release(ctx, op)
			return nil, errors.Join(fmt.Errorf("lock %s in repository %q: %w", target.resource, target.alias, err), releaseErr)
		}
		set.held = append(set.held, heldLock{target: target, handle: handle})
	}
	return set, nil
}

// lockProviderFor returns the lock provider and lock resource for a target. A repository locks
// itself unless locking.repository delegates coordination, in which case the delegate repository is
// opened (resolving its own identity, never the target's) and the resource is scoped by the target
// collection, so unrelated collections that share one coordinator never contend. The delegate must
// advertise the required scope before anything is mutated.
func (s *Service) lockProviderFor(ctx context.Context, target *lockTarget) (artifact.LockProvider, string, error) {
	provider := target.info.LockProvider
	if provider == "" || provider == target.alias {
		return target.repo, target.resource, nil
	}
	delegateInfo, err := s.repos.Describe(provider)
	if err != nil {
		return nil, "", fmt.Errorf("lock delegate %q of repository %q: %w", provider, target.alias, err)
	}
	required := requiredScope(target.info)
	if !delegateInfo.Capabilities.HasLockScope(required) {
		return nil, "", artifact.NewUnsupportedCapability(provider, "lock", "lock/"+string(required),
			fmt.Sprintf("the lock delegate of repository %q cannot provide %s-scope coordination", target.alias, required))
	}
	delegate, err := s.repos.Open(ctx, provider)
	if err != nil {
		return nil, "", fmt.Errorf("lock delegate %q of repository %q: %w", provider, target.alias, err)
	}
	return delegate, lockNamespace(target) + "/" + target.resource, nil
}

// lockNamespace identifies the collection a lock protects: the repository's own namespace when it
// reports one, otherwise its alias.
func lockNamespace(target *lockTarget) string {
	if namespacer, ok := target.repo.(artifact.LockNamespacer); ok {
		if namespace := namespacer.LockNamespace(); namespace != "" {
			return namespace
		}
	}
	return target.alias
}

// canonicalOrder sorts targets by repository alias then resource and drops exact duplicates.
func canonicalOrder(targets []lockTarget) []lockTarget {
	ordered := append([]lockTarget(nil), targets...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].alias != ordered[j].alias {
			return ordered[i].alias < ordered[j].alias
		}
		return ordered[i].resource < ordered[j].resource
	})
	unique := ordered[:0]
	for index := range ordered {
		if index > 0 && ordered[index].alias == ordered[index-1].alias && ordered[index].resource == ordered[index-1].resource {
			continue
		}
		unique = append(unique, ordered[index])
	}
	return unique
}

// handleFor returns the lock held for the repository alias, or nil.
func (l *lockSet) handleFor(alias string) artifact.LockHandle {
	if l == nil {
		return nil
	}
	for index := range l.held {
		if l.held[index].target.alias == alias {
			return l.held[index].handle
		}
	}
	return nil
}

// scope returns the weakest actual scope across held locks, which results report.
func (l *lockSet) scope() artifact.LockScope {
	if l == nil || len(l.held) == 0 {
		return ""
	}
	weakest := l.held[0].handle.Scope()
	for index := range l.held {
		if scope := l.held[index].handle.Scope(); !scope.Satisfies(artifact.LockScopeRepository) {
			weakest = scope
		}
	}
	return weakest
}

// release relinquishes every held lock in reverse order. It uses a context detached from
// cancellation so a canceled operation never leaks ownership. A failed release is reported as a
// warning with the owner and resource an operator needs for recovery, and returned.
func (l *lockSet) release(ctx context.Context, op *operation) error {
	if l == nil {
		return nil
	}
	detached := context.WithoutCancel(ctx)
	var errs []error
	for index := len(l.held) - 1; index >= 0; index-- {
		held := l.held[index]
		if err := held.handle.Release(detached); err != nil {
			op.warn("lock-release-failed", fmt.Sprintf("repository %q resource %q owner %q: operator recovery may be required: %v", held.target.alias, held.handle.Resource(), held.handle.Owner(), err))
			errs = append(errs, fmt.Errorf("release lock %s in repository %q: %w", held.handle.Resource(), held.target.alias, err))
		}
	}
	l.held = nil
	return errors.Join(errs...)
}
