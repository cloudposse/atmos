// Package adapters wires pkg/taskgraph's generic dependency runner to specific integrations --
// mirroring pkg/scheduler/adapters' role for the scheduler. CobraCommand in this file is the
// custom-command-depends-on-command integration: it resolves and invokes an already-registered
// *cobra.Command in-process, in the same fashion internal/exec's workflow dependency adapter
// resolves and invokes workflows.
package adapters

import (
	"context"
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/taskgraph"
)

// IsCustomCommand reports whether cmd was constructed from Atmos command configuration.
// Injected rather than reaching into cmd package internals directly, so this package stays
// decoupled from cmd's own annotation implementation detail.
type IsCustomCommand func(cmd *cobra.Command) bool

type dependenciesResolvedKey struct{}

// WithDependenciesResolved marks ctx so a command invoked as someone else's dependency skips
// re-resolving its own dependencies.commands/dependencies.workflows -- without it, invoking a
// command via its full Run (which unconditionally resolves dependencies at the top of
// executeCustomCommand) would resolve and run that command's dependencies a second, redundant
// time on top of the graph that already ran them.
func WithDependenciesResolved(ctx context.Context) context.Context {
	defer perf.Track(nil, "adapters.WithDependenciesResolved")()

	return context.WithValue(ctx, dependenciesResolvedKey{}, true)
}

// DependenciesAlreadyResolved reports whether cmd was invoked as a dependency of another command
// whose taskgraph run already satisfied cmd's own dependencies.
func DependenciesAlreadyResolved(cmd *cobra.Command) bool {
	defer perf.Track(nil, "adapters.DependenciesAlreadyResolved")()

	ctx := cmd.Context()
	if ctx == nil {
		return false
	}
	resolved, _ := ctx.Value(dependenciesResolvedKey{}).(bool)
	return resolved
}

// dependencyErrorKey is the context key under which a dependency invocation's error sink is
// stored (see WithDependencyErrorSink/RecordDependencyError). It lets a failure deep inside
// executeCustomCommand be reported back to the taskgraph dispatch that invoked it in-process,
// instead of hard-exiting the whole process before taskgraph.Run's fail-mode handling
// (wait_all/fail_fast/best_effort) ever sees it -- exiting from inside a single dependency's own
// execution made that handling unreachable regardless of what fail: mode was declared.
type dependencyErrorKey struct{}

// errorSink collects at most one error from a dependency's in-process execution. A struct (not a
// bare *error) so a pointer to it can be stored in context.Value and mutated after the caller
// (CustomCommandRunner) retrieves it, once the dependency's Run() call returns.
type errorSink struct {
	mu  sync.Mutex
	err error
}

// WithDependencyErrorSink returns ctx carrying a fresh error sink a dependency invocation can
// report its failure into, plus the sink itself so the caller can read it back after Run()
// returns (Cobra's Run has no return value, so this is the channel back to the dispatcher).
func WithDependencyErrorSink(ctx context.Context) (context.Context, *errorSink) {
	defer perf.Track(nil, "adapters.WithDependencyErrorSink")()

	sink := &errorSink{}
	return context.WithValue(ctx, dependencyErrorKey{}, sink), sink
}

// RecordDependencyError reports err into cmd's error sink, if one is present in its context (i.e.
// cmd is running as someone else's already-resolved dependency -- see
// DependenciesAlreadyResolved). A no-op when no sink is present, so call sites outside a
// dependency invocation (a command's own top-level, non-dependency run) are unaffected.
func RecordDependencyError(cmd *cobra.Command, err error) {
	defer perf.Track(nil, "adapters.RecordDependencyError")()

	ctx := cmd.Context()
	if ctx == nil {
		return
	}
	sink, ok := ctx.Value(dependencyErrorKey{}).(*errorSink)
	if !ok || sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.err = err
}

// FindRegisteredCommand searches root's custom subcommands (and their own nested custom
// subcommands, recursively) for a *cobra.Command whose own Name() matches name, mirroring
// schema.FindCommandByName's by-name-regardless-of-nesting semantics for the already-registered
// cobra tree. Deliberately restricted to isCustom(cmd) matches -- without this guard, a
// depth-first walk of the FULL command tree (including built-ins) can shadow the intended custom
// command with an unrelated built-in subcommand that happens to share a leaf name deep in the
// tree (e.g. `atmos mcp client test <name>` matching a dependencies.commands: [test] entry meant
// for a top-level custom command named "test").
func FindRegisteredCommand(root *cobra.Command, name string, isCustom IsCustomCommand) (*cobra.Command, bool) {
	defer perf.Track(nil, "adapters.FindRegisteredCommand")()

	for _, sub := range root.Commands() {
		if !isCustom(sub) {
			continue
		}
		if sub.Name() == name {
			return sub, true
		}
		if found, ok := FindRegisteredCommand(sub, name, isCustom); ok {
			return found, true
		}
	}
	return nil, false
}

// commandLocks lazily creates and shares one *sync.Mutex per already-registered *cobra.Command,
// letting CustomCommandRunner serialize dispatches that resolve to the SAME command (see its own
// doc comment for why) without blocking dispatches of different commands.
type commandLocks struct {
	mu    sync.Mutex
	locks map[*cobra.Command]*sync.Mutex
}

// lockFor returns the shared mutex for target, creating it on first use. Unexported: internal
// bookkeeping for CustomCommandRunner, not part of this package's public API.
func (c *commandLocks) lockFor(target *cobra.Command) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks == nil {
		c.locks = make(map[*cobra.Command]*sync.Mutex)
	}
	l, ok := c.locks[target]
	if !ok {
		l = &sync.Mutex{}
		c.locks[target] = l
	}
	return l
}

// resetFlagsToDeclaredDefaults resets every flag in fs NOT present in overrides back to its
// declared default. Without this, a dispatch with no override for a given flag (e.g. the bare
// `compile` in dependencies.commands: [compile, {name: compile, flags: {target: test}}]) would
// silently inherit whatever value a PRIOR dispatch sharing the same *cobra.Command last Set() --
// deterministically wrong (not just under a race), since a shared FlagSet's mutated state
// persists across sequential dispatches too, not only concurrent ones.
func resetFlagsToDeclaredDefaults(fs *pflag.FlagSet, overrides map[string]string) {
	fs.VisitAll(func(f *pflag.Flag) {
		if _, ok := overrides[f.Name]; ok {
			return
		}
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// CustomCommandRunner executes a taskgraph.Ref{Kind: KindCommand} dependency in-process by
// locating its already-registered *cobra.Command under root and invoking it directly with the
// given flag/arg overrides. In-process, not a subprocess, since command-depends-on-command is
// entirely resolvable within the cobra tree -- unlike workflow-depends-on-command (see
// internal/exec's WorkflowRunner-via-subprocess), which cannot reach this cobra tree without an
// import cycle.
//
// Two Refs sharing a Name -- e.g. the same command depended on twice with different flags:
// dependencies.commands: [{name: build, flags: {env: dev}}, {name: build, flags: {env: prod}}]
// -- resolve to the SAME already-registered *cobra.Command (FindRegisteredCommand is keyed by
// name, not by Ref), and taskgraph runs independent graph nodes concurrently by default. Without
// commandLocks, concurrent dispatches would race on that one shared, mutable Cobra object's
// PersistentFlags/context/annotations. Same-name dependencies serialize; differently-named ones
// still run fully concurrently.
func CustomCommandRunner(root *cobra.Command, isCustom IsCustomCommand) taskgraph.Runner {
	defer perf.Track(nil, "adapters.CustomCommandRunner")()

	locks := &commandLocks{}

	return func(ctx context.Context, ref taskgraph.Ref) error {
		target, ok := FindRegisteredCommand(root, ref.Name, isCustom)
		if !ok || target == nil {
			return fmt.Errorf("%w: %q", errUtils.ErrCustomCommandDependencyNotRegistered, ref.Name)
		}

		targetLock := locks.lockFor(target)
		targetLock.Lock()
		defer targetLock.Unlock()

		return dispatchCustomCommand(ctx, target, &ref)
	}
}

// dispatchCustomCommand applies ref's flag overrides to target (resetting every other flag to its
// declared default first), replicates Cobra's own PreRun -> Run lifecycle (custom commands set no
// PostRun; invoking target directly here bypasses Command.Execute()'s normal lifecycle), and
// returns whatever error the invocation recorded via its dependency error sink -- Cobra's Run has
// no return value, so the sink is the channel back to the caller. Ref is passed by pointer only to
// avoid copying its (currently 96-byte) value; it is never mutated here.
func dispatchCustomCommand(ctx context.Context, target *cobra.Command, ref *taskgraph.Ref) error {
	resetFlagsToDeclaredDefaults(target.PersistentFlags(), ref.Flags)
	// Custom-command flags are always registered via PersistentFlags -- Command.Flags() returns
	// only the local flag set and does not merge persistent flags in until Cobra's own Execute()
	// path runs, which this in-process invocation bypasses.
	for name, value := range ref.Flags {
		if err := target.PersistentFlags().Set(name, value); err != nil {
			return fmt.Errorf("failed to set flag %q for dependency %q: %w", name, ref.Name, err)
		}
	}

	depCtx, sink := WithDependencyErrorSink(WithDependenciesResolved(ctx))
	target.SetContext(depCtx)
	if target.PreRun != nil {
		target.PreRun(target, ref.Args)
	}
	target.Run(target, ref.Args)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.err
}

// CustomCommandDependencyOptions returns the taskgraph.Options wiring shared by every custom
// command's dependencies.commands/dependencies.workflows resolution: commands run in-process
// against root's registered cobra tree; workflows require an explicit `file:` (a custom command
// has no "current workflow file" to default same-name lookups against).
func CustomCommandDependencyOptions(atmosConfig *schema.AtmosConfiguration, root *cobra.Command, isCustom IsCustomCommand) []taskgraph.Option {
	defer perf.Track(atmosConfig, "adapters.CustomCommandDependencyOptions")()

	return []taskgraph.Option{
		taskgraph.WithCommandRunner(CustomCommandRunner(root, isCustom)),
		taskgraph.WithCommandLookup(e.CommandLookup(atmosConfig)),
		taskgraph.WithWorkflowRunner(e.WorkflowRunner(atmosConfig, "", false, "")),
		taskgraph.WithWorkflowLookup(e.WorkflowLookup(atmosConfig, "")),
	}
}
