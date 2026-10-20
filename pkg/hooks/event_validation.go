package hooks

import "github.com/cloudposse/atmos/pkg/hooks/events"

// ValidateEvents validates all declarations before any event/condition filtering.
func ValidateEvents(pin string, items map[string]Hook) error {
	declarations := make(map[string][]string, len(items))
	for name := range items {
		declarations[name] = items[name].Events
	}
	return events.DefaultRegistry.Validate(pin, declarations)
}
