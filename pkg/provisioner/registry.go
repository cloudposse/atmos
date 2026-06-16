package provisioner

import (
	"context"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HookEvent represents when a provisioner should run.
// This is a string type alias compatible with pkg/hooks.HookEvent to avoid circular dependencies.
// Use pkg/hooks.HookEvent constants (e.g., hooks.BeforeTerraformInit) when registering provisioners.
type HookEvent string

// TerraformExecContext carries the live execution environment for provisioners that
// must run a terraform/tofu subcommand against the same working directory, binary, RC,
// and environment as the triggering command (e.g. the post-init providers-lock hook).
// It is nil for events where no subprocess context is available (e.g. before.terraform.init,
// whose provisioners use in-process SDKs). It lives here, with the runner closure built in
// the exec layer, so pkg/provisioner does not need to import internal/exec (a cycle).
type TerraformExecContext struct {
	// Run executes a terraform/tofu subcommand (args after the binary, e.g.
	// {"providers","lock","-platform=linux_amd64"}) with the live env, RC, and workdir.
	Run func(args []string) error
	// WorkingDir is the resolved component/workdir path the subcommand runs in.
	WorkingDir string
}

// ProvisionerFunc is a function that provisions infrastructure.
// It receives the Atmos configuration, component configuration, auth context, and an
// optional terraform execution context (nil unless the dispatching event provides one).
// Returns an error if provisioning fails.
type ProvisionerFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
	execCtx *TerraformExecContext,
) error

// Provisioner represents a self-registering provisioner.
// All fields are validated at registration time by RegisterProvisioner.
type Provisioner struct {
	// Type is the provisioner type (e.g., "backend", "component").
	Type string

	// HookEvent declares when this provisioner should run.
	// Must not be empty; use pkg/hooks.HookEvent constants.
	HookEvent HookEvent

	// Func is the provisioning function to execute.
	// Must not be nil.
	Func ProvisionerFunc
}

var (
	// ProvisionersByEvent stores provisioners indexed by hook event.
	provisionersByEvent = make(map[HookEvent][]Provisioner)
	registryMu          sync.RWMutex
)

// RegisterProvisioner registers a provisioner for a specific hook event.
// Provisioners self-declare when they should run by specifying a hook event.
// Returns an error if Func is nil or HookEvent is empty.
func RegisterProvisioner(p Provisioner) error {
	defer perf.Track(nil, "provisioner.RegisterProvisioner")()

	// Validate provisioner at registration time to catch configuration errors early.
	if p.Func == nil {
		return fmt.Errorf("%w: provisioner %q has nil Func", errUtils.ErrNilParam, p.Type)
	}
	if p.HookEvent == "" {
		return fmt.Errorf("%w: provisioner %q has empty HookEvent", errUtils.ErrInvalidConfig, p.Type)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	provisionersByEvent[p.HookEvent] = append(provisionersByEvent[p.HookEvent], p)
	return nil
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
//
// The execCtx is optional: events that run a terraform subcommand (e.g. after.terraform.init)
// pass a single *TerraformExecContext; before-events and callers without one pass nothing.
// It is variadic so the many existing call sites that have no execution context need no change.
func ExecuteProvisioners(
	ctx context.Context,
	event HookEvent,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
	execCtx ...*TerraformExecContext,
) error {
	defer perf.Track(atmosConfig, "provisioner.ExecuteProvisioners")()

	var ec *TerraformExecContext
	if len(execCtx) > 0 {
		ec = execCtx[0]
	}

	provisioners := GetProvisionersForEvent(event)
	if len(provisioners) == 0 {
		return nil
	}

	for _, p := range provisioners {
		// Defensive check: validation should happen at registration time,
		// but guard against invalid entries that may have been added directly to the registry.
		if p.Func == nil {
			return errUtils.Build(errUtils.ErrProvisionerFailed).
				WithExplanation("provisioner has nil function").
				WithContext("provisioner_type", p.Type).
				WithContext("event", string(event)).
				WithHint("Ensure provisioners are registered using RegisterProvisioner").
				Err()
		}

		if err := p.Func(ctx, atmosConfig, componentConfig, authContext, ec); err != nil {
			return errUtils.Build(errUtils.ErrProvisionerFailed).
				WithCause(err).
				WithContext("provisioner_type", p.Type).
				WithContext("event", string(event)).
				Err()
		}
	}

	return nil
}
