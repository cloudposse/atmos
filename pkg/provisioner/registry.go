package provisioner

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HookEvent represents when a provisioner should run.
// Using string type to avoid circular dependency with pkg/hooks.
type HookEvent string

// ProvisionerFunc is a function that provisions infrastructure.
// It receives the Atmos configuration, component configuration, and auth context.
// Returns an error if provisioning fails.
type ProvisionerFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error

// Provisioner represents a self-registering provisioner.
type Provisioner struct {
	// Type is the provisioner type (e.g., "backend", "component").
	Type string

	// HookEvent declares when this provisioner should run.
	HookEvent HookEvent

	// Func is the provisioning function to execute.
	Func ProvisionerFunc
}

var (
	// ProvisionersByEvent stores provisioners indexed by hook event.
	provisionersByEvent = make(map[HookEvent][]Provisioner)
	registryMu          sync.RWMutex
)

// RegisterProvisioner registers a provisioner for a specific hook event.
// Provisioners self-declare when they should run by specifying a hook event.
func RegisterProvisioner(p Provisioner) {
	defer perf.Track(nil, "provisioner.RegisterProvisioner")()

	registryMu.Lock()
	defer registryMu.Unlock()

	provisionersByEvent[p.HookEvent] = append(provisionersByEvent[p.HookEvent], p)
}

// GetProvisionersForEvent returns all provisioners registered for a specific hook event.
func GetProvisionersForEvent(event HookEvent) []Provisioner {
	defer perf.Track(nil, "provisioner.GetProvisionersForEvent")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	provisioners, ok := provisionersByEvent[event]
	if !ok {
		return nil
	}

	// Return a copy to prevent external modification.
	result := make([]Provisioner, len(provisioners))
	copy(result, provisioners)
	return result
}

// ExecuteProvisioners executes all provisioners registered for a specific hook event.
// Returns an error if any provisioner fails (fail-fast behavior).
func ExecuteProvisioners(
	ctx context.Context,
	event HookEvent,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "provisioner.ExecuteProvisioners")()

	provisioners := GetProvisionersForEvent(event)
	if len(provisioners) == 0 {
		return nil
	}

	for _, p := range provisioners {
		if err := p.Func(ctx, atmosConfig, componentConfig, authContext); err != nil {
			return fmt.Errorf("provisioner %s failed: %w", p.Type, err)
		}
	}

	return nil
}
