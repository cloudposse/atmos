package lifecycle

import (
	"fmt"
	"slices"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Event is one of the supported stack lifecycle events. A component type opts in
// to an event by registering its handler; other command names imply no support.
type Event string

const (
	PlanEvent Event = "plan"
	Build     Event = "build"
	Test      Event = "test"
	Deploy    Event = "deploy"
	Destroy   Event = "destroy"
	Validate  Event = "validate"
)

func (e Event) valid() bool {
	switch e {
	case PlanEvent, Build, Test, Deploy, Destroy, Validate:
		return true
	default:
		return false
	}
}

// Registry associates a component type and a fixed lifecycle event with an
// isolated-command handler. Custom configuration is merged into a per-invocation
// snapshot, so loading one project's commands cannot affect another project.
type Registry struct {
	mu       sync.RWMutex
	handlers map[Event]map[string]*Handler
}

// DefaultRegistry contains handlers registered by the component command packages.
var DefaultRegistry = NewRegistry()

// NewRegistry creates an independent registry, useful for hosts and tests.
func NewRegistry() *Registry {
	defer perf.Track(nil, "lifecycle.NewRegistry")()

	return &Registry{handlers: make(map[Event]map[string]*Handler)}
}

// Register opts a component type into an event. Duplicate handlers are rejected
// rather than allowing package initialization order to choose an implementation.
func (r *Registry) Register(kind string, event Event, handler *Handler) error {
	defer perf.Track(nil, "lifecycle.Registry.Register")()
	if kind == "" || !event.valid() || handler == nil || len(handler.Path) == 0 {
		return fmt.Errorf("%w: invalid lifecycle registration for %q/%q", errUtils.ErrInvalidConfig, kind, event)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(map[Event]map[string]*Handler)
	}
	if r.handlers[event] == nil {
		r.handlers[event] = make(map[string]*Handler)
	}
	if _, exists := r.handlers[event][kind]; exists {
		return fmt.Errorf("%w: duplicate lifecycle handler for %s/%s", errUtils.ErrInvalidConfig, kind, event)
	}
	r.handlers[event][kind] = cloneHandler(handler)
	return nil
}

// MustRegister registers a built-in handler during package initialization.
func MustRegister(kind string, event Event, handler *Handler) {
	defer perf.Track(nil, "lifecycle.MustRegister")()
	if err := DefaultRegistry.Register(kind, event, handler); err != nil {
		panic(err)
	}
}

// Handlers returns an independent snapshot of types supporting the given event.
func (r *Registry) Handlers(event Event) map[string]*Handler {
	defer perf.Track(nil, "lifecycle.Registry.Handlers")()
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*Handler, len(r.handlers[event]))
	for kind, h := range r.handlers[event] {
		result[kind] = cloneHandler(h)
	}
	return result
}

// Lookup reports whether a component type supports an event and returns its handler.
func (r *Registry) Lookup(kind string, event Event) (*Handler, bool) {
	defer perf.Track(nil, "lifecycle.Registry.Lookup")()
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[event][kind]
	return cloneHandler(handler), ok
}

func cloneHandler(source *Handler) *Handler {
	if source == nil {
		return nil
	}
	h := *source
	h.Path = slices.Clone(h.Path)
	h.Args = slices.Clone(h.Args)
	h.Flags = slices.Clone(h.Flags)
	for i := range h.Args {
		h.Args[i].Values = slices.Clone(h.Args[i].Values)
	}
	for i := range h.Flags {
		h.Flags[i].Values = slices.Clone(h.Flags[i].Values)
	}
	h.HookEvents = slices.Clone(h.HookEvents)
	h.ApprovalArgs = slices.Clone(h.ApprovalArgs)
	return &h
}
