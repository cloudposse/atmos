// Package validate defines the extension point for native CI validators.
package validate

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/validation"
)

// Request identifies the repository and optional workflow selection a validator
// should check. An empty Paths slice and WorkflowPath requests the validator's
// repository-wide default discovery behavior.
type Request struct {
	Root         string
	Paths        []string
	WorkflowPath string
}

// Validator runs one kind of CI validation and returns provider-neutral
// diagnostics. New validators can register themselves without changing the
// command layer.
type Validator interface {
	Name() string
	Validate(ctx context.Context, request Request) (validation.Report, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Validator{}
)

// Register makes a validator available to CI commands. It panics for an
// invalid registration because registrations occur during package setup and a
// duplicate would otherwise make command behavior non-deterministic.
func Register(validator Validator) {
	if validator == nil {
		panic("ci validator must not be nil")
	}
	name := validator.Name()
	if name == "" {
		panic("ci validator name must not be empty")
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("ci validator %q already registered", name))
	}
	registry[name] = validator
}

// Get returns a registered validator by name.
func Get(name string) (Validator, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	validator, ok := registry[name]
	return validator, ok
}

// Names returns registered validator names in stable order.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
