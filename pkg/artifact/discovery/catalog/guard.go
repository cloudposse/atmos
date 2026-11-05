package catalog

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// guard is the repository coordinator held around a read-latest, modify, publish cycle. A guard
// without a handle means the repository offers no coordinator: writers must then be serialized
// externally, which the catalog documentation states.
type guard struct {
	lock artifact.LockHandle
}

func (g *guard) handle() artifact.LockHandle { return g.lock }

func (g *guard) scope() artifact.LockScope {
	if g.lock == nil {
		return ""
	}
	return g.lock.Scope()
}

// release relinquishes the lock with a context detached from cancellation, so an interrupted
// command never leaks ownership.
func (g *guard) release(ctx context.Context) error {
	if g.lock == nil {
		return nil
	}
	if err := g.lock.Release(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("release catalog lock %s: operator recovery may be required: %w", g.lock.Resource(), err)
	}
	return nil
}

// acquire takes the repository's coordinator when it advertises a scope that satisfies the
// required one. A repository without a coordinator proceeds unguarded; one that has a coordinator
// but cannot grant the lock fails the write instead of proceeding unserialized.
func (s *Service) acquire(ctx context.Context, t *target, repo artifact.Repository, operationID string) (*guard, error) {
	required := t.info.RequiredLockScope
	if required == "" {
		required = artifact.LockScopeHost
	}
	if !t.info.Capabilities.HasLockScope(required) {
		return &guard{}, nil
	}
	provider, resource, err := s.provider(ctx, t, repo)
	if err != nil {
		return nil, err
	}
	handle, err := provider.Lock(ctx, artifact.LockRequest{Resource: resource, Owner: s.owner, OperationID: operationID, Required: required})
	if err != nil {
		return nil, fmt.Errorf("lock the catalog %s of repository %q: %w", t.ref, t.alias, err)
	}
	return &guard{lock: handle}, nil
}

// provider returns the lock provider and the resource it locks. A repository that delegates
// coordination (locking.repository) is locked through its delegate, opened with its own identity,
// with the resource namespaced by the collection so unrelated collections sharing a coordinator
// never contend.
func (s *Service) provider(ctx context.Context, t *target, repo artifact.Repository) (artifact.LockProvider, string, error) {
	resource := "catalog/" + t.ref.String()
	if t.info.LockProvider == "" || t.info.LockProvider == t.alias {
		return repo, resource, nil
	}
	delegate, err := s.repos.Open(ctx, t.info.LockProvider)
	if err != nil {
		return nil, "", fmt.Errorf("lock delegate %q of repository %q: %w", t.info.LockProvider, t.alias, err)
	}
	namespace := t.alias
	if namespacer, ok := repo.(artifact.LockNamespacer); ok && namespacer.LockNamespace() != "" {
		namespace = namespacer.LockNamespace()
	}
	return delegate, namespace + "/" + resource, nil
}
