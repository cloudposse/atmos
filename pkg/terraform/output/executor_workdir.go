package output

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/singleflight"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
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

// defaultWorkdirProvisioner delegates to the source and workdir provisioners.
type defaultWorkdirProvisioner struct{}

func (d *defaultWorkdirProvisioner) Provision(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	// For source-provisioned components (source.uri + provision.workdir.enabled),
	// run the source provisioner first to create and hydrate the workdir from the
	// source URI before terraform init runs.
	//
	// In the ExecuteTerraform path, source provisioning is handled by the
	// before.terraform.init hook registered in pkg/provisioner/source. The output
	// executor has its own runInit that calls tfexec directly and never fires that
	// hook system. Without this call, !terraform.output on a source-provisioned
	// component with no existing workdir always fails — nothing creates the
	// directory before init runs.
	//
	// Trade-off: remote source URIs (GitHub, S3) will now trigger a network fetch
	// during !terraform.output evaluation. TTL caching limits this to the first
	// call per TTL window, but callers should be aware that output reads may
	// require source credentials and network access when the workdir is cold.
	if err := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, componentConfig, authContext); err != nil {
		return err
	}

	// For local components with no source, ProvisionWorkdir copies component files
	// to the workdir. For source-provisioned components where AutoProvisionSource
	// already set WorkdirPathKey, ProvisionWorkdir detects the key and returns
	// immediately without duplicating the copy.
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

	result, sfErr, _ := workdirProvisionGroup.Do(cacheKey, func() (any, error) {
		// Short-circuit: a previous in-flight call completed successfully.
		if _, done := workdirProvisionCache.Load(cacheKey); done {
			return false, nil
		}

		log.Debug("Auto-provisioning JIT workdir for output fetch", "component", component, "stack", stack)

		// Clear the spinner's current frame before the provisioner writes its own
		// status messages. Without this, the provisioner's ui.Info/ui.Success calls
		// interleave with the live bubbletea spinner on the same stderr stream,
		// producing spurious leading whitespace equal to the spinner frame width.
		ui.ClearLine()

		if err := e.workdirProvisioner.Provision(ctx, atmosConfig, sections, authContext); err != nil {
			// Do NOT store cacheKey on failure. singleflight does not cache errors,
			// so the next call will re-enter this closure and retry provisioning.
			return false, errUtils.Build(errUtils.ErrWorkdirProvision).
				WithCause(fmt.Errorf("component '%s' in stack '%s': %w", component, stack, err)).
				Err()
		}

		// If the provisioner freshly synced files, it sets WorkdirReprovisionedKey.
		// A new workdir has no .terraform/ directory — terraform init must run with -reconfigure
		// to avoid an interactive "migrate workspaces?" prompt that would hang the process.
		// Return the bool so every waiting goroutine (not just the leader) can apply it
		// to its own config pointer after Do returns.
		_, freshlyProvisioned := sections[provWorkdir.WorkdirReprovisionedKey]

		// Only store on success. workdirProvisionCache handles post-completion
		// short-circuiting for calls after the in-flight group completes.
		workdirProvisionCache.Store(cacheKey, struct{}{})

		// Clear any spinner frame the provisioner may have left behind before
		// writing the final status line.
		ui.ClearLine()
		ui.Info(fmt.Sprintf("Auto-provisioned JIT workdir for component '%s' in stack '%s'", component, stack))
		log.Debug("Consider using !terraform.state for workdir-independent output access (no terraform init required)",
			"component", component, "stack", stack)

		return freshlyProvisioned, nil
	})

	if sfErr == nil {
		if reconfigure, _ := result.(bool); reconfigure {
			config.InitRunReconfigure = true
		}
	}

	return sfErr
}
