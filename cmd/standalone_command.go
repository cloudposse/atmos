package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

// standaloneCommandParser adapts native flag definitions to a script-owned CLI.
// Its command and registry are local: standalone programs do not register built-in
// Atmos commands or inherit root flags, configuration, or Viper state.
func standaloneCommandParser(file *script.File, stdout io.Writer) script.CommandParser {
	defer perf.Track(nil, "cmd.standaloneCommandParser")()

	argv := append([]string(nil), file.Args...)
	filename := filepath.Base(file.Path)
	return func(ctx context.Context, spec script.CommandSpec) (script.CommandInput, error) {
		if err := ctx.Err(); err != nil {
			return script.CommandInput{}, err
		}
		if err := validateStandaloneCommandSpec(&spec); err != nil {
			return script.CommandInput{}, err
		}
		if spec.Name == "" {
			spec.Name = filename
		}
		return parseStandaloneCommand(ctx, &spec, argv, stdout)
	}
}

func parseStandaloneCommand(ctx context.Context, spec *script.CommandSpec, argv []string, stdout io.Writer) (script.CommandInput, error) {
	defer perf.Track(nil, "cmd.parseStandaloneCommand")()

	builder := flags.NewPositionalArgsBuilder()
	for _, arg := range spec.Args {
		builder.AddArg(arg)
	}
	_, validateArgs, usage := builder.Build()
	if len(spec.Args) == 0 {
		validateArgs = cobra.ExactArgs(0)
	}
	cmd := &cobra.Command{
		Use: strings.TrimSpace(spec.Name + " " + usage), Short: spec.Description,
		// Execution belongs to the language runtime. Mark the command runnable
		// so Cobra's native help includes usage and flags.
		Run: func(_ *cobra.Command, _ []string) {},
	}
	cmd.SetContext(ctx)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	parser := flags.NewStandardParser(flags.WithBoolFlag("help", "h", false, "Show help for this script"))
	for _, flag := range spec.Flags {
		parser.Registry().Register(flag)
	}
	parser.RegisterFlags(cmd)
	if err := cmd.ParseFlags(argv); err != nil {
		return script.CommandInput{}, err
	}
	if help, _ := cmd.Flags().GetBool("help"); help {
		return script.CommandInput{Help: true}, cmd.Help()
	}
	// Here -- ends script flag parsing. It does not introduce arguments for an
	// external tool, so validate every remaining positional value. A nil command
	// keeps the native builder's separator-aware validator from discarding them.
	positional := cmd.Flags().Args()
	if err := validateArgs(nil, positional); err != nil {
		return script.CommandInput{}, err
	}
	values, err := resolveStandaloneCommandFlags(parser, cmd, spec.Flags)
	if err != nil {
		return script.CommandInput{}, err
	}
	args := make(map[string]any, len(spec.Args))
	for index, arg := range spec.Args {
		args[arg.Name] = nil
		if index < len(positional) {
			args[arg.Name] = positional[index]
		}
	}
	return script.CommandInput{Args: args, Flags: values}, nil
}

func resolveStandaloneCommandFlags(parser *flags.StandardParser, cmd *cobra.Command, definitions []flags.Flag) (map[string]any, error) {
	defer perf.Track(nil, "cmd.resolveStandaloneCommandFlags")()

	values := viper.New()
	if err := parser.BindFlagsToViper(cmd, values); err != nil {
		return nil, err
	}
	if err := parser.ValidateFlagValues(cmd); err != nil {
		return nil, err
	}
	result := make(map[string]any, len(definitions))
	for _, flag := range definitions {
		name := flag.GetName()
		explicit := cmd.Flags().Changed(name)
		rawEnv, hasEnv := standaloneFlagEnvironment(flag)
		if !explicit && hasEnv {
			if err := validateStandaloneFlagEnvironment(cmd, flag, rawEnv); err != nil {
				return nil, err
			}
		}
		if flag.IsRequired() && !explicit && !hasEnv {
			continue
		}
		result[name] = standaloneFlagValue(flag, values)
	}
	return result, parser.Registry().Validate(result)
}

func validateStandaloneCommandSpec(spec *script.CommandSpec) error {
	defer perf.Track(nil, "cmd.validateStandaloneCommandSpec")()

	names := map[string]bool{"help": true}
	shorts := map[string]bool{"h": true}
	for _, flag := range spec.Flags {
		if !supportedStandaloneFlag(flag) {
			return fmt.Errorf("%w: unsupported or nil standalone flag", errUtils.ErrInvalidFlagValue)
		}
		name, short := flag.GetName(), flag.GetShorthand()
		if !validStandaloneFlagName(name) || names[name] {
			return fmt.Errorf("%w: invalid or duplicate standalone flag %q", errUtils.ErrInvalidFlagValue, name)
		}
		if short != "" && (len(short) != 1 || shorts[short] || strings.ContainsAny(short, "- \t\r\n=")) {
			return fmt.Errorf("%w: invalid or duplicate shorthand %q", errUtils.ErrInvalidFlagValue, short)
		}
		names[name], shorts[short] = true, true
	}
	return validateStandaloneCommandArgs(spec.Args)
}

func supportedStandaloneFlag(flag flags.Flag) bool {
	defer perf.Track(nil, "cmd.supportedStandaloneFlag")()

	switch value := flag.(type) {
	case *flags.StringFlag:
		return value != nil
	case *flags.BoolFlag:
		return value != nil
	case *flags.IntFlag:
		return value != nil
	case *flags.StringSliceFlag:
		return value != nil
	default:
		return false
	}
}

func validateStandaloneCommandArgs(args []*flags.PositionalArgSpec) error {
	defer perf.Track(nil, "cmd.validateStandaloneCommandArgs")()

	names := make(map[string]bool, len(args))
	optional := false
	for _, arg := range args {
		if arg == nil || arg.Name == "" || names[arg.Name] {
			return fmt.Errorf("%w: nil, unnamed, or duplicate standalone argument", errUtils.ErrInvalidPositionalArgs)
		}
		if optional && arg.Required {
			return fmt.Errorf("%w: required standalone argument follows an optional argument", errUtils.ErrInvalidPositionalArgs)
		}
		names[arg.Name] = true
		optional = !arg.Required
	}
	return nil
}

func validStandaloneFlagName(name string) bool {
	defer perf.Track(nil, "cmd.validStandaloneFlagName")()

	return name != "" && !strings.HasPrefix(name, "-") && !strings.ContainsAny(name, " \t\r\n=")
}

// standaloneFlagEnvironment follows Viper's default behavior: the first nonempty
// bound variable wins, and an empty variable falls through to the next binding.
func standaloneFlagEnvironment(flag flags.Flag) (string, bool) {
	defer perf.Track(nil, "cmd.standaloneFlagEnvironment")()

	for _, name := range flag.GetEnvVars() {
		if value, exists := os.LookupEnv(name); exists && value != "" {
			return value, true
		}
	}
	return "", false
}

// Viper's typed getters silently turn malformed environment values into zero
// values. Reuse the registered native flag's conversion before reading them.
// CLI input was already parsed once and takes precedence over the environment.
func validateStandaloneFlagEnvironment(cmd *cobra.Command, flag flags.Flag, value string) error {
	defer perf.Track(nil, "cmd.validateStandaloneFlagEnvironment")()

	switch flag.(type) {
	case *flags.IntFlag, *flags.BoolFlag:
		if err := cmd.Flags().Lookup(flag.GetName()).Value.Set(value); err != nil {
			return fmt.Errorf("%w: invalid environment value for --%s: %w", errUtils.ErrInvalidFlagValue, flag.GetName(), err)
		}
	}
	return nil
}

func standaloneFlagValue(flag flags.Flag, values *viper.Viper) any {
	defer perf.Track(nil, "cmd.standaloneFlagValue")()

	name := flag.GetName()
	switch flag.(type) {
	case *flags.StringFlag:
		return values.GetString(name)
	case *flags.BoolFlag:
		return values.GetBool(name)
	case *flags.IntFlag:
		return values.GetInt(name)
	default: // Spec validation restricts the remaining type to StringSliceFlag.
		return values.GetStringSlice(name)
	}
}
