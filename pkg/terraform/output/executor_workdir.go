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
	"github.com/cloudposse/atmos/pkg/provisioner"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// workdirProvisionGroup deduplicates concurrent provision calls for the same (stack, component).
// When multiple goroutines race to provision the same JIT workdir (e.g. during parallel
// atmos describe stacks), singleflight ensures Provision is called exactly once per key.
var workdirProvisionGroup singleflight.Group

// workdirProvisionCache records (stack, component) pairs that have been successfully
// provisioned before -- nothing more. It is checked inside workdirProvisionGroup.Do to
// short-circuit re-running Provision on a later, separate call for a key a prior generation
// already provisioned. It deliberately does NOT record whether that provisioning was "fresh":
// freshness (config.WorkdirReprovisioned) is a one-time signal scoped to the single
// provisioning generation that actually ran -- see ensureWorkdirProvisioned's closure, which
// returns freshlyProvisioned directly to that generation's callers instead of persisting it
// here, so a later call reusing an already-provisioned workdir doesn't inherit stale freshness
// and force a full re-init on every subsequent `terraform output`.
var workdirProvisionCache sync.Map

// doChanEntryHook, when non-nil, is called immediately before every
// workdirProvisionGroup.DoChan call -- a test-only seam letting a concurrency test
// deterministically confirm a caller has reached singleflight registration, instead of
// inferring it from scheduling behavior (e.g. runtime.Gosched()) that proves nothing about
// which goroutine the runtime actually chose to run next.
var doChanEntryHook func()

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
	if err := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, componentConfig, authContext, provisioner.OutputWriters{}); err != nil {
		return err
	}

	// For local components with no source, ProvisionWorkdir copies component files
	// to the workdir. For source-provisioned components where AutoProvisionSource
	// already set WorkdirPathKey, ProvisionWorkdir detects the key and returns
	// immediately without duplicating the copy.
	return provWorkdir.ProvisionWorkdir(ctx, atmosConfig, componentConfig, authContext, provisioner.OutputWriters{})
}

// ensureWorkdirProvisioned provisions a JIT working directory if the component
// has provision.workdir.enabled: true and auto-provisioning is enabled.
//
// This is called in execute() before terraform init so that the workdir exists
// and contains component files when init runs. Without this, !terraform.output
// and atmos.Component calls against JIT components fail with an empty-directory
// error from terraform init.
//
// The config argument must be passed by pointer — this function may set
// config.WorkdirReprovisioned to true when a fresh provision occurs, which
// must be visible to the subsequent autoinit.Decide call (ensureInitialized
// forces init via Request.Force when set, since a brand-new workdir has no
// .terraform/ directory to fingerprint against).
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

	if doChanEntryHook != nil {
		doChanEntryHook()
	}

	resultCh := workdirProvisionGroup.DoChan(cacheKey, func() (any, error) {
		// LoadOrStore at the TOP of the closure: atomically claim the key before
		// Provision runs. This closes the TOCTOU window — any goroutine arriving
		// after DoChan returns will find the key already present and short-circuit.
		// NOTE: must be inside DoChan (not outside) so that concurrent callers still
		// wait via singleflight rather than returning nil before provisioning completes.
		//
		// The stored value is a constant `true` ("has been provisioned before"), never the
		// freshness bool: singleflight already serializes concurrent callers of a single
		// provisioning generation onto this one closure execution, which returns
		// freshlyProvisioned directly to every caller of THAT generation below. A `loaded`
		// hit here only ever means a *later, separate* generation (this closure runs again
		// only after the prior one fully completed) is reusing an already-provisioned
		// workdir — it must report false, or every later call would keep inheriting the
		// one-time freshness signal and force a full re-init on every subsequent call.
		if _, loaded := workdirProvisionCache.LoadOrStore(cacheKey, true); loaded {
			return false, nil
		}

		log.Debug("Auto-provisioning JIT workdir for output fetch", "component", component, "stack", stack)

		// Detach from the caller's per-request context so that a leader whose context
		// is cancelled mid-provisioning does not abort work that all waiters depend on.
		// Each waiter can still exit early via its own ctx.Done() branch in the select below;
		// this only insulates the shared provisioning run from the leader's deadline.
		provCtx := context.WithoutCancel(ctx)
		if spinnersSuppressed() {
			provCtx = provWorkdir.WithOutputSuppressed(provCtx)
		}
		if err := e.workdirProvisioner.Provision(provCtx, atmosConfig, sections, authContext); err != nil {
			// Provision failed: remove the key so the next caller can retry.
			workdirProvisionCache.Delete(cacheKey)
			return false, errUtils.Build(errUtils.ErrWorkdirProvision).
				WithCause(fmt.Errorf("component '%s' in stack '%s': %w", component, stack, err)).
				WithHint("Set components.terraform.auto_provision_workdir_for_outputs: false in atmos.yaml (or ATMOS_COMPONENTS_TERRAFORM_AUTO_PROVISION_WORKDIR_FOR_OUTPUTS=false) to disable. For source.uri components, check that source credentials are available.").
				Err()
		}

		// If the provisioner freshly synced files, it sets WorkdirReprovisionedKey.
		// A new workdir has no .terraform/ directory — terraform init must run with -reconfigure
		// to avoid an interactive "migrate workspaces?" prompt that would hang the process.
		// Returned directly (never persisted to workdirProvisionCache) so every waiting
		// goroutine in THIS provisioning generation (not just the leader) can apply it to its
		// own config pointer after DoChan returns, while later, separate generations that reuse
		// this already-provisioned workdir correctly get false via the `loaded` branch above.
		_, freshlyProvisioned := sections[provWorkdir.WorkdirReprovisionedKey]

		writeVisibleOutput(func() {
			ui.ClearLine()
			ui.Info(fmt.Sprintf("Auto-provisioned JIT workdir for component '%s' in stack '%s'", component, stack))
			ui.Hint("Tip: use `!terraform.state` instead of `!terraform.output` to read outputs without terraform init")
		})

		return freshlyProvisioned, nil
	})

	// DoChan returns a buffered channel (capacity 1) so the leader's result is
	// never lost even if this goroutine exits early via ctx.Done(). The select
	// below allows a waiter whose context has been cancelled to return immediately
	// without blocking until the leader finishes. If both resultCh and ctx.Done()
	// are ready simultaneously (e.g. the leader just finished and the context was
	// already cancelled), Go selects non-deterministically; either outcome is safe
	// since the cache is already populated for subsequent callers.
	select {
	case res := <-resultCh:
		if res.Err != nil {
			return res.Err
		}
		if reprovisioned, _ := res.Val.(bool); reprovisioned {
			config.WorkdirReprovisioned = true
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
