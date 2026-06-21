package container

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// newVerbCmd builds a container subcommand that takes a required `component`
// positional argument and dispatches to the container component provider. The
// positional-args validator is separator-aware: pass-through args after "--"
// (used by `exec`) are not counted as positional args.
func newVerbCmd(name, short, long string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Long:  long,
		// Allow unknown flags after "--" to pass through to the container (e.g. exec).
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
		RunE: func(c *cobra.Command, args []string) error {
			return runVerb(c, c.Name(), args)
		},
	}

	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:        "component",
		Description: "Container component",
		Required:    true,
		TargetField: "Component",
	})
	_, validator, usage := argsBuilder.Build()
	cmd.Use = name + " " + usage
	cmd.Args = validator

	return cmd
}

// Image artifact lifecycle.
var (
	buildCmd = newVerbCmd("build",
		"Build the component image from vars.build",
		"Build the component's container image using Docker or Podman from the `vars.build` configuration.")
	pushCmd = newVerbCmd("push",
		"Push the component image to its registry",
		"Push the component's image (vars.image) to its container registry.")
	pullCmd = newVerbCmd("pull",
		"Pull the component image",
		"Pull the component's image (vars.image) from its container registry.")
)

// Container lifecycle.
var (
	runCmd = newVerbCmd("run",
		"Run the component as a one-shot foreground container",
		"Run the component's container in the foreground as a one-shot process using `vars.run`.")
	upCmd = newVerbCmd("up",
		"Create or start the long-running container",
		"Create or start the component's long-running named container, labeled by its canonical instance address.")
	restartCmd = newVerbCmd("restart",
		"Restart the component container",
		"Stop and start the component's long-running container, discovered by label.")
	stopCmd = newVerbCmd("stop",
		"Stop the component container",
		"Stop the component's long-running container without removing it, discovered by label.")
	rmCmd = newVerbCmd("rm",
		"Remove the component container",
		"Remove the component's container, discovered by label.")
	downCmd = newVerbCmd("down",
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
	psCmd = newVerbCmd("ps",
		"Show the component container running state",
		"Show whether the component's container is running, discovered by label.")
	logsCmd = newVerbCmd("logs",
		"Show logs from the component container",
		"Show logs from the component's container, discovered by label.")
	execCmd = newVerbCmd("exec",
		"Execute a command in the component container",
		"Execute a command in the component's running container. Use `--` to separate the command, e.g. `atmos container exec api -s dev -- sh`.")
)
