package container

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/tags"
)

// newVerbCmd builds a container subcommand that takes a required `component`
// positional argument and dispatches to the container component provider. The
// positional-args validator is separator-aware: pass-through args after "--"
// (used by `exec`) are not counted as positional args.
func newVerbCmd(name, short, long string) *cobra.Command {
	return buildVerbCmd(name, short, long, false, false)
}

// newBulkVerbCmd builds a lifecycle subcommand that can operate on a single
// component (positional arg), on all container components in a stack/everywhere
// (`--all`), or interactively (no component, picker prompts for stack +
// components). The `component` positional arg is therefore optional.
func newBulkVerbCmd(name, short, long string) *cobra.Command {
	return buildVerbCmd(name, short, long, true, true)
}

// buildVerbCmd is the shared constructor. The optionalArg flag makes the
// `component` positional optional (usage `[component]`); withAllFlag registers
// an `--all` flag on the command.
func buildVerbCmd(name, short, long string, optionalArg, withAllFlag bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Long:  long,
		RunE: func(c *cobra.Command, args []string) error {
			return runVerb(c, c.Name(), args)
		},
	}

	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:        "component",
		Description: "Container component",
		Required:    !optionalArg,
		TargetField: "Component",
	})
	_, validator, usage := argsBuilder.Build()
	cmd.Use = name + " " + usage
	cmd.Args = validator

	if withAllFlag {
		// Register `--all`/`--tags`/`--labels` as local flags on this verb only.
		// They are read directly from the executing command's flags (see
		// buildConfigAndStacksInfo), not via the global Viper, so binding them to
		// Viper here would only risk colliding with other commands' keys without
		// adding value.
		parser := flags.NewStandardParser(
			flags.WithBoolFlag("all", "", false, "Operate on all container components in all stacks (scope to one stack with --stack)"),
			flags.WithStringSliceFlag("tags", "", nil, "Filter by tags (comma-separated, matches any): --tags=production,tier-1"),
			flags.WithStringFlag("labels", "", "", "Filter by labels (comma-separated key=value pairs, matches all): --labels=cost-center=platform,compliance=sox"),
		)
		parser.RegisterFlags(cmd)

		// Wrap the positional-args validator to also reject a malformed --labels
		// value up front, so a parse failure never silently falls through to "no
		// filter" (which would be dangerous for destructive verbs like down/rm/stop).
		baseValidator := cmd.Args
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := baseValidator(c, args); err != nil {
				return err
			}
			labelsFlag, _ := c.Flags().GetString("labels")
			_, err := tags.ParseLabelsFlag(labelsFlag)
			return err
		}
	}

	return cmd
}

// newLogsCmd builds the `logs` subcommand. Like the bulk verbs the `component`
// argument is optional (omit it to stream `--all` components or pick them
// interactively), and it adds `--follow`/`-f` and `--tail`. Streaming is handled
// directly by the provider, not the generic bulk fan-out, so multiple components
// can be followed concurrently with a per-component line prefix.
func newLogsCmd() *cobra.Command {
	cmd := buildVerbCmd("logs",
		"Show logs from container components",
		"Show logs from one or many container components, discovered by label. Omit the component to stream all (`--all`) or pick interactively; `--follow` streams continuously.",
		true, true)

	parser := flags.NewStandardParser(
		flags.WithBoolFlag("follow", "f", false, "Follow log output (stream). With multiple components, output is interleaved and prefixed with the component name"),
		flags.WithStringFlag("tail", "", "all", "Number of lines to show from the end of the logs, or \"all\" (use --tail=N)"),
		// Bare `--tail` means "all" and does not consume the next token, so
		// `--tail --chdir=…` does not swallow the following flag. Use `--tail=N`.
		flags.WithNoOptDefVal("tail", "all"),
	)
	parser.RegisterFlags(cmd)

	return cmd
}

// Image artifact lifecycle. These verbs support bulk operation (`--all` or an
// interactive picker) since they are non-interactive and safe to batch.
var (
	buildCmd = newBulkVerbCmd("build",
		"Build the component image from build",
		"Build the component's container image using Docker or Podman from the `build` configuration.")
	pushCmd = newBulkVerbCmd("push",
		"Push the component image to its registry",
		"Push the component's image (image) to its container registry.")
	pullCmd = newBulkVerbCmd("pull",
		"Pull the component image",
		"Pull the component's image (image) from its container registry.")
)

// Container lifecycle. The up/restart/stop/rm/down verbs support bulk operation;
// run is a one-shot foreground process and stays single-component.
var (
	runCmd = newVerbCmd("run",
		"Run the component as a one-shot foreground container",
		"Run the component's container in the foreground as a one-shot process using `run`.")
	upCmd = newBulkVerbCmd("up",
		"Create or start the long-running container",
		"Create or start the component's long-running named container, labeled by its canonical instance address.")
	startCmd = newBulkVerbCmd("start",
		"Start the existing stopped container",
		"Start the component's existing (stopped) container in place, discovered by label. The inverse of `stop`; unlike `up` it never creates or recreates the container.")
	restartCmd = newBulkVerbCmd("restart",
		"Restart the component container",
		"Stop and start the component's long-running container, discovered by label.")
	stopCmd = newBulkVerbCmd("stop",
		"Stop the component container",
		"Stop the component's long-running container without removing it, discovered by label.")
	rmCmd = newBulkVerbCmd("rm",
		"Remove the component container",
		"Remove the component's container, discovered by label.")
	downCmd = newBulkVerbCmd("down",
		"Stop and remove the component container",
		"Stop and remove the component's long-running container (stop + rm), discovered by label.")
)

// listCmd lists container components and their running state. Unlike the other
// verbs it takes no component argument (it lists all of them). Container running
// state lives here, not in the generic `atmos list components`.
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List container components and their running state",
	Long:    "List all container components across stacks (optionally filtered by --stack) with their running state.",
	Args:    cobra.NoArgs,
	RunE: func(c *cobra.Command, args []string) error {
		return runVerb(c, "list", args)
	},
}

// Inspection.
var (
	// The ps verb takes an optional component: with one it shows that container's
	// detail; with none it lists every container component's running state (like
	// `list`, optionally filtered by --stack). No --all flag — no component
	// already means "all".
	psCmd = buildVerbCmd("ps",
		"Show container components' running state",
		"Show running state for one container component (by label) or, with no component, for all of them (optionally filtered by --stack).",
		true, false)
	logsCmd = newLogsCmd()
	execCmd = newVerbCmd("exec",
		"Execute a command in the component container",
		"Execute a command in the component's running container. Use `--` to separate the command, e.g. `atmos container exec api -s dev -- sh`.")
	attachCmd = newVerbCmd("attach",
		"Attach to the component container's main process",
		"Attach local stdin/stdout/stderr to the component's running container (its PID 1 / main process), discovered by label. Unlike `exec`, this does not start a new shell — it connects to the existing process. Detach with the runtime's detach keys (Ctrl-P Ctrl-Q), which leaves the container running.")
)
