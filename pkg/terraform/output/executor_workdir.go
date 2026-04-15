package output

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/singleflight"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// workdirProvisionGroup deduplicates concurrent provision calls for the same (stack, component).
// When multiple goroutines race to provision the same JIT workdir (e.g. during parallel
// atmos describe stacks), singleflight ensures Provision is called exactly once per key.
var workdirProvisionGroup singleflight.Group

// workdirProvisionCache records successfully provisioned (stack, component) pairs.
// Checked inside workdirProvisionGroup.Do to short-circuit subsequent calls after
// the first in-flight call completes.
var workdirProvisionCache sync.Map

// ResetWorkdirProvisionCache clears the workdir provision cache.
// Exported for use in tests to ensure cache isolation between test functions.
// Note: workdirProvisionGroup (singleflight.Group) is not reset — it tracks
// only in-flight calls and is self-cleaning once all pending calls complete.
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

	cacheKey := stackComponentKey(stack, component)

	_, sfErr, _ := workdirProvisionGroup.Do(cacheKey, func() (any, error) {
		// Short-circuit: a previous in-flight call completed successfully.
		if _, done := workdirProvisionCache.Load(cacheKey); done {
			return nil, nil
		}

		log.Debug("Auto-provisioning JIT workdir for output fetch", "component", component, "stack", stack)

		if err := e.workdirProvisioner.Provision(ctx, atmosConfig, sections, authContext); err != nil {
			// Do NOT store cacheKey on failure. singleflight does not cache errors,
			// so the next call will re-enter this closure and retry provisioning.
			return nil, errUtils.Build(errUtils.ErrWorkdirProvision).
				WithCause(fmt.Errorf("component '%s' in stack '%s': %w", component, stack, err)).
				Err()
		}

		// If the provisioner freshly synced files, it sets WorkdirReprovisionedKey.
		// A new workdir has no .terraform/ directory — terraform init must run with -reconfigure
		// to avoid an interactive "migrate workspaces?" prompt that would hang the process.
		if _, freshlyProvisioned := sections[provWorkdir.WorkdirReprovisionedKey]; freshlyProvisioned {
			config.InitRunReconfigure = true
		}

		// Only store on success. workdirProvisionCache handles post-completion
		// short-circuiting for calls after the in-flight group completes.
		workdirProvisionCache.Store(cacheKey, struct{}{})

		ui.Info(fmt.Sprintf("Auto-provisioned JIT workdir for component '%s' in stack '%s'", component, stack))
		log.Info("Consider using !terraform.state for workdir-independent output access (no terraform init required)",
			"component", component, "stack", stack)

		return nil, nil
	})

	return sfErr
}
