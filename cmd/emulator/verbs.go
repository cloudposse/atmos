package emulator

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// componentPromptParsers owns the positional parser for each lifecycle verb.
// Each parser is command-scoped because StandardParser retains the Cobra command
// it parses, including inherited flags such as --stack.
var componentPromptParsers = map[*cobra.Command]*flags.StandardParser{}

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
		Name:           "component",
		Description:    "Emulator component",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: componentArgCompletion,
		PromptTitle:    "Choose an emulator component",
	})
	specs, validator, usage := argsBuilder.Build()
	cmd.Use = name + " " + usage

	parser := flags.NewStandardParser(
		flags.WithPositionalArgPrompt("component", "Choose an emulator component", componentArgCompletion),
	)
	parser.SetPositionalArgs(specs, validator, usage)
	parser.RegisterFlags(cmd)
	componentPromptParsers[cmd] = parser

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
	psCmd = &cobra.Command{
		Use:   "ps",
		Short: "List configured running emulators",
		Long:  "List running emulator components configured in the current project. Scope to a single stack with --stack, or pass --runtime to inspect raw runtime containers.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return runVerb(c, "ps", args)
		},
	}
	logsCmd = newVerbCmd("logs",
		"Show the emulator container logs",
		"Stream the emulator container's logs, discovered by label.")
	execCmd = newVerbCmd("exec",
		"Run a command in the emulator container",
		"Run a command in the emulator's container (args after `--`); defaults to a shell.")
	// List takes no component positional: it inventories configured emulator
	// components (optionally scoped to a stack) with a status dot.
	listCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List emulators and their status",
		Long:    "List configured emulator components in a clean table with a status dot, image, and container ID. Scope to a single stack with `--stack`, or pass --runtime to inspect raw runtime containers.",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return runVerb(c, "list", args)
		},
	}
)

func init() {
	for _, cmd := range []*cobra.Command{listCmd, psCmd} {
		cmd.Flags().Bool(flagRuntime, false, "Inspect raw emulator containers instead of configured emulator components")
	}
}
