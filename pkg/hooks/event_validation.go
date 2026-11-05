package hooks

import (
	"github.com/cloudposse/atmos/pkg/hooks/events"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validateEvents resolves the edition using the same fallback as policy hooks.
func (h *Hooks) validateEvents(config *schema.AtmosConfiguration) error {
	if config == nil {
		config = h.config
	}
	var pin string
	if config != nil {
		pin = config.Edition
	}
	return ValidateEvents(pin, h.items)
}

// ValidateEvents validates all declarations before any event/condition filtering.
func ValidateEvents(pin string, items map[string]Hook) error {
	declarations := make(map[string][]string, len(items))
	for name := range items {
		declarations[name] = items[name].Events
	}
	return events.DefaultRegistry.Validate(pin, declarations)
}
