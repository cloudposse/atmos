// Package background supervises workflow steps marked `background: true`: a step
// starts and the workflow continues while Atmos keeps it alive, later waiting for
// it to become ready (`wait`/`wait-all`) or tearing it down (`cancel`).
//
// The package is intentionally generic. v1 ships a single container-backed Runner
// (the container runtime is the supervisor, so no goroutine is needed); a future
// shell/process Runner can implement the same Runner/Handle interfaces — backed by
// a goroutine and a readiness probe — without changing the workflow orchestration.
package background

import (
	"context"
	stderrors "errors"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Handle supervises a single running background step.
type Handle interface {
	// Name is the step name the background step was registered under.
	Name() string
	// WaitReady blocks until the background step is ready to use — for a container
	// service, until its health check passes — or returns immediately when no
	// readiness is configured. A service that reports terminally unhealthy fails fast.
	WaitReady(ctx context.Context) error
	// Stop gracefully tears the background step down (for a container, stop+remove).
	Stop(ctx context.Context) error
}

// Runner starts a background step and returns a Handle to supervise it.
type Runner interface {
	Start(ctx context.Context, step *schema.WorkflowStep, env []string) (Handle, error)
}

// Registry tracks the background steps started during one workflow run, keyed by
// step name. It is safe for concurrent use.
type Registry struct {
	mu      sync.Mutex
	handles map[string]Handle
	order   []string
}

// NewRegistry returns an empty run-scoped registry.
func NewRegistry() *Registry {
	defer perf.Track(nil, "background.NewRegistry")()

	return &Registry{handles: make(map[string]Handle)}
}

// Register records a started handle under its name, preserving first-seen order.
func (r *Registry) Register(h Handle) {
	defer perf.Track(nil, "background.Registry.Register")()

	r.mu.Lock()
	defer r.mu.Unlock()
	name := h.Name()
	if _, exists := r.handles[name]; !exists {
		r.order = append(r.order, name)
	}
	r.handles[name] = h
}

// Get returns the handle registered under name.
func (r *Registry) Get(name string) (Handle, bool) {
	defer perf.Track(nil, "background.Registry.Get")()

	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.handles[name]
	return h, ok
}

// Remove drops a handle from the registry (after it has been stopped).
func (r *Registry) Remove(name string) {
	defer perf.Track(nil, "background.Registry.Remove")()

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handles, name)
}

// Names returns the still-registered background step names in registration order.
func (r *Registry) Names() []string {
	defer perf.Track(nil, "background.Registry.Names")()

	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.handles))
	for _, name := range r.order {
		if _, ok := r.handles[name]; ok {
			names = append(names, name)
		}
	}
	return names
}

// StopAll tears down every still-registered background step — the implicit
// auto-teardown at the end of a scope. It attempts every handle and joins errors so
// a single failure does not leak the rest.
func (r *Registry) StopAll(ctx context.Context) error {
	defer perf.Track(nil, "background.Registry.StopAll")()

	var errs []error
	for _, name := range r.Names() {
		h, ok := r.Get(name)
		if !ok {
			continue
		}
		if err := h.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
		r.Remove(name)
	}
	return stderrors.Join(errs...)
}
