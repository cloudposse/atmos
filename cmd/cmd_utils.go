package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component/custom"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	pkgFlags "github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/reexec"
	"github.com/cloudposse/atmos/pkg/retry"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

//go:embed markdown/getting_started.md
var gettingStartedMarkdown string

//go:embed markdown/missing_config_default.md
var missingConfigDefaultMarkdown string

//go:embed markdown/missing_config_found.md
var missingConfigFoundMarkdown string

// Define a constant for the dot string that appears multiple times.
const currentDirPath = "."

const (
	customCommandKeyCommand  = "command"
	customCommandKeyIdentity = "identity"
	annotationDefaultChain   = "atmos.custom.default.chain"
)

var errCustomCommandFlagNotRegistered = errors.New("flag is not registered")

// FlagStack is the name of the stack flag used across commands.
const FlagStack = "stack"

// ValidateConfig holds configuration options for Atmos validation.
// CheckStack determines whether stack configuration validation should be performed.
type ValidateConfig struct {
	CheckStack bool
	// Other configuration fields
}

type AtmosValidateOption func(*ValidateConfig)

func WithStackValidation(check bool) AtmosValidateOption {
	return func(cfg *ValidateConfig) {
		cfg.CheckStack = check
	}
}

// processCustomCommands registers custom commands defined in the Atmos configuration onto the given parent Cobra command.
//
// It reads the provided command definitions, reuses any existing top-level commands when appropriate, and adds new Cobra
// commands with their descriptions, persistent flags (including an `--identity` override), required-flag enforcement, and
// nested subcommands. The function mutates parentCommand by attaching the created commands and returns an error if any
// configuration cloning, flag setup, or recursive processing fails.
func processCustomCommands(
	atmosConfig schema.AtmosConfiguration,
	commands []schema.Command,
	parentCommand *cobra.Command,
) error {
	var command *cobra.Command

	for _, commandCfg := range commands {
		// Clone the 'commandCfg' struct into a local variable because of the automatic closure in the `Run` function of the Cobra command.
		// Cloning will make a closure over the local variable 'commandConfig' which is different in each iteration.
		// https://www.calhoun.io/gotchas-and-common-mistakes-with-closures-in-go/
		commandConfig, err := cloneCommand(&commandCfg)
		if err != nil {
			return err
		}

		// Check if the parent already has a subcommand with this name (built-in or previously added).
		// If so, reuse it to allow custom subcommands to merge into built-in namespaces.
		existing := findSubcommand(parentCommand, commandConfig.Name)
		if existing != nil {
			if len(commandConfig.Steps) > 0 {
				// The custom command collides with a built-in and defines steps, which are
				// ignored (built-in behavior wins; see PR #2191). Defer the warning until the
				// conflicting command is actually invoked instead of emitting it here:
				// processCustomCommands runs during root init for nearly every Atmos invocation,
				// so emitting at registration printed the warning even for unrelated commands
				// (e.g. `atmos list stacks`), polluting stderr in scripting and CI.
				// See https://github.com/cloudposse/atmos/issues/2102.
				//
				// TODO(#2102): when `override:`/`invoke:` lands (custom-command-builtin-override PRD),
				// skip this warning for commands that opt into running their steps, since the
				// "custom steps ignored" message would then be wrong.
				warnStepsConflictOnRun(existing, commandConfig.Name)
			}
			command = existing
		} else {
			// Create new custom command with flag validation.
			customCommand, err := createCustomCommand(&atmosConfig, commandConfig, parentCommand)
			if err != nil {
				return err
			}
			parentCommand.AddCommand(customCommand)
			command = customCommand
		}

		err = processCustomCommands(atmosConfig, commandConfig.Commands, command)
		if err != nil {
			return err
		}
	}

	return nil
}

// processCommandAliases registers command aliases from the provided configuration as subcommands of
// parentCommand. When topLevel is true, aliases that would shadow existing top-level commands are
// skipped. Each created alias executes the target command in a separate process using the current
// executable, strips any --chdir / -C flags from the forwarded arguments, and removes ATMOS_CHDIR
// from the child environment to avoid reapplying the parent's working-directory change.
func processCommandAliases(
	atmosConfig schema.AtmosConfiguration,
	aliases schema.CommandAliases,
	parentCommand *cobra.Command,
	topLevel bool,
) error {
	existingTopLevelCommands := make(map[string]*cobra.Command)

	if topLevel {
		existingTopLevelCommands = getTopLevelCommands()
	}

	for k, v := range aliases {
		alias := strings.TrimSpace(k)
		aliasParts, err := shell.Fields(alias, nil)
		if err != nil || len(aliasParts) != 1 {
			continue
		}
		alias = aliasParts[0]

		if existing, exist := existingTopLevelCommands[alias]; exist && topLevel {
			if !isCustomCommand(existing) {
				continue
			}
			parentCommand.RemoveCommand(existing)
		}

		if topLevel {
			aliasCmd := strings.TrimSpace(v)
			aliasFor := fmt.Sprintf("alias for `%s`", aliasCmd)

			aliasCommand := &cobra.Command{
				Use:                alias,
				Short:              aliasFor,
				Long:               aliasFor,
				FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
				Annotations: map[string]string{
					"configAlias": aliasCmd, // marks as CLI config alias, stores target command
				},
				Run: func(cmd *cobra.Command, args []string) {
					err := cmd.ParseFlags(args)
					errUtils.CheckErrorPrintAndExit(err, "", "")

					// Use os.Executable() to get the absolute path to the currently running binary.
					// This ensures that the same binary is used even when invoked via relative paths,
					// symlinks, or from different working directories.
					execPath, err := os.Executable()
					errUtils.CheckErrorPrintAndExit(err, "", "")

					// Filter out --chdir and -C flags from args before passing to the aliased command.
					// The chdir has already been processed by the parent atmos invocation, and passing
					// it again would cause the new process to try to chdir to a relative path that's
					// now invalid (since we already changed directories).
					filteredArgs := reexec.StripChdirArgs(args)

					// Filter out ATMOS_CHDIR from environment variables to prevent the child process
					// from re-applying the parent's chdir directive.
					filteredEnv := reexec.FilterChdirEnv(os.Environ())

					// Build command arguments: split aliasCmd into parts and append filteredArgs.
					// Use direct process execution instead of shell to avoid path escaping
					// issues on Windows where backslashes in paths are misinterpreted.
					cmdArgs, err := shell.Fields(aliasCmd, nil)
					errUtils.CheckErrorPrintAndExit(err, "", "")
					cmdArgs = append(cmdArgs, filteredArgs...)

					execCmd := exec.Command(execPath, cmdArgs...)
					execCmd.Dir = currentDirPath
					execCmd.Env = filteredEnv
					execCmd.Stdin = os.Stdin
					execCmd.Stdout = os.Stdout
					execCmd.Stderr = os.Stderr

					err = execCmd.Run()
					if err != nil {
						var exitErr *exec.ExitError
						if errors.As(err, &exitErr) {
							// Preserve the subprocess exit code.
							err = errUtils.WithExitCode(err, exitErr.ExitCode())
						}
						errUtils.CheckErrorPrintAndExit(err, "", "")
					}
				},
			}

			aliasCommand.DisableFlagParsing = true

			// Add the alias to the parent command
			parentCommand.AddCommand(aliasCommand)
		}
	}

	return nil
}

// preCustomCommand is run before a custom command is executed.
func preCustomCommand(
	cmd *cobra.Command,
	args []string,
	parentCommand *cobra.Command,
	commandConfig *schema.Command,
) {
	var sb strings.Builder

	if shouldRunDefaultSubcommand(args, commandConfig) {
		return
	}

	// checking for zero arguments in config
	if len(commandConfig.Arguments) == 0 {
		if len(commandConfig.Steps) > 0 {
			// do nothing here; let the code proceed
		} else if len(commandConfig.Commands) > 0 {
			// show sub-commands
			sb.WriteString("Available command(s):\n")
			for i, c := range commandConfig.Commands {
				sb.WriteString(
					fmt.Sprintf("%d. %s %s %s\n", i+1, parentCommand.Use, commandConfig.Name, c.Name),
				)
			}
			ui.Writeln(sb.String())
			errUtils.Exit(1)
		} else {
			// truly invalid, nothing to do
			er := errors.New(fmt.Sprintf("The `%s` command has no steps or subcommands configured.", cmd.CommandPath()))
			errUtils.CheckErrorPrintAndExit(er, "Invalid Command", "https://atmos.tools/cli/configuration/commands")
		}
	}

	// Check on many arguments required and have no default value
	requiredNoDefaultCount := 0
	for _, arg := range commandConfig.Arguments {
		if arg.Required && arg.Default == "" {
			requiredNoDefaultCount++
		}
	}

	// Check if the number of arguments provided is less than the required number of arguments
	if len(args) < requiredNoDefaultCount {
		sb.WriteString(
			fmt.Sprintf("Command requires at least %d argument(s) (no defaults provided for them):\n",
				requiredNoDefaultCount),
		)

		// List out which arguments are missing
		missingIndex := 1
		for _, arg := range commandConfig.Arguments {
			if arg.Required && arg.Default == "" {
				sb.WriteString(fmt.Sprintf("  %d. %s\n", missingIndex, arg.Name))
				missingIndex++
			}
		}
		if len(args) > 0 {
			sb.WriteString(fmt.Sprintf("\nReceived %d argument(s): %s\n", len(args), strings.Join(args, ", ")))
		}
		errUtils.CheckErrorPrintAndExit(errors.New(sb.String()), "", "")
	}

	// Merge user-supplied arguments with defaults
	finalArgs := make([]string, len(commandConfig.Arguments))

	for i, arg := range commandConfig.Arguments {
		if i < len(args) {
			finalArgs[i] = args[i]
		} else {
			if arg.Default != "" {
				finalArgs[i] = fmt.Sprintf("%v", arg.Default)
			} else {
				// This theoretically shouldn't happen:
				sb.WriteString(fmt.Sprintf("Missing required argument '%s' with no default!\n", arg.Name))
				errUtils.CheckErrorPrintAndExit(errors.New(sb.String()), "", "")
			}
		}
	}
	// Set the resolved arguments as annotations on the command
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations["resolvedArgs"] = strings.Join(finalArgs, ",")

	// no "steps" means a sub command should be specified
	if len(commandConfig.Steps) == 0 {
		if err := cmd.Help(); err != nil {
			log.Trace("Failed to display command help", "error", err, customCommandKeyCommand, cmd.Name())
		}
		errUtils.Exit(0)
	}
}

func shouldRunDefaultSubcommand(args []string, commandConfig *schema.Command) bool {
	return len(args) == 0 &&
		strings.TrimSpace(commandConfig.Default) != "" &&
		len(commandConfig.Commands) > 0
}

func runDefaultSubcommand(cmd *cobra.Command, commandConfig *schema.Command) bool {
	defaultName := strings.TrimSpace(commandConfig.Default)
	if defaultName == "" {
		return false
	}

	chain := defaultSubcommandChain(cmd)
	if Contains(chain, commandConfig.Name) {
		err := errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanationf("Custom command %q has a recursive default subcommand chain", commandConfig.Name).
			WithHint("Remove the self-reference or cycle from the command's `default` settings").
			WithContext(customCommandKeyCommand, commandConfig.Name).
			WithContext("default", defaultName).
			Err()
		errUtils.CheckErrorPrintAndExit(err, "Invalid Command", "https://atmos.tools/cli/configuration/commands")
		return true
	}

	subcommand := findSubcommand(cmd, defaultName)
	if subcommand == nil {
		err := errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanationf("Custom command %q declares default subcommand %q, but no matching subcommand exists", commandConfig.Name, defaultName).
			WithHint("Set `default` to one of the command's configured subcommands").
			WithContext(customCommandKeyCommand, commandConfig.Name).
			WithContext("default", defaultName).
			Err()
		errUtils.CheckErrorPrintAndExit(err, "Invalid Command", "https://atmos.tools/cli/configuration/commands")
		return true
	}

	_ = subcommand.InheritedFlags()
	restoreChain := restoreDefaultSubcommandChain(subcommand, append(chain, commandConfig.Name))
	defer restoreChain()

	if subcommand.PreRunE != nil {
		errUtils.CheckErrorPrintAndExit(subcommand.PreRunE(subcommand, nil), "", "")
	} else if subcommand.PreRun != nil {
		subcommand.PreRun(subcommand, nil)
	}

	if subcommand.RunE != nil {
		errUtils.CheckErrorPrintAndExit(subcommand.RunE(subcommand, nil), "", "")
	} else if subcommand.Run != nil {
		subcommand.Run(subcommand, nil)
	} else if err := subcommand.Help(); err != nil {
		log.Trace("Failed to display default subcommand help", "error", err, customCommandKeyCommand, subcommand.Name())
	}

	return true
}

func defaultSubcommandChain(cmd *cobra.Command) []string {
	if cmd.Annotations == nil {
		return nil
	}
	raw := strings.TrimSpace(cmd.Annotations[annotationDefaultChain])
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func restoreDefaultSubcommandChain(cmd *cobra.Command, chain []string) func() {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	previous, hadPrevious := cmd.Annotations[annotationDefaultChain]
	cmd.Annotations[annotationDefaultChain] = strings.Join(chain, "\n")
	return func() {
		if hadPrevious {
			cmd.Annotations[annotationDefaultChain] = previous
		} else {
			delete(cmd.Annotations, annotationDefaultChain)
		}
	}
}

// findSubcommand returns the existing subcommand of parent with the given name, or nil.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// warnStepsConflictOnRun arranges for the "custom steps ignored" collision warning to be
// emitted only when the conflicting built-in command cmd is actually run, rather than at
// registration time. It wraps cmd.PreRunE (preserving any existing PreRunE/PreRun and honoring
// Cobra's precedence of PreRunE over PreRun) so the warning surfaces once, at the point the user
// invokes the colliding command. See https://github.com/cloudposse/atmos/issues/2102.
func warnStepsConflictOnRun(cmd *cobra.Command, customName string) {
	commandPath := cmd.CommandPath()
	prevPreRunE := cmd.PreRunE
	prevPreRun := cmd.PreRun

	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		ui.Warningf(
			"Custom command %q defines steps that conflict with built-in command %q; "+
				"built-in behavior preserved, custom steps ignored",
			customName, commandPath,
		)

		switch {
		case prevPreRunE != nil:
			return prevPreRunE(c, args)
		case prevPreRun != nil:
			prevPreRun(c, args)
		}
		return nil
	}
}

// createCustomCommand creates a new cobra command with flags from commandConfig,
// validating flag names and types for conflicts with parent command flags.
func createCustomCommand(
	atmosConfig *schema.AtmosConfiguration,
	commandConfig *schema.Command,
	parentCommand *cobra.Command,
) (*cobra.Command, error) {
	customCommand := &cobra.Command{
		Use:   commandConfig.Name,
		Short: commandConfig.Description,
		Long:  commandConfig.Description,
		Args:  customCommandArgsValidator(commandConfig),
		Annotations: map[string]string{
			annotationCustomCommand: annotationValueTrue,
		},
		PreRun: func(cmd *cobra.Command, args []string) {
			preCustomCommand(cmd, args, parentCommand, commandConfig)
		},
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 && runDefaultSubcommand(cmd, commandConfig) {
				return
			}
			executeCustomCommand(*atmosConfig, cmd, args, parentCommand, commandConfig)
		},
	}
	customCommand.PersistentFlags().Bool("", false, doubleDashHint)

	// Add --identity flag to all custom commands to allow runtime override.
	customCommand.PersistentFlags().String(customCommandKeyIdentity, "", "Identity to use for authentication (overrides identity in command config)")
	AddIdentityCompletion(customCommand)

	if err := validateCustomCommandFlags(commandConfig, parentCommand); err != nil {
		return nil, err
	}

	if err := registerCustomCommandFlags(commandConfig, customCommand, parentCommand); err != nil {
		return nil, err
	}

	// Set ValidArgsFunction for first semantic-typed positional argument.
	setSemanticArgCompletion(customCommand, commandConfig)

	// Register completion for semantic-typed flags.
	registerSemanticFlagCompletions(customCommand, commandConfig)

	return customCommand, nil
}

func customCommandArgsValidator(commandConfig *schema.Command) cobra.PositionalArgs {
	if len(commandConfig.Arguments) == 0 {
		return pkgFlags.SeparatorAwareValidator(cobra.NoArgs)
	}

	requiredNoDefaultCount := 0
	for _, arg := range commandConfig.Arguments {
		if arg.Required && arg.Default == "" {
			requiredNoDefaultCount++
		}
	}

	return pkgFlags.SeparatorAwareValidator(cobra.RangeArgs(requiredNoDefaultCount, len(commandConfig.Arguments)))
}

// validateCustomCommandFlags checks for duplicate flags and type conflicts
// between the custom command's flags and parent/inherited flags.
func validateCustomCommandFlags(commandConfig *schema.Command, parentCommand *cobra.Command) error {
	seen := make(map[string]bool)
	for i := range commandConfig.Flags {
		if err := validateFlag(commandConfig.Name, &commandConfig.Flags[i], seen, parentCommand); err != nil {
			return err
		}
	}
	return nil
}

// validateFlag checks a single flag for duplicates and type conflicts with parent flags.
func validateFlag(cmdName string, flag *schema.CommandFlag, seen map[string]bool, parentCommand *cobra.Command) error {
	// Detect duplicates within the same command config early.
	if seen[flag.Name] {
		return errUtils.Build(errUtils.ErrDuplicateFlagRegistration).
			WithExplanation(fmt.Sprintf("Custom command '%s' defines duplicate flag '--%s'", cmdName, flag.Name)).
			WithHint("Remove or rename the duplicate flag in your atmos.yaml").
			WithContext(customCommandKeyCommand, cmdName).
			WithContext("flag", flag.Name).
			Err()
	}
	seen[flag.Name] = true

	// Check if this flag already exists on parent or globally.
	// Only check PersistentFlags() and InheritedFlags() - NOT Flags().
	// Local flags (Flags()) on the parent are NOT inherited by subcommands,
	// so they shouldn't block subcommands from defining the same flag.
	existingFlag := parentCommand.PersistentFlags().Lookup(flag.Name)
	if existingFlag == nil {
		existingFlag = parentCommand.InheritedFlags().Lookup(flag.Name)
	}

	if existingFlag != nil {
		return validateFlagTypeConflict(cmdName, flag, existingFlag)
	}

	// Flag doesn't exist yet - validate shorthand for new flags only.
	return validateFlagShorthand(cmdName, flag, seen, parentCommand)
}

// validateFlagTypeConflict checks that a custom flag's type matches an existing flag's type.
func validateFlagTypeConflict(cmdName string, flag *schema.CommandFlag, existingFlag *pflag.Flag) error {
	customFlagType := flag.Type
	if customFlagType == "" || customFlagType == "string" {
		customFlagType = "string"
	}

	existingFlagType := existingFlag.Value.Type()
	if existingFlagType != customFlagType {
		return errUtils.Build(errUtils.ErrReservedFlagName).
			WithExplanation(fmt.Sprintf("Custom command '%s' in atmos.yaml declares flag '--%s' with type '%s', but it already exists with type '%s'",
				cmdName, flag.Name, customFlagType, existingFlagType)).
			WithHint("Check the 'commands' section in atmos.yaml").
			WithHint("Either use the existing flag type, or rename your flag to avoid conflicts").
			WithContext(customCommandKeyCommand, cmdName).
			WithContext("flag", flag.Name).
			WithContext("declared_type", customFlagType).
			WithContext("existing_type", existingFlagType).
			WithContext("config_path", fmt.Sprintf("commands.%s.flags", cmdName)).
			Err()
	}
	return nil
}

// validateFlagShorthand checks for shorthand conflicts with parent/inherited flags.
func validateFlagShorthand(cmdName string, flag *schema.CommandFlag, seen map[string]bool, parentCommand *cobra.Command) error {
	if flag.Shorthand == "" {
		return nil
	}

	if len([]rune(flag.Shorthand)) != 1 {
		return errUtils.Build(errUtils.ErrDuplicateFlagRegistration).
			WithExplanation(fmt.Sprintf("Custom command '%s' defines invalid shorthand '-%s' for flag '--%s'", cmdName, flag.Shorthand, flag.Name)).
			WithHint("Use exactly one character for shorthand flags").
			WithContext(customCommandKeyCommand, cmdName).
			WithContext("flag", flag.Name).
			WithContext("shorthand", flag.Shorthand).
			Err()
	}

	if seen[flag.Shorthand] {
		return errUtils.Build(errUtils.ErrDuplicateFlagRegistration).
			WithExplanation(fmt.Sprintf("Custom command '%s' defines duplicate flag shorthand '-%s'", cmdName, flag.Shorthand)).
			WithHint("Remove or change the duplicate shorthand in your atmos.yaml").
			WithContext(customCommandKeyCommand, cmdName).
			WithContext("shorthand", flag.Shorthand).
			Err()
	}
	seen[flag.Shorthand] = true

	// Check if shorthand conflicts with existing persistent/inherited flags.
	// Only check PersistentFlags() and InheritedFlags() - NOT Flags().
	// Local flags (Flags()) on the parent are NOT inherited by subcommands.
	existingByShorthand := parentCommand.PersistentFlags().ShorthandLookup(flag.Shorthand)
	if existingByShorthand == nil {
		existingByShorthand = parentCommand.InheritedFlags().ShorthandLookup(flag.Shorthand)
	}
	if existingByShorthand != nil {
		return errUtils.Build(errUtils.ErrReservedFlagName).
			WithExplanation(fmt.Sprintf("Custom command '%s' in atmos.yaml defines flag shorthand '-%s' which conflicts with existing flag '--%s'",
				cmdName, flag.Shorthand, existingByShorthand.Name)).
			WithHint("Check the 'commands' section in atmos.yaml").
			WithHint("Change the shorthand to avoid conflicts").
			WithContext(customCommandKeyCommand, cmdName).
			WithContext("shorthand", flag.Shorthand).
			WithContext("existing_flag", existingByShorthand.Name).
			WithContext("config_path", fmt.Sprintf("commands.%s.flags", cmdName)).
			Err()
	}
	return nil
}

// registerCustomCommandFlags adds validated flags to the custom command,
// skipping flags that are inherited from the parent command chain.
func registerCustomCommandFlags(commandConfig *schema.Command, customCommand *cobra.Command, parentCommand *cobra.Command) error {
	for i := range commandConfig.Flags {
		flag := &commandConfig.Flags[i]
		// Skip flags that already exist as persistent/inherited (not local).
		existingFlag := parentCommand.PersistentFlags().Lookup(flag.Name)
		if existingFlag == nil {
			existingFlag = parentCommand.InheritedFlags().Lookup(flag.Name)
		}
		if existingFlag != nil {
			continue
		}

		registerFlag(customCommand, flag)

		if flag.Required {
			if err := customCommand.MarkPersistentFlagRequired(flag.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// registerFlag adds a single flag to the command with the appropriate type and defaults.
func registerFlag(cmd *cobra.Command, flag *schema.CommandFlag) {
	// Get flag description, preferring Description over Usage for backward compatibility.
	flagUsage := flag.Description
	if flagUsage == "" {
		flagUsage = flag.Usage
	}

	if flag.Type == "bool" {
		registerBoolFlag(cmd, flag, flagUsage)
		return
	}
	registerStringFlag(cmd, flag, flagUsage)
}

// registerBoolFlag adds a boolean persistent flag to the command.
func registerBoolFlag(cmd *cobra.Command, flag *schema.CommandFlag, usage string) {
	defaultVal := false
	if flag.Default != nil {
		if boolVal, ok := flag.Default.(bool); ok {
			defaultVal = boolVal
		}
	}
	if flag.Shorthand != "" {
		cmd.PersistentFlags().BoolP(flag.Name, flag.Shorthand, defaultVal, usage)
	} else {
		cmd.PersistentFlags().Bool(flag.Name, defaultVal, usage)
	}
}

// registerStringFlag adds a string persistent flag to the command.
func registerStringFlag(cmd *cobra.Command, flag *schema.CommandFlag, usage string) {
	defaultVal := ""
	if flag.Default != nil {
		if strVal, ok := flag.Default.(string); ok {
			defaultVal = strVal
		}
	}
	if flag.Shorthand != "" {
		cmd.PersistentFlags().StringP(flag.Name, flag.Shorthand, defaultVal, usage)
	} else {
		cmd.PersistentFlags().String(flag.Name, defaultVal, usage)
	}
}

// getTopLevelCommands returns the top-level commands.
func getTopLevelCommands() map[string]*cobra.Command {
	existingTopLevelCommands := make(map[string]*cobra.Command)

	for _, c := range RootCmd.Commands() {
		existingTopLevelCommands[c.Name()] = c
	}

	return existingTopLevelCommands
}

// executeCustomCommand executes a custom command.
func executeCustomCommand(
	atmosConfig schema.AtmosConfiguration,
	cmd *cobra.Command,
	args []string,
	parentCommand *cobra.Command,
	commandConfig *schema.Command,
) {
	var err error

	// Extract arguments after "--" separator using safe shell quoting.
	separated := ExtractSeparatedArgs(cmd, args, os.Args)
	args = separated.BeforeSeparator
	trailingArgs, err := separated.GetAfterSeparatorAsQuotedString()
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: failed to quote trailing arguments: %w",
			errUtils.ErrFailedToProcessArgs, err), "", "")
	}

	if commandConfig.Verbose {
		atmosConfig.Logs.Level = u.LogLevelTrace
	}

	mergedArgsStr := cmd.Annotations["resolvedArgs"]
	finalArgs := strings.Split(mergedArgsStr, ",")
	if mergedArgsStr == "" {
		// If for some reason no annotation was set, just fallback
		finalArgs = args
	}

	commandConditionEnv := envpkg.EnvironToMap()
	if commandConditionEnv == nil {
		commandConditionEnv = make(map[string]string)
	}
	for key, value := range envpkg.CommandEnvToMap(commandConfig.Env) {
		commandConditionEnv[key] = value
	}
	hasRunnableStep := false
	for i := range commandConfig.Steps {
		step := &commandConfig.Steps[i]
		if err := schema.ValidateStepCondition(step.When); err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
		runs, err := step.When.EvaluateWithImplicitSuccessE(customCommandConditionContext(commandConfig.Name, step, i, commandConditionEnv, schema.ConditionPredicateSuccess))
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
		if runs {
			hasRunnableStep = true
			break
		}
	}
	if !hasRunnableStep {
		log.Debug("Skipping custom command, no steps matched `when` conditions", customCommandKeyCommand, commandConfig.Name)
		return
	}

	// Resolve and install dependencies declared by this command.
	resolver := dependencies.NewResolver(&atmosConfig)

	// Get command-specific dependencies.
	deps, err := resolver.ResolveCommandDependencies(commandConfig)
	if err != nil {
		err = errUtils.Build(errUtils.ErrDependencyResolution).
			WithCause(err).
			WithExplanationf("Failed to resolve dependencies for command '%s'", commandConfig.Name).
			WithHint("Check the command's dependencies section for valid tool specifications").
			WithHint("See https://atmos.tools/cli/commands/toolchain/ for toolchain configuration").
			Err()
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	if len(deps) > 0 {
		installer := dependencies.NewInstaller(&atmosConfig)
		if err := installer.EnsureTools(deps); err != nil {
			err = errUtils.Build(errUtils.ErrDependencyResolution).
				WithCause(err).
				WithExplanationf("Failed to install dependencies for command '%s'", commandConfig.Name).
				WithHint("Check the command's dependencies section for valid tool specifications").
				WithHint("See https://atmos.tools/cli/commands/toolchain/ for toolchain configuration").
				Err()
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}

		log.Debug("Adding configured command dependencies to PATH", customCommandKeyCommand, commandConfig.Name, "tools", deps)
		if err := dependencies.UpdatePathForTools(&atmosConfig, deps); err != nil {
			err = errUtils.Build(errUtils.ErrDependencyResolution).
				WithCause(err).
				WithExplanationf("Failed to update PATH for command '%s'", commandConfig.Name).
				WithHint("Run `atmos toolchain install` to install tools from .tool-versions").
				WithHint("See https://atmos.tools/cli/commands/toolchain/ for toolchain configuration").
				Err()
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
	}

	// Create auth manager if identity is specified for this custom command and
	// at least one step will run.
	// Check for --identity flag first (it overrides the config).
	identityFlag, _ := cmd.Flags().GetString(customCommandKeyIdentity)
	commandIdentity := strings.TrimSpace(identityFlag)
	if commandIdentity == "" {
		// Fall back to identity from command config
		commandIdentity = strings.TrimSpace(commandConfig.Identity)
	}

	authManager := prepareCustomCommandAuth(&atmosConfig, commandIdentity, commandConfig.Name, hasRunnableStep)

	// Determine working directory for command execution.
	workDir, err := resolveWorkingDirectory(commandConfig.WorkingDirectory, atmosConfig.BasePath, currentDirPath)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "Invalid working_directory", "https://atmos.tools/cli/configuration/commands/working-directory")
	}
	if commandConfig.WorkingDirectory != "" {
		log.Debug("Using working directory for custom command", customCommandKeyCommand, commandConfig.Name, "working_directory", workDir)
	}

	// Validate exec steps before executing anything: an exec step replaces
	// the Atmos process, so it must be the final step and must not set
	// supervisor-only fields (tty, interactive, retry, timeout, output).
	if err := schema.ValidateExecTasks(commandConfig.Steps); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "https://atmos.tools/cli/configuration/commands/steps#interactive-and-tty-steps")
	}

	// Initialize step executor once before loop - reused across steps to preserve outputs.
	executor := stepPkg.NewStepExecutor()
	stepVars := executor.Variables()
	stepVars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		return e.ProcessTmpl(&atmosConfig, name, input, data, false)
	})
	stepVars.SetTemplatePasses(3)
	stepVars.ProtectTemplateRoots("Arguments", "Flags", "flags", "TrailingArgs")

	// Execute custom command's steps
	var commandErr error
	conditionStatus := schema.ConditionPredicateSuccess
	for i, step := range commandConfig.Steps {
		runs, err := step.When.EvaluateWithImplicitSuccessE(customCommandConditionContext(commandConfig.Name, &step, i, commandConditionEnv, conditionStatus))
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
		if !runs {
			log.Debug("Skipping custom command step, `when` condition did not match", customCommandKeyCommand, commandConfig.Name, "step", i)
			continue
		}

		// Prepare template data for arguments
		argumentsData := map[string]string{}
		for ix, arg := range commandConfig.Arguments {
			argumentsData[arg.Name] = finalArgs[ix]
		}

		// Prepare template data for flags
		flagsData := map[string]any{}
		for _, fl := range commandConfig.Flags {
			flag := cmd.Flag(fl.Name)
			if flag == nil {
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %q", errCustomCommandFlagNotRegistered, fl.Name), "", "")
			}
			if fl.Type == "" || fl.Type == "string" {
				flagsData[fl.Name] = flag.Value.String()
			} else if fl.Type == "bool" {
				boolFlag, err := strconv.ParseBool(flag.Value.String())
				errUtils.CheckErrorPrintAndExit(err, "", "")
				flagsData[fl.Name] = boolFlag
			}
		}

		// Prompt for missing semantic-typed values if interactive mode is enabled.
		// This enables interactive selection for custom commands with component/stack arguments.
		promptForSemanticValues(cmd, commandConfig, argumentsData, flagsData, nil)

		// Prepare template data
		data := map[string]any{
			"Arguments":    argumentsData,
			"Cwd":          workDir,
			"cwd":          workDir,
			"Flags":        flagsData,
			"flags":        flagsData,
			"TrailingArgs": trailingArgs,
		}

		// If the custom command defines 'component' section with a custom component type,
		// register the type, resolve component config, and expose it in {{ .Component.* }} Go template variables.
		if commandConfig.Component != nil && commandConfig.Component.Type != "" {
			processCustomComponentType(&atmosConfig, commandConfig, argumentsData, flagsData, data, authManager)
		} else if commandConfig.ComponentConfig.Component != "" && commandConfig.ComponentConfig.Stack != "" {
			// Legacy: If the custom command defines 'component_config' section with 'component' and 'stack' attributes,
			// process the component stack config and expose it in {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables.
			// Process Go templates in the command's 'component_config.component'.
			component, err := e.ProcessTmpl(&atmosConfig, fmt.Sprintf("component-config-component-%d", i), commandConfig.ComponentConfig.Component, data, false)
			errUtils.CheckErrorPrintAndExit(err, "", "")
			if component == "" || component == "<no value>" {
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("the command defines an invalid 'component_config.component: %s' in '%s'",
					commandConfig.ComponentConfig.Component, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension), "", "")
			}

			// Process Go templates in the command's 'component_config.stack'.
			stack, err := e.ProcessTmpl(&atmosConfig, fmt.Sprintf("component-config-stack-%d", i), commandConfig.ComponentConfig.Stack, data, false)
			errUtils.CheckErrorPrintAndExit(err, "", "")
			if stack == "" || stack == "<no value>" {
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("the command defines an invalid 'component_config.stack: %s' in '%s'",
					commandConfig.ComponentConfig.Stack, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension), "", "")
			}

			// Get the config for the component in the stack.
			componentConfig, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
				Component:            component,
				Stack:                stack,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
				Skip:                 nil,
				AuthManager:          authManager,
			})
			errUtils.CheckErrorPrintAndExit(err, "", "")
			data["ComponentConfig"] = componentConfig
		}

		// Prepare ENV vars
		// ENV var values support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		// Start with current environment + global env from atmos.yaml to inherit PATH and other variables.
		env := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)

		// Expose the absolute path of the running atmos binary so custom command steps can
		// re-invoke the SAME binary (e.g. `"${ATMOS_CLI_PATH:-atmos}" describe ...`) instead of
		// relying on a possibly-stale or absent `atmos` on PATH.
		if execPath, execErr := os.Executable(); execErr == nil {
			env = envpkg.UpdateEnvVar(env, "ATMOS_CLI_PATH", execPath)
			// Also prepend the running binary's directory to PATH so custom command steps can
			// invoke a bare `atmos` and resolve to the SAME binary — even when atmos isn't on
			// the caller's PATH (e.g. `./build/atmos <cmd>`). Keeps example steps readable.
			env = envpkg.EnsureBinaryInPath(env, execPath)
		}
		stepVars.SetTemplateData(data)
		for _, envVar := range env {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				stepVars.SetEnv(parts[0], parts[1])
			}
		}

		// Export the custom component's resolved `env` section into the step subprocess
		// environment, mirroring the built-in terraform/helmfile/packer providers. Without this,
		// a custom component's `env` section (including resolved `!secret` values) would only be
		// available as `{{ .Component.env.* }}` template data and never reach the step subprocess.
		// This runs before the command-level `env:` loop below so command-defined vars take
		// precedence on key collisions.
		if commandConfig.Component != nil && commandConfig.Component.Type != "" {
			if componentConfig, ok := data["Component"].(map[string]any); ok {
				env = appendComponentEnvVars(env, componentConfig)
			}
		}

		for _, v := range commandConfig.Env {
			key := strings.TrimSpace(v.Key)
			value := v.Value
			valCommand := v.ValueCommand

			if value != "" && valCommand != "" {
				err = fmt.Errorf("either 'value' or 'valueCommand' can be specified for the ENV var, but not both.\n"+
					"Custom command '%s %s' defines 'value=%s' and 'valueCommand=%s' for the ENV var '%s'",
					parentCommand.Name(), commandConfig.Name, value, valCommand, key)
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			// If the command to get the value for the ENV var is provided, execute it
			if valCommand != "" {
				valCommandName := fmt.Sprintf("env-var-%s-valcommand", key)
				res, err := u.ExecuteShellAndReturnOutput(valCommand, valCommandName, workDir, env, false)
				errUtils.CheckErrorPrintAndExit(err, "", "")
				value = strings.TrimRight(res, "\r\n")
			} else {
				// Process Go templates in the values of the command's ENV vars
				value, err = stepVars.Resolve(value)
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			// Add or update the environment variable in the env slice
			env = envpkg.UpdateEnvVar(env, key, value)
			stepVars.SetEnv(key, value)
		}

		// A command-level env can override PATH (e.g. rebuilding it from the
		// caller's environment); re-ensure the running binary's directory so a
		// bare `atmos` in steps still resolves to the SAME binary even when
		// atmos isn't installed on the system PATH (fresh CI runners).
		if execPath, execErr := os.Executable(); execErr == nil {
			env = envpkg.EnsureBinaryInPath(env, execPath)
			stepVars.EnsureBinaryInPath(execPath)
		}

		if len(commandConfig.Env) > 0 && commandConfig.Verbose {
			var envVarsList []string
			for _, v := range commandConfig.Env {
				envVarsList = append(envVarsList, fmt.Sprintf("%s=%s", strings.TrimSpace(v.Key), "***"))
			}
			log.Debug("Using custom ENV vars", "env", envVarsList)
		}

		// Prepare shell environment with authentication credentials if identity is specified.
		if commandIdentity != "" && authManager != nil {
			ctx := context.Background()
			env, err = authManager.PrepareShellEnvironment(ctx, commandIdentity, env)
			if err != nil {
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("failed to prepare shell environment for identity %q in custom command %q step %d: %w",
					commandIdentity, commandConfig.Name, i, err), "", "")
			}
			log.Debug("Prepared environment with identity for custom command step", customCommandKeyIdentity, commandIdentity, customCommandKeyCommand, commandConfig.Name, "step", i)
		}
		for _, envVar := range env {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				stepVars.SetEnv(parts[0], parts[1])
			}
		}

		stepEnv := step.Env
		if atmosConfig.CaseMaps != nil {
			stepEnv = atmosConfig.CaseMaps.ApplyCase("env", stepEnv)
		}
		if len(stepEnv) > 0 {
			resolvedStepEnv, resolveErr := stepVars.ResolveEnvMap(stepEnv)
			errUtils.CheckErrorPrintAndExit(resolveErr, "", "")
			for key, value := range resolvedStepEnv {
				env = envpkg.UpdateEnvVar(env, key, value)
			}
		}

		// Determine step type - default to shell if not specified.
		stepType := strings.TrimSpace(step.Type)
		if stepType == "" {
			stepType = "shell"
		}

		// If this step will be enclosed in a CI log group, mark the subprocess
		// environment so a nested `atmos` invocation skips unsupported nested
		// grouping.
		if stepType != schema.TaskTypeExec && ci.ShouldPropagateLogGroupSentinel(&atmosConfig, ci.DimensionStep) {
			sentinel := ci.LogGroupSentinelEnv()
			env = append(env, sentinel)
			if key, value, ok := strings.Cut(sentinel, "="); ok {
				stepVars.SetEnv(key, value)
			}
		}

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables.
		commandToRun, err := stepVars.Resolve(step.Command)
		errUtils.CheckErrorPrintAndExit(err, "", "")

		stepWorkDir := workDir
		if strings.TrimSpace(step.WorkingDirectory) != "" {
			stepWorkDir, err = resolveWorkingDirectory(step.WorkingDirectory, workDir, workDir)
			errUtils.CheckErrorPrintAndExit(err, "Invalid working_directory", "https://atmos.tools/cli/configuration/commands/working-directory")
		}

		// Execute the step based on type.
		//
		// shell/exec/atmos use the legacy, cross-platform, child-reaping paths
		// below; only genuinely-extended step types (container, input, confirm,
		// …) route through the registered step handlers via the default case.
		// Routing shell/atmos through the handlers regressed Windows (handlers
		// hardcode `sh -c` → exit 126) and leaked orphaned `atmos` child
		// processes on Linux (no process-group cleanup), so they stay on the
		// legacy paths.
		//
		// The dispatch is wrapped in a collapsible CI log group (no-op outside
		// CI / when disabled), labeled with the step name or command. Exec steps
		// run bare because a successful Unix exec never returns to close a group.
		runStep := func() error {
			switch stepType {
			case "shell":
				// Execute shell command (backward compatible).
				// Steps with tty/interactive attach the user's terminal so commands
				// like `aws ssm start-session` get a real TTY and own Ctrl-C.
				commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)
				return process.RunShellStep(context.Background(), &process.ShellSessionSpec{
					Command:     commandToRun,
					Name:        commandName,
					Dir:         stepWorkDir,
					Env:         env,
					TTY:         step.Tty,
					Interactive: step.Interactive,
				}, func() error {
					if step.Output == string(stepPkg.OutputModeNone) {
						return e.ExecuteShellWithWriters(&e.ExecuteShellSpec{
							Command: commandToRun,
							Name:    commandName,
							Dir:     stepWorkDir,
							EnvVars: env,
							Stdout:  io.Discard,
							Stderr:  io.Discard,
						})
					}
					return e.ExecuteShell(commandToRun, commandName, stepWorkDir, env, false)
				})
			case schema.TaskTypeExec:
				// Replace the Atmos process with the command (shell exec semantics).
				return process.ReplaceShellSession(&process.ExecSpec{
					Command: commandToRun,
					Name:    fmt.Sprintf("%s-step-%d", commandConfig.Name, i),
					Dir:     stepWorkDir,
					Env:     env,
				})
			case "atmos":
				// Execute atmos command.
				args := strings.Fields(commandToRun)
				execPath, execErr := os.Executable()
				if execErr != nil {
					return execErr
				}
				return e.ExecuteShellCommand(atmosConfig, execPath, args, stepWorkDir, env, false, "")
			default:
				// Check if this is an extended step type (input, confirm, choose, etc.).
				if stepPkg.IsExtendedStepType(stepType) {
					// Convert Task to WorkflowStep for handler compatibility.
					workflowStep := step.ToWorkflowStep()
					// Carry env onto the step so handlers that read step.Env (e.g. the
					// container handler's in-container env) see it. The step's own
					// declared `env:` had its map keys lowercased by Viper, so restore
					// the original case from the shared env case map, then merge it over
					// the resolved command/process env (step vars win on collisions).
					stepOwnEnv := workflowStep.Env
					if atmosConfig.CaseMaps != nil {
						stepOwnEnv = atmosConfig.CaseMaps.ApplyCase("env", stepOwnEnv)
					}
					mergedStepEnv := envpkg.SliceToMap(env)
					for key, value := range stepOwnEnv {
						mergedStepEnv[key] = value
					}
					workflowStep.Env = mergedStepEnv
					workflowStep.WorkingDirectory = stepWorkDir

					if stack, ok := flagsData["stack"].(string); ok && stack != "" {
						executor.SetFlag("stack", stack)
					}

					// Execute the extended step.
					_, execErr := executor.Execute(context.Background(), &workflowStep)
					return execErr
				}
				return fmt.Errorf("%w: unsupported step type %q for custom command step %d", errUtils.ErrInvalidWorkflowStepType, stepType, i)
			}
		}
		err = stepPkg.RunGroupedForType(&atmosConfig, step.Name, commandToRun, stepType, func() error {
			if step.Retry != nil {
				return retry.Do(context.Background(), step.Retry, runStep)
			}
			return runStep()
		})
		if err != nil {
			var silentExit errUtils.ExitCodeError
			if errors.As(err, &silentExit) && silentExit.Silent {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}
			if commandErr == nil {
				commandErr = err
			} else {
				commandErr = errors.Join(commandErr, err)
			}
			conditionStatus = schema.ConditionPredicateFailure
		}
	}
	errUtils.CheckErrorPrintAndExit(commandErr, "", "")
}

func customCommandConditionContext(commandName string, step *schema.Task, index int, env map[string]string, status string) schema.ConditionContext {
	stepName := ""
	stack := ""
	stepEnv := env
	if step != nil {
		stepName = step.Name
		stack = step.Stack
		if len(step.Env) > 0 {
			stepEnv = make(map[string]string, len(env))
			for key, value := range env {
				stepEnv[key] = value
			}
			for key, value := range step.Env {
				stepEnv[key] = value
			}
		}
	}
	if stepName == "" {
		stepName = fmt.Sprintf("step-%d", index)
	}
	return schema.ConditionContext{
		CI:       telemetry.IsCI(),
		Status:   status,
		Stack:    stack,
		Workflow: commandName,
		Step:     stepName,
		Env:      stepEnv,
	}
}

func prepareCustomCommandAuth(atmosConfig *schema.AtmosConfiguration, commandIdentity, commandName string, hasRunnableStep bool) auth.AuthManager {
	if commandIdentity == "" || !hasRunnableStep {
		return nil
	}

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
	validator := validation.NewValidator()
	authManager, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err), "", "")
	}

	ctx := context.Background()
	if _, err = authManager.GetCachedCredentials(ctx, commandIdentity); err == nil {
		log.Debug("Authenticated with cached identity for custom command", customCommandKeyIdentity, commandIdentity, customCommandKeyCommand, commandName)
		return authManager
	}

	log.Debug("No valid cached credentials found, authenticating", customCommandKeyIdentity, commandIdentity, "error", err)
	if _, err = authManager.Authenticate(ctx, commandIdentity); err == nil {
		log.Debug("Authenticated with identity for custom command", customCommandKeyIdentity, commandIdentity, customCommandKeyCommand, commandName)
		return authManager
	}
	if errors.Is(err, errUtils.ErrUserAborted) {
		errUtils.CheckErrorPrintAndExit(errUtils.ErrUserAborted, "", "")
	}
	errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w for identity %q in custom command %q: %w",
		errUtils.ErrAuthenticationFailed, commandIdentity, commandName, err), "", "")
	return authManager
}

// cloneCommand clones a custom command config into a new struct.
func cloneCommand(orig *schema.Command) (*schema.Command, error) {
	origJSON, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}

	clone := schema.Command{}
	if err = json.Unmarshal(origJSON, &clone); err != nil {
		return nil, err
	}

	return &clone, nil
}

// findTypedValue finds the value of an argument or flag with the specified semantic type.
// For arguments, it checks the Type field.
// For flags, it checks the SemanticType field.
// Returns empty string if no matching typed argument/flag is found.
func findTypedValue(cmd *schema.Command, argumentsData map[string]string, flagsData map[string]any, semanticType string) string {
	// Check arguments first.
	for _, arg := range cmd.Arguments {
		if arg.Type == semanticType {
			if val, ok := argumentsData[arg.Name]; ok {
				return val
			}
		}
	}

	// Check flags.
	for _, flag := range cmd.Flags {
		if flag.SemanticType == semanticType {
			if val, ok := flagsData[flag.Name]; ok {
				if strVal, ok := val.(string); ok {
					return strVal
				}
			}
		}
	}

	return ""
}

// validateAtmosConfig checks the Atmos configuration and returns an error instead of exiting.
// This makes the function testable by allowing errors to be handled by the caller.
func validateAtmosConfig(opts ...AtmosValidateOption) error {
	vCfg := &ValidateConfig{
		CheckStack: true, // Default value true to check the stack
	}

	// Apply options
	for _, opt := range opts {
		opt(vCfg)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	if vCfg.CheckStack {
		atmosConfigExists, err := u.IsDirectory(atmosConfig.StacksBaseAbsolutePath)
		if !atmosConfigExists || err != nil {
			// Return an error with context instead of printing and exiting
			return errUtils.Build(errUtils.ErrStacksDirectoryDoesNotExist).
				WithExplanationf("Stacks directory not found:  \n%s", atmosConfig.StacksBaseAbsolutePath).
				WithContext("base_path", atmosConfig.BasePath).
				WithContext("stacks_base_path", atmosConfig.Stacks.BasePath).
				Err()
		}
	}

	return nil
}

// checkAtmosConfig checks Atmos config and exits on error.
// This is the legacy wrapper that preserves existing behavior for backward compatibility.
func checkAtmosConfig(opts ...AtmosValidateOption) {
	err := validateAtmosConfig(opts...)
	if err != nil {
		// Try to load config for error display (may fail, that's OK)
		atmosConfig, _ := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		printMessageForMissingAtmosConfig(atmosConfig)
		errUtils.Exit(1)
	}
}

// checkAtmosConfigE checks Atmos config and returns an error instead of exiting.
// This is the testable version that should be used in commands' RunE functions.
// The returned error has exit code 1 attached for proper process termination.
func checkAtmosConfigE(opts ...AtmosValidateOption) error {
	err := validateAtmosConfig(opts...)
	if err == nil {
		return nil
	}

	// Try to load config for error display (may fail, that's OK).
	atmosConfig, _ := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)

	// Build the error using the error builder pattern.
	builder := errUtils.Build(errUtils.ErrMissingAtmosConfig).
		WithExitCode(1)

	// Build explanation with problem details.
	var explanation strings.Builder
	stacksDir := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)

	// Add git repo warning to explanation if applicable.
	if gitErr := verifyInsideGitRepoE(); gitErr != nil {
		explanation.WriteString(gitErr.Error())
		explanation.WriteString("\n\n")
	}

	if _, statErr := os.Stat(atmosConfig.CliConfigPath); os.IsNotExist(statErr) {
		explanation.WriteString("The `atmos.yaml` CLI config file was not found.\n\n")
	}

	if _, statErr := os.Stat(stacksDir); os.IsNotExist(statErr) {
		fmt.Fprintf(&explanation, "The default Atmos stacks directory `%s` does not exist in the current path.\n\n", stacksDir)
	} else if atmosConfig.CliConfigPath != "" {
		if _, statErr := os.Stat(atmosConfig.CliConfigPath); !os.IsNotExist(statErr) {
			fmt.Fprintf(&explanation, "The `atmos.yaml` config file specifies stacks directory as `%s`, but it does not exist.\n\n", stacksDir)
		}
	}

	if explanation.Len() > 0 {
		builder = builder.WithExplanation(strings.TrimSpace(explanation.String()))
	}

	// Add actionable hints - what the user should DO.
	builder = builder.
		WithHint("Initialize your git repository if you haven't already: git init").
		WithHint("Create atmos.yaml configuration file in your repository root").
		WithHint("Set up your stacks directory structure").
		WithHint("See documentation: https://atmos.tools/cli/configuration")

	return builder.Err()
}

// printMessageForMissingAtmosConfig prints Atmos logo and instructions on how to configure and start using Atmos.
func printMessageForMissingAtmosConfig(atmosConfig schema.AtmosConfiguration) {
	u.PrintMessage("")
	err := tuiUtils.PrintStyledText("ATMOS")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	// Check if we're in a git repo. Warn if not.
	verifyInsideGitRepo()

	stacksDir := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)

	u.PrintfMarkdownToTUI("\n")

	if atmosConfig.Default {
		// If Atmos did not find an `atmos.yaml` config file and is using the default config.
		u.PrintfMarkdownToTUI(missingConfigDefaultMarkdown, stacksDir)
	} else {
		// If Atmos found an `atmos.yaml` config file, but it defines invalid paths to Atmos stacks and components.
		u.PrintfMarkdownToTUI(missingConfigFoundMarkdown, stacksDir)
	}

	u.PrintfMarkdownToTUI("\n")

	// Use markdown formatting for consistent output to stderr.
	u.PrintfMarkdownToTUI("%s", gettingStartedMarkdown)
}

// CheckForAtmosUpdateAndPrintMessage checks if a version update is needed and prints a message if a newer version is found.
// It loads the cache, decides if it's time to check for updates, compares the current version to the latest available release,
// and if newer, prints the update message. It also updates the cache's timestamp after printing.
func CheckForAtmosUpdateAndPrintMessage(atmosConfig schema.AtmosConfiguration) {
	// If version checking is disabled in the configuration, do nothing
	if !atmosConfig.Version.Check.Enabled {
		return
	}

	// Load the cache
	cacheCfg, err := cfg.LoadCache()
	if err != nil {
		log.Warn("Could not load cache", "error", err)
		return
	}

	// Determine if it's time to check for updates based on frequency and last_checked
	if !cfg.ShouldCheckForUpdates(cacheCfg.LastChecked, atmosConfig.Version.Check.Frequency) {
		// Not due for another check yet, so return without printing anything
		return
	}

	// Get the latest Atmos release from GitHub
	latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
	if err != nil {
		log.Warn("Failed to retrieve latest Atmos release info", "error", err)
		return
	}

	if latestReleaseTag == "" {
		log.Warn("No release information available")
		return
	}

	// Trim "v" prefix to compare versions
	latestVersion := strings.TrimPrefix(latestReleaseTag, "v")
	currentVersion := strings.TrimPrefix(version.Version, "v")

	// If the versions differ, print the update message
	if latestVersion != currentVersion {
		u.PrintMessageToUpgradeToAtmosLatestRelease(latestVersion)
	}

	// Update the cache to mark the current timestamp
	cacheCfg.LastChecked = time.Now().Unix()
	if saveErr := cfg.SaveCache(cacheCfg); saveErr != nil {
		log.Warn("Unable to save cache", "error", saveErr)
	}
}

// Check Atmos is version command.
func isVersionCommand() bool {
	return len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version")
}

// isHelpRequestedInArgs reports whether this invocation is a help request.
// Help screens must always render — even when atmos.yaml is missing or
// invalid — so config-load failures are tolerated when this returns true.
// Uses the same argument matching as the root help function.
func isHelpRequestedInArgs() bool {
	return Contains(os.Args, "help") || Contains(os.Args, "--help") || Contains(os.Args, "-h")
}

// isVersionManagementCommand checks if the current command is a version management command.
// These commands should not trigger re-exec to avoid infinite loops.
func isVersionManagementCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	// Check the command hierarchy.
	cmdName := cmd.Name()

	// Direct version subcommands that manage local installations (install, uninstall).
	// Note: "list" is excluded because it can reasonably work with --use-version
	// to list releases using a different Atmos version.
	if cmd.Parent() != nil && cmd.Parent().Name() == "version" {
		return cmdName == "install" || cmdName == "uninstall"
	}

	// The version command itself (shows current version).
	if cmdName == "version" && cmd.Parent() != nil && cmd.Parent().Name() == "atmos" {
		return true
	}

	return false
}

// handleHelpRequest shows help content and exits only if the first argument is "help" or "--help" or "-h".
func handleHelpRequest(cmd *cobra.Command, args []string) {
	if (len(args) > 0 && args[0] == "help") || Contains(args, "--help") || Contains(args, "-h") {
		cmd.Help()
		errUtils.Exit(0)
	}
}

// showUsageAndExit we display the Markdown usage or fallback to our custom usage.
// Markdown usage is not compatible with all outputs. We should therefore have fallback option.
func showUsageAndExit(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		showErrorExampleFromMarkdown(cmd, "")
	}
	if len(args) > 0 {
		showErrorExampleFromMarkdown(cmd, args[0])
	}
	errUtils.Exit(1)
}

func showFlagUsageAndExit(cmd *cobra.Command, err error) error {
	unknownCommand := fmt.Sprintf("%v for command `%s`\n\n", err.Error(), cmd.CommandPath())
	args := strings.Split(err.Error(), ": ")
	if len(args) == 2 {
		if strings.Contains(args[0], "flag needs an argument") {
			unknownCommand = fmt.Sprintf("`%s` %s for command `%s`\n\n", args[1], args[0], cmd.CommandPath())
		} else {
			unknownCommand = fmt.Sprintf("%s `%s` for command `%s`\n\n", args[0], args[1], cmd.CommandPath())
		}
	}
	showUsageExample(cmd, unknownCommand)
	return errUtils.WithExitCode(err, 1)
}

// GetConfigAndStacksInfo processes the CLI config and stacks.
// Exported for use by command packages (e.g., terraform package).
func GetConfigAndStacksInfo(commandName string, cmd *cobra.Command, args []string) (schema.ConfigAndStacksInfo, error) {
	return getConfigAndStacksInfo(commandName, cmd, args)
}

// getConfigAndStacksInfo processes the CLI config and stacks.
// Returns error instead of calling os.Exit for better testability.
func getConfigAndStacksInfo(commandName string, cmd *cobra.Command, args []string) (schema.ConfigAndStacksInfo, error) {
	// Check Atmos configuration.
	if err := validateAtmosConfig(); err != nil {
		return schema.ConfigAndStacksInfo{}, err
	}

	var argsAfterDoubleDash []string
	finalArgs := args

	doubleDashIndex := lo.IndexOf(args, "--")
	if doubleDashIndex > 0 {
		finalArgs = lo.Slice(args, 0, doubleDashIndex)
		argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
	}

	info, err := e.ProcessCommandLineArgs(commandName, cmd, finalArgs, argsAfterDoubleDash)
	if err != nil {
		return schema.ConfigAndStacksInfo{}, err
	}

	// Resolve path-based component arguments to component names.
	if info.NeedsPathResolution && info.ComponentFromArg != "" {
		if err := resolveComponentPath(&info, commandName); err != nil {
			return schema.ConfigAndStacksInfo{}, err
		}
	}

	return info, nil
}

// resolveComponentPath resolves a path-based component argument to a component name.
// It validates the component exists in the specified stack and handles ambiguous paths.
func resolveComponentPath(info *schema.ConfigAndStacksInfo, commandName string) error {
	// Initialize config with processStacks=true to enable stack-based validation.
	// This is needed to detect ambiguous paths (multiple components referencing the same folder).
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPathResolutionFailed, err)
	}

	// Resolve component from path WITH stack validation.
	// This will:
	// 1. Extract the component name from the path (e.g., "vpc" from "components/terraform/vpc")
	// 2. Look up which Atmos components reference this terraform folder in the stack
	// 3. If multiple components reference the same folder, return an ambiguous path error.
	resolvedComponent, err := e.ResolveComponentFromPath(
		&atmosConfig,
		info.ComponentFromArg,
		info.Stack,
		commandName, // Component type is the command name (terraform, helmfile, packer).
	)
	if err != nil {
		return handlePathResolutionError(err)
	}

	log.Debug(
		"Resolved component from path",
		"original_path", info.ComponentFromArg,
		"resolved_component", resolvedComponent,
		"stack", info.Stack,
	)

	info.ComponentFromArg = resolvedComponent
	info.NeedsPathResolution = false // Mark as resolved.
	return nil
}

// handlePathResolutionError wraps path resolution errors with appropriate hints.
func handlePathResolutionError(err error) error {
	// These errors already have detailed hints from the resolver, return directly.
	// Using fmt.Errorf to wrap would lose the cockroachdb/errors hints.
	if errors.Is(err, errUtils.ErrAmbiguousComponentPath) ||
		errors.Is(err, errUtils.ErrComponentNotInStack) ||
		errors.Is(err, errUtils.ErrStackNotFound) ||
		errors.Is(err, errUtils.ErrUserAborted) {
		return err
	}
	// Generic path resolution error - add hint.
	// Use WithCause to preserve the underlying error for errors.Is introspection.
	return errUtils.Build(errUtils.ErrPathResolutionFailed).
		WithCause(err).
		WithHint("Make sure the path is within your component directories").
		Err()
}

// enableHeatmapIfRequested checks os.Args for --heatmap and --heatmap-mode flags.
// This is needed for commands with DisableFlagParsing=true (terraform, helmfile, packer)
// where Cobra doesn't parse the flags, so PersistentPreRun can't detect them.
// We only enable tracking if --heatmap is present; --heatmap-mode is only relevant when --heatmap is set.
func enableHeatmapIfRequested() {
	for _, arg := range os.Args {
		if arg == "--heatmap" {
			perf.EnableTracking(true)
			return
		}
	}
}

// isGitRepository checks if the current directory is within a git repository.
func isGitRepository() bool {
	_, err := git.PlainOpenWithOptions(currentDirPath, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		if !errors.Is(err, git.ErrRepositoryNotExists) {
			log.Debug("Git check failed", "error", err)
		}
		return false
	}

	return true
}

// verifyInsideGitRepo checks if we're in a git repo.
func verifyInsideGitRepo() bool {
	// Check if we're in a git repo
	if !isGitRepository() {
		log.Warn("You're not inside a git repository. Atmos feels lonely outside - bring it home!")
		return false
	}
	return true
}

// verifyInsideGitRepoE returns an error if not inside a git repository.
// This is the testable version that returns an error instead of just logging.
func verifyInsideGitRepoE() error {
	if !isGitRepository() {
		return fmt.Errorf("%w: Atmos feels lonely outside, bring it home", errUtils.ErrNotInGitRepository)
	}
	return nil
}

func showErrorExampleFromMarkdown(cmd *cobra.Command, arg string) {
	commandPath := cmd.CommandPath()
	suggestions := []string{}
	details := fmt.Sprintf("You invoked `%s` incorrectly.\n", commandPath)
	if len(arg) > 0 {
		details = fmt.Sprintf("Unknown command `%s` for `%s`\n", arg, commandPath)
	} else if len(cmd.Commands()) != 0 && arg == "" {
		details = fmt.Sprintf("`%s` requires a subcommand.\n", commandPath)
	}
	if len(arg) > 0 {
		suggestions = cmd.SuggestionsFor(arg)
	}
	if len(suggestions) > 0 {
		details = details + "Did you mean this?\n"
		for _, suggestion := range suggestions {
			details += "* " + suggestion + "\n"
		}
	} else {
		if len(cmd.Commands()) > 0 {
			details += "\nValid subcommands are:\n"
		}
		// Retrieve valid subcommands dynamically
		for _, subCmd := range cmd.Commands() {
			details = details + "* " + subCmd.Name() + "\n"
		}
	}
	showUsageExample(cmd, details)
}

func showUsageExample(cmd *cobra.Command, details string) {
	contentName := strings.ReplaceAll(strings.ReplaceAll(cmd.CommandPath(), " ", "_"), "-", "_")
	suggestion := fmt.Sprintf("\n\nRun `%s --help` for usage", cmd.CommandPath())
	details = appendUsageSection(details, cmd)
	if exampleContent, ok := examples[contentName]; ok {
		suggestion = exampleContent.Suggestion
		details += "\n## Usage Examples:\n" + exampleContent.Content
	}
	errUtils.CheckErrorPrintAndExit(errors.New(details), "Incorrect Usage", suggestion)
}

// appendUsageSection appends a generated "Usage" Markdown section (a fenced
// shell block containing the command's usage lines) to the details string.
// When the command yields no usage lines, details is returned unchanged.
func appendUsageSection(details string, cmd *cobra.Command) string {
	usage := commandUsageLines(cmd)
	if usage == "" {
		return details
	}

	return details + "\n## Usage\n\n```shell\n" + usage + "\n```\n"
}

// commandUsageLines generates the usage line(s) for a cobra command: the
// UseLine when the command is runnable, plus a "<path> [sub-command] [flags]"
// line when it has available subcommands. Returns an empty string for a nil
// command.
func commandUsageLines(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}

	lines := []string{}
	if cmd.Runnable() {
		lines = append(lines, cmd.UseLine())
	}
	if cmd.HasAvailableSubCommands() {
		lines = append(lines, fmt.Sprintf("%s [sub-command] [flags]", cmd.CommandPath()))
	}

	return strings.Join(lines, "\n")
}

// StackFlagCompletion provides shell completion for the --stack flag.
// If a component was provided as the first positional argument, it filters stacks
// to only those containing that component.
func StackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		component := args[0]

		// Check if argument is a path that needs resolution
		// Paths are: ".", or contain path separators
		if component == "." || strings.Contains(component, string(filepath.Separator)) {
			// Attempt to resolve path to component name for stack filtering
			// Use silent error handling - if resolution fails, just list all stacks (graceful degradation)
			configAndStacksInfo := schema.ConfigAndStacksInfo{}
			atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
			if err == nil {
				// Determine component type from command
				// Walk up command chain to find root command (terraform, helmfile, packer)
				componentType := determineComponentTypeFromCommand(cmd)

				// Try to resolve the path (without stack context for completion)
				resolvedComponent, err := e.ResolveComponentFromPath(
					&atmosConfig,
					component,
					"", // No stack context yet - we're completing the stack flag
					componentType,
				)
				if err != nil {
					// If resolution fails, fall through to list all stacks (graceful degradation)
					log.Trace(
						"Could not resolve path for stack completion, listing all stacks",
						"path", component,
						"error", err,
					)
					output, err := listStacks(cmd)
					if err != nil {
						return nil, cobra.ShellCompDirectiveNoFileComp
					}
					return output, cobra.ShellCompDirectiveNoFileComp
				}
				component = resolvedComponent
				log.Trace(
					"Resolved path for stack completion",
					"original", args[0],
					"resolved", component,
				)
			}
		}

		output, err := listStacksForComponent(component)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks.
	output, err := listStacks(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// determineComponentTypeFromCommand walks up the command chain to find the component type.
func determineComponentTypeFromCommand(cmd *cobra.Command) string {
	// Walk up to find the root component command (terraform, helmfile, packer).
	current := cmd
	for current != nil {
		switch current.Name() {
		case "terraform":
			return "terraform"
		case "helmfile":
			return "helmfile"
		case "packer":
			return "packer"
		}
		current = current.Parent()
	}
	// Default to terraform if we can't determine
	return "terraform"
}

// listStacksForComponent returns stacks that contain the specified component.
// It initializes the CLI configuration, describes all stacks, and filters them
// to include only those defining the given component.
func listStacksForComponent(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, component)
	return output, err
}

// listStacks is a wrapper that calls the list package's listAllStacks function.
func listStacks(cmd *cobra.Command) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %v", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, "")
	return output, err
}

// listComponents is a wrapper that lists all components.
func listComponents(cmd *cobra.Command) ([]string, error) {
	flags := cmd.Flags()
	stackFlag, err := flags.GetString("stack")
	if err != nil {
		stackFlag = ""
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %v", err)
	}

	output, err := l.FilterAndListComponents(stackFlag, stacksMap)
	return output, err
}

func AddStackCompletion(cmd *cobra.Command) {
	if cmd.Flag(FlagStack) == nil {
		cmd.PersistentFlags().StringP(FlagStack, "s", "", stackHint)
	}
	_ = cmd.RegisterFlagCompletionFunc(FlagStack, StackFlagCompletion)
}

// identityFlagCompletion provides shell completion for identity flags by fetching
// available identities from the Atmos configuration.
func identityFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

// AddIdentityCompletion registers shell completion for the identity flag if present on the command.
func AddIdentityCompletion(cmd *cobra.Command) {
	if cmd.Flag(customCommandKeyIdentity) != nil {
		if err := cmd.RegisterFlagCompletionFunc(customCommandKeyIdentity, identityFlagCompletion); err != nil {
			log.Trace("Failed to register identity flag completion", "error", err)
		}
	}
}

// identityArgCompletion provides shell completion for identity positional arguments.
// It returns a list of available identities from the Atmos configuration.
func identityArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first positional argument.
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

func ComponentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Check if user is typing a path
		// Enable directory completion for paths
		// Check for both platform-specific separator and forward slash (works on all platforms)
		if toComplete == "." || strings.Contains(toComplete, string(filepath.Separator)) || strings.Contains(toComplete, "/") {
			log.Trace("Enabling directory completion for path input", "toComplete", toComplete)
			return nil, cobra.ShellCompDirectiveFilterDirs
		}

		// Otherwise, suggest component names from stack configurations
		output, err := listComponents(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) > 0 {
		flagName := args[len(args)-1]
		if strings.HasPrefix(flagName, "--") {
			flagName = strings.ReplaceAll(flagName, "--", "")
		}
		if strings.HasPrefix(toComplete, "--") {
			flagName = strings.ReplaceAll(toComplete, "--", "")
		}
		flagName = strings.ReplaceAll(flagName, "=", "")
		if option, ok := cmd.GetFlagCompletionFunc(flagName); ok {
			return option(cmd, args, toComplete)
		}
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// Contains checks if a slice of strings contains an exact match for the target string.
func Contains(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

// processCustomComponentType registers a custom component type and resolves its configuration.
//
//nolint:revive // argument-limit: parameters are necessary for component processing
func processCustomComponentType(
	_ *schema.AtmosConfiguration,
	commandConfig *schema.Command,
	argumentsData map[string]string,
	flagsData map[string]any,
	data map[string]any,
	authManager auth.AuthManager,
) {
	componentConfig, err := resolveCustomComponentConfig(
		commandConfig,
		argumentsData,
		flagsData,
		authManager,
		custom.EnsureRegistered,
		e.ExecuteDescribeComponent,
	)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	data["Component"] = componentConfig
}

// appendComponentEnvVars exports a custom component's resolved `env` section (from the
// describe-component result) into the step subprocess environment, mirroring how the built-in
// terraform/helmfile/packer/ansible providers export `ComponentEnvSection`. Without this, a custom
// component's `env` section (including resolved `!secret` values) would only be available as
// `{{ .Component.env.* }}` template data and never reach the step subprocess. Null and "null"
// values are skipped (same convention as pkg/env.ConvertEnvVars). Existing entries are overwritten,
// so callers can layer the command-level `env:` on top for higher precedence. Keys are applied in
// sorted order for deterministic output.
func appendComponentEnvVars(env []string, componentConfig map[string]any) []string {
	defer perf.Track(nil, "cmd.appendComponentEnvVars")()

	envSection, ok := componentConfig[cfg.EnvSectionName].(map[string]any)
	if !ok {
		return env
	}

	keys := make([]string, 0, len(envSection))
	for k := range envSection {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := envSection[k]
		if v == nil || v == "null" {
			continue
		}
		env = envpkg.UpdateEnvVar(env, k, fmt.Sprint(v))
	}

	return env
}

// resolveCustomComponentConfig finds the component/stack, registers the custom type,
// and resolves the component config. It returns an error instead of exiting so it is
// testable; the caller is responsible for handling the error (see processCustomComponentType).
// The ensureRegisteredFn and describeFn dependencies are injected to enable unit testing.
//
//nolint:revive // argument-limit: parameters are necessary for component processing
func resolveCustomComponentConfig(
	commandConfig *schema.Command,
	argumentsData map[string]string,
	flagsData map[string]any,
	authManager auth.AuthManager,
	ensureRegisteredFn func(typeName, basePath string) error,
	describeFn func(*e.ExecuteDescribeComponentParams) (map[string]any, error),
) (map[string]any, error) {
	defer perf.Track(nil, "cmd.resolveCustomComponentConfig")()

	// Find component name from argument/flag with type: component.
	componentName := findTypedValue(commandConfig, argumentsData, flagsData, semanticTypeComponent)
	if componentName == "" {
		return nil, errUtils.ErrComponentArgumentNotFound
	}

	// Find stack name from argument/flag with type: stack (or semantic_type: stack for flags).
	stackName := findTypedValue(commandConfig, argumentsData, flagsData, semanticTypeStack)
	if stackName == "" {
		return nil, errUtils.ErrStackArgumentNotFound
	}

	// Register the custom component type if not already registered.
	basePath := commandConfig.Component.BasePath
	if basePath == "" {
		basePath = fmt.Sprintf("components/%s", commandConfig.Component.Type)
	}
	if err := ensureRegisteredFn(commandConfig.Component.Type, basePath); err != nil {
		return nil, err
	}

	// Get the config for the component in the stack.
	return describeFn(&e.ExecuteDescribeComponentParams{
		Component:            componentName,
		Stack:                stackName,
		ComponentType:        commandConfig.Component.Type,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          authManager,
	})
}
