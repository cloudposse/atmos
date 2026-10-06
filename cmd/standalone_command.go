package cmd

import (
	"context"
	"errors"
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
	"github.com/cloudposse/atmos/pkg/script/standalone"
)

// standaloneCommandParser adapts native flag definitions to a script-owned CLI.
// Its command and registry are local: standalone programs do not register built-in
// Atmos commands or inherit root flags, configuration, or Viper state.
func standaloneCommandParser(file *script.File, stdout io.Writer) script.CommandParser {
	defer perf.Track(nil, "cmd.standaloneCommandParser")()

	argv := append([]string(nil), file.Args...)
	filename := filepath.Base(file.Path)
	if file.Stdin {
		filename = "stdin"
	}
	invoked := file.Invoked
	if invoked == "" {
		invoked = filename
	}
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
		return parseStandaloneCommand(ctx, &spec, argv, stdout, invoked)
	}
}

func parseStandaloneCommand(ctx context.Context, spec *script.CommandSpec, argv []string, stdout io.Writer, invoked string) (script.CommandInput, error) {
	defer perf.Track(nil, "cmd.parseStandaloneCommand")()

	builder := flags.NewPositionalArgsBuilder()
	for _, arg := range spec.Args {
		builder.AddArg(arg)
	}
	_, _, usage := builder.Build()
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
	useDecimalIntegers(cmd, spec.Flags)
	standalone.AnnotateFlags(cmd.Flags(), spec.Flags)
	if err := cmd.ParseFlags(argv); err != nil {
		return script.CommandInput{}, standalone.NewUsageError(cmd, invoked, err)
	}
	if help, _ := cmd.Flags().GetBool("help"); help {
		return script.CommandInput{Help: true}, cmd.Help()
	}
	input, err := collectStandaloneInput(cmd, parser, spec, argv)
	if err != nil {
		return script.CommandInput{}, standalone.NewUsageError(cmd, invoked, err)
	}
	return input, nil
}

// useDecimalIntegers makes integer flags accept base-10 digits only, as environment values do.
func useDecimalIntegers(cmd *cobra.Command, definitions []flags.Flag) {
	defer perf.Track(nil, "cmd.useDecimalIntegers")()

	for _, definition := range definitions {
		if intFlag, ok := definition.(*flags.IntFlag); ok {
			cmd.Flags().Lookup(intFlag.Name).Value = standalone.NewDecimalInt(intFlag.Default)
		}
	}
}

// collectStandaloneInput validates the parsed command line and gathers argument and flag values.
// Every failure here is caused by the user's input rather than by the script.
func collectStandaloneInput(cmd *cobra.Command, parser *flags.StandardParser, spec *script.CommandSpec, argv []string) (script.CommandInput, error) {
	defer perf.Track(nil, "cmd.collectStandaloneInput")()

	if err := standalone.CheckBoolPositional(cmd.Flags(), argv); err != nil {
		return script.CommandInput{}, err
	}
	// Here -- ends script flag parsing. It does not introduce arguments for an
	// external tool, so validate every remaining positional value.
	positional := cmd.Flags().Args()
	if err := standalone.ValidatePositionals(spec.Args, positional); err != nil {
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
	if err := applyStandaloneEnvironment(cmd, values, definitions); err != nil {
		return nil, err
	}
	if err := parser.ValidateFlagValues(cmd); err != nil {
		return nil, err
	}
	result := make(map[string]any, len(definitions))
	for _, flag := range definitions {
		name := flag.GetName()
		explicit := cmd.Flags().Changed(name)
		_, hasEnv := standaloneFlagEnvironment(flag)
		if flag.IsRequired() && !explicit && !hasEnv {
			continue
		}
		result[name] = standaloneFlagValue(flag, values)
	}
	return result, parser.Registry().Validate(result)
}

func validateStandaloneCommandSpec(spec *script.CommandSpec) error {
	defer perf.Track(nil, "cmd.validateStandaloneCommandSpec")()

	// Viper keys are case-insensitive, so names that differ only by case would share one value.
	names := map[string]bool{"help": true}
	shorts := map[string]bool{"h": true}
	for _, flag := range spec.Flags {
		if !supportedStandaloneFlag(flag) {
			return fmt.Errorf("%w: unsupported or nil standalone flag", errUtils.ErrInvalidFlagValue)
		}
		name, short := flag.GetName(), flag.GetShorthand()
		if !validStandaloneFlagName(name) || names[strings.ToLower(name)] {
			return fmt.Errorf("%w: invalid or duplicate standalone flag %q (flag names are case-insensitive)", errUtils.ErrInvalidFlagValue, name)
		}
		if short != "" && (len(short) != 1 || shorts[short] || strings.ContainsAny(short, "- \t\r\n=")) {
			return fmt.Errorf("%w: invalid or duplicate shorthand %q", errUtils.ErrInvalidFlagValue, short)
		}
		names[strings.ToLower(name)], shorts[short] = true, true
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
		if arg == nil || arg.Name == "" || names[strings.ToLower(arg.Name)] {
			return fmt.Errorf("%w: nil, unnamed, or duplicate standalone argument (argument names are case-insensitive)", errUtils.ErrInvalidPositionalArgs)
		}
		if optional && arg.Required {
			return fmt.Errorf("%w: required standalone argument follows an optional argument", errUtils.ErrInvalidPositionalArgs)
		}
		names[strings.ToLower(arg.Name)] = true
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

// Viper's typed getters silently turn malformed environment values into zero values, split lists
// on whitespace, and read integers with base prefixes. Environment values for a flag the command
// line did not set are therefore parsed exactly like command-line values (base-10 integers,
// comma-separated lists with CSV quoting) and handed to Viper as typed values.
func applyStandaloneEnvironment(cmd *cobra.Command, values *viper.Viper, definitions []flags.Flag) error {
	defer perf.Track(nil, "cmd.applyStandaloneEnvironment")()

	for _, flag := range definitions {
		name := flag.GetName()
		raw, exists := standaloneFlagEnvironment(flag)
		if !exists || cmd.Flags().Changed(name) {
			continue
		}
		parsed, err := standaloneEnvironmentValue(cmd, flag, raw)
		if err != nil {
			if !errors.Is(err, errUtils.ErrInvalidFlagValue) {
				err = fmt.Errorf("%w: %w", errUtils.ErrInvalidFlagValue, err)
			}
			return fmt.Errorf("invalid environment value for --%s: %w", name, err)
		}
		if parsed != nil {
			values.Set(name, parsed)
		}
	}
	return nil
}

// standaloneEnvironmentValue parses one environment value. A nil result means Viper's own
// conversion is already correct for the flag's type.
func standaloneEnvironmentValue(cmd *cobra.Command, flag flags.Flag, raw string) (any, error) {
	switch flag.(type) {
	case *flags.IntFlag:
		return standalone.ParseDecimalInt(raw)
	case *flags.StringSliceFlag:
		return standalone.ParseStringList(raw)
	case *flags.BoolFlag:
		return nil, cmd.Flags().Lookup(flag.GetName()).Value.Set(raw)
	default:
		return nil, nil
	}
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
