package output

import (
	"context"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// workdirProvisionCache caches provisioned JIT workdirs by stack-component key.
// Prevents redundant provisioning when multiple components reference the same
// JIT component during a parallel describe-stacks run.
var workdirProvisionCache sync.Map

// ResetWorkdirProvisionCache clears the workdir provision cache.
// Exported for use in tests to ensure cache isolation between test functions.
func ResetWorkdirProvisionCache() {
	defer perf.Track(nil, "output.ResetWorkdirProvisionCache")()

	workdirProvisionCache.Range(func(key, _ any) bool {
		workdirProvisionCache.Delete(key)
		return true
	})
}

// WorkdirProvisioner provisions a JIT working directory before terraform operations.
type WorkdirProvisioner interface {
	// Provision ensures the JIT working directory exists and is populated
	// with component files before terraform init runs.
	Provision(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error
}

// defaultWorkdirProvisioner delegates to workdir.ProvisionWorkdir.
type defaultWorkdirProvisioner struct{}

func (d *defaultWorkdirProvisioner) Provision(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	return provWorkdir.ProvisionWorkdir(ctx, atmosConfig, componentConfig, authContext)
}

// ensureWorkdirProvisioned provisions a JIT working directory if the component
// has provision.workdir.enabled: true and auto-provisioning is enabled.
//
// This is called in execute() before terraform init so that the workdir exists
// and contains component files when init runs. Without this, !terraform.output
// and atmos.Component calls against JIT components fail with an empty-directory
// error from terraform init.
//
// config must be passed by pointer — this function may set config.InitRunReconfigure
// to true when a fresh provision occurs, which must be visible to the subsequent
// runInit call.
func (e *Executor) ensureWorkdirProvisioned(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	sections map[string]any,
	authContext *schema.AuthContext,
	component, stack string,
	config *ComponentConfig,
) error {
	defer perf.Track(atmosConfig, "output.Executor.ensureWorkdirProvisioned")()

	if !config.AutoProvisionWorkdirForOutputs {
		return nil
	}

	if !provWorkdir.IsWorkdirEnabled(sections) {
		return nil
	}

	cacheKey := stackComponentKey(stack, component) // only allocate after early-return guards
	if _, alreadyProvisioned := workdirProvisionCache.LoadOrStore(cacheKey, struct{}{}); alreadyProvisioned {
		log.Debug("JIT workdir already provisioned this run, skipping", "component", component, "stack", stack)
		return nil
	}

	log.Debug("Auto-provisioning JIT workdir for output fetch", "component", component, "stack", stack)

	if err := e.workdirProvisioner.Provision(ctx, atmosConfig, sections, authContext); err != nil {
		workdirProvisionCache.Delete(cacheKey)
		return errUtils.Build(errUtils.ErrWorkdirProvision).
			WithCause(fmt.Errorf("component '%s' in stack '%s': %w", component, stack, err)).
			Err()
	}

	// If the provisioner freshly synced files, it sets WorkdirReprovisionedKey.
	// A new workdir has no .terraform/ directory — terraform init must run with -reconfigure
	// to avoid an interactive "migrate workspaces?" prompt that would hang the process.
	if _, freshlyProvisioned := sections[provWorkdir.WorkdirReprovisionedKey]; freshlyProvisioned {
		config.InitRunReconfigure = true
	}

	ui.Info(fmt.Sprintf("Auto-provisioned JIT workdir for component '%s' in stack '%s'", component, stack))
	log.Info("Consider using !terraform.state for workdir-independent output access (no terraform init required)",
		"component", component, "stack", stack)

	return nil
}
