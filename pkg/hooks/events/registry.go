package events

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
)

// Registry holds the event names emitted by lifecycle integrations. Register
// integrations before loading configuration; declaring a hook does not register
// its event. A registry can be shared by concurrent stack readers.
type Registry struct {
	mu    sync.RWMutex
	names map[HookEvent]struct{}
}

// NewRegistry creates an isolated registry with the supplied event names.
func NewRegistry(names ...HookEvent) *Registry {
	r := &Registry{names: make(map[HookEvent]struct{})}
	r.Register(names...)
	return r
}

// Register adds events emitted by an integration. Repeated registration is safe.
func (r *Registry) Register(names ...HookEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		r.names[name] = struct{}{}
	}
}

// DefaultRegistry contains the built-in events and allows integrations to
// register additional events without making arbitrary configuration strings valid.
var DefaultRegistry = NewRegistry(builtins()...)

// Names returns a sorted copy of the registered event names.
func (r *Registry) Names() []HookEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]HookEvent, 0, len(r.names))
	for name := range r.names {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Contains recognizes exact event names and the legacy hyphen-separated spelling
// accepted by Hook.MatchesEvent. Exact lookup preserves hyphens in operation names.
func (r *Registry) Contains(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exact := r.names[HookEvent(name)]
	_, legacy := r.names[HookEvent(strings.ReplaceAll(name, "-", "."))]
	return exact || legacy
}

// Validate checks every declared event, regardless of matching, conditions, or
// skip flags. An empty events list still matches all events. Earlier editions
// preserve permissive names; invalid pins always propagate an error.
func (r *Registry) Validate(pin string, declarations map[string][]string) error {
	strict, err := edition.BehaviorChanged(pin, "hooks.registered_events")
	if err != nil || !strict {
		return err
	}
	names := make([]string, 0, len(declarations))
	for name := range declarations {
		names = append(names, name)
	}
	slices.Sort(names)
	var failures []error
	for _, name := range names {
		for _, event := range declarations[name] {
			if !r.Contains(event) {
				failures = append(failures, fmt.Errorf("%w: hook %q declares %q; use a registered lifecycle event (see https://atmos.tools/stacks/hooks)", errUtils.ErrUnsupportedHookEvent, name, event))
			}
		}
	}
	return errors.Join(failures...)
}

// ValidateSection checks event declarations in a raw hooks block without
// depending on hook engines. Structural errors remain the responsibility of
// the existing scope-specific schema validators and hook decoders.
func ValidateSection(pin string, section any) error {
	strict, err := edition.BehaviorChanged(pin, "hooks.registered_events")
	if err != nil || !strict || section == nil {
		return err
	}
	hooks, _ := section.(map[string]any)
	declarations := make(map[string][]string, len(hooks))
	for _, name := range slices.Sorted(maps.Keys(hooks)) {
		hook, _ := hooks[name].(map[string]any)
		switch list := hook["events"].(type) {
		case []string:
			declarations[name] = list
		case []any:
			for _, item := range list {
				event, ok := item.(string)
				if !ok {
					return fmt.Errorf("%w: hook %q events must be strings", errUtils.ErrInvalidHooksSection, name)
				}
				declarations[name] = append(declarations[name], event)
			}
		}
	}
	return DefaultRegistry.Validate(pin, declarations)
}
