// Package adapters wires pkg/taskgraph's generic dependency runner to specific integrations --
// mirroring pkg/scheduler/adapters' role for the scheduler. CobraCommand in this file is the
// custom-command-depends-on-command integration: it resolves and invokes an already-registered
// *cobra.Command in-process, in the same fashion internal/exec's workflow dependency adapter
// resolves and invokes workflows.
package adapters

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

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

// CustomCommandRunner executes a taskgraph.Ref{Kind: KindCommand} dependency in-process by
// locating its already-registered *cobra.Command under root and invoking it directly with the
// given flag/arg overrides. In-process, not a subprocess, since command-depends-on-command is
// entirely resolvable within the cobra tree -- unlike workflow-depends-on-command (see
// internal/exec's WorkflowRunner-via-subprocess), which cannot reach this cobra tree without an
// import cycle.
func CustomCommandRunner(root *cobra.Command, isCustom IsCustomCommand) taskgraph.Runner {
	defer perf.Track(nil, "adapters.CustomCommandRunner")()

	return func(ctx context.Context, ref taskgraph.Ref) error {
		target, ok := FindRegisteredCommand(root, ref.Name, isCustom)
		if !ok || target == nil {
			return fmt.Errorf("%w: %q", errUtils.ErrCustomCommandDependencyNotRegistered, ref.Name)
		}
		// Custom-command flags are always registered via PersistentFlags -- Command.Flags()
		// returns only the local flag set and does not merge persistent flags in until Cobra's
		// own Execute() path runs, which this in-process invocation bypasses.
		for name, value := range ref.Flags {
			if err := target.PersistentFlags().Set(name, value); err != nil {
				return fmt.Errorf("failed to set flag %q for dependency %q: %w", name, ref.Name, err)
			}
		}
		target.SetContext(WithDependenciesResolved(ctx))
		// Replicate Cobra's own PreRun -> Run ordering (custom commands set no PostRun), since
		// invoking target directly here bypasses Command.Execute()'s normal lifecycle.
		if target.PreRun != nil {
			target.PreRun(target, ref.Args)
		}
		target.Run(target, ref.Args)
		return nil
	}
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
