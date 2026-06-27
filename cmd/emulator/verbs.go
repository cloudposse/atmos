package emulator

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// newVerbCmd builds an emulator subcommand that takes a required `component`
// positional argument and dispatches to the emulator component provider. The
// positional-args validator is separator-aware: pass-through args after "--"
// (used by `exec`) are not counted as positional args.
func newVerbCmd(name, short, long string) *cobra.Command {
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
		Description: "Emulator component",
		Required:    true,
		TargetField: "Component",
	})
	_, validator, usage := argsBuilder.Build()
	cmd.Use = name + " " + usage
	cmd.Args = validator

	return cmd
}

var (
	upCmd = newVerbCmd("up",
		"Start the emulator container",
		"Start (or reuse) the emulator's long-running container, labeled by its canonical instance address; it outlives the atmos process.")
	downCmd = newVerbCmd("down",
		"Stop and remove the emulator container",
		"Stop and remove the emulator's container, discovered by label. Persisted state is kept; use `reset` to wipe it.")
	resetCmd = newVerbCmd("reset",
		"Stop the emulator and wipe its persisted state",
		"Stop and remove the emulator's container, then delete its persisted state directory under the XDG cache. The next `up` starts a fresh instance.")
	psCmd = newVerbCmd("ps",
		"List running emulators in the stack",
		"List the running emulator containers in the component's stack, discovered by label.")
	logsCmd = newVerbCmd("logs",
		"Show the emulator container logs",
		"Stream the emulator container's logs, discovered by label.")
	execCmd = newVerbCmd("exec",
		"Run a command in the emulator container",
		"Run a command in the emulator's container (args after `--`); defaults to a shell.")
)
