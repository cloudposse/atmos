package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
	topLevel bool,
) error {
	var command *cobra.Command
	existingTopLevelCommands := make(map[string]*cobra.Command)

	if topLevel {
		existingTopLevelCommands = getTopLevelCommands()
	}

	for _, commandCfg := range commands {
		// Clone the 'commandCfg' struct into a local variable because of the automatic closure in the `Run` function of the Cobra command.
		// Cloning will make a closure over the local variable 'commandConfig' which is different in each iteration.
		// https://www.calhoun.io/gotchas-and-common-mistakes-with-closures-in-go/
		commandConfig, err := cloneCommand(&commandCfg)
		if err != nil {
			return err
		}

		if _, exist := existingTopLevelCommands[commandConfig.Name]; exist && topLevel {
			command = existingTopLevelCommands[commandConfig.Name]
		} else {
			customCommand := &cobra.Command{
				Use:   commandConfig.Name,
				Short: commandConfig.Description,
				Long:  commandConfig.Description,
				PreRun: func(cmd *cobra.Command, args []string) {
					preCustomCommand(cmd, args, parentCommand, commandConfig)
				},
				Run: func(cmd *cobra.Command, args []string) {
					executeCustomCommand(atmosConfig, cmd, args, parentCommand, commandConfig)
				},
			}
			customCommand.PersistentFlags().Bool("", false, doubleDashHint)

			// Add --identity flag to all custom commands to allow runtime override
			customCommand.PersistentFlags().String("identity", "", "Identity to use for authentication (overrides identity in command config)")
			AddIdentityCompletion(customCommand)

			// Process and add flags to the command.
			for _, flag := range commandConfig.Flags {
				if flag.Type == "bool" {
					defaultVal := false
					if flag.Default != nil {
						if boolVal, ok := flag.Default.(bool); ok {
							defaultVal = boolVal
						}
					}
					if flag.Shorthand != "" {
						customCommand.PersistentFlags().BoolP(flag.Name, flag.Shorthand, defaultVal, flag.Usage)
					} else {
						customCommand.PersistentFlags().Bool(flag.Name, defaultVal, flag.Usage)
					}
				} else {
					defaultVal := ""
					if flag.Default != nil {
						if strVal, ok := flag.Default.(string); ok {
							defaultVal = strVal
						}
					}
					if flag.Shorthand != "" {
						customCommand.PersistentFlags().StringP(flag.Name, flag.Shorthand, defaultVal, flag.Usage)
					} else {
						customCommand.PersistentFlags().String(flag.Name, defaultVal, flag.Usage)
					}
				}

				if flag.Required {
					err := customCommand.MarkPersistentFlagRequired(flag.Name)
					if err != nil {
						return err
					}
				}
			}

			// Add the command to the parent command
			parentCommand.AddCommand(customCommand)
			command = customCommand
		}

		err = processCustomCommands(atmosConfig, commandConfig.Commands, command, false)
		if err != nil {
			return err
		}
	}

	return nil
}

// filterChdirEnv removes ATMOS_CHDIR from environ and optionally adds an empty
// ATMOS_CHDIR= entry to override any parent value when spawning child processes.
// This prevents child processes from re-applying the parent's chdir directive.
func filterChdirEnv(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	foundAtmosChdir := false
	for _, env := range environ {
		if strings.HasPrefix(env, "ATMOS_CHDIR=") {
			foundAtmosChdir = true
			continue
		}
		filtered = append(filtered, env)
	}
	// Add empty ATMOS_CHDIR to override parent's value in merged environment.
	if foundAtmosChdir {
		filtered = append(filtered, "ATMOS_CHDIR=")
	}
	return filtered
}

// filterChdirArgs returns a copy of args with any chdir flags and their values removed.
// It removes `--chdir`, `--chdir=<value>`, `-C`, `-C=<value>`, and concatenated `-C<value>` forms, preserving the order of all other arguments.
func filterChdirArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	skipNext := false

	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// Skip --chdir=value, -C=value, -C<value> (concatenated).
		if strings.HasPrefix(arg, "--chdir=") ||
			strings.HasPrefix(arg, "-C=") ||
			(strings.HasPrefix(arg, "-C") && len(arg) > 2) {
			continue
		}

		// Skip --chdir value or -C value (next arg is the value).
		if arg == "--chdir" || arg == "-C" {
			skipNext = true
			continue
		}

		// Keep all other args.
		filtered = append(filtered, arg)
	}

	return filtered
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

		if _, exist := existingTopLevelCommands[alias]; !exist && topLevel {
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
					filteredArgs := filterChdirArgs(args)

					// Filter out ATMOS_CHDIR from environment variables to prevent the child process
					// from re-applying the parent's chdir directive.
					filteredEnv := filterChdirEnv(os.Environ())

					// Build command arguments: split aliasCmd into parts and append filteredArgs.
					// Use direct process execution instead of shell to avoid path escaping
					// issues on Windows where backslashes in paths are misinterpreted.
					cmdArgs := strings.Fields(aliasCmd)
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
			log.Info(sb.String())
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
				requiredNoDefaultCount))

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
			log.Trace("Failed to display command help", "error", err, "command", cmd.Name())
		}
		errUtils.Exit(0)
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

	// Create auth manager if identity is specified for this custom command.
	// Check for --identity flag first (it overrides the config).
	var authManager auth.AuthManager
	var authStackInfo *schema.ConfigAndStacksInfo
	identityFlag, _ := cmd.Flags().GetString("identity")
	commandIdentity := strings.TrimSpace(identityFlag)
	if commandIdentity == "" {
		// Fall back to identity from command config
		commandIdentity = strings.TrimSpace(commandConfig.Identity)
	}

	if commandIdentity != "" {
		// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
		// This enables YAML template functions to access authenticated credentials.
		authStackInfo = &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{},
		}

		credStore := credentials.NewCredentialStore()
		validator := validation.NewValidator()
		authManager, err = auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo)
		if err != nil {
			errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err), "", "")
		}

		ctx := context.Background()

		// Try to use cached credentials first (passive check, no prompts).
		// Only authenticate if cached credentials are not available or expired.
		_, err = authManager.GetCachedCredentials(ctx, commandIdentity)
		if err != nil {
			log.Debug("No valid cached credentials found, authenticating", "identity", commandIdentity, "error", err)
			// No valid cached credentials - perform full authentication.
			_, err = authManager.Authenticate(ctx, commandIdentity)
			if err != nil {
				// Check for user cancellation - return clean error without wrapping.
				if errors.Is(err, errUtils.ErrUserAborted) {
					errUtils.CheckErrorPrintAndExit(errUtils.ErrUserAborted, "", "")
				}
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w for identity %q in custom command %q: %w",
					errUtils.ErrAuthenticationFailed, commandIdentity, commandConfig.Name, err), "", "")
			}
		}

		log.Debug("Authenticated with identity for custom command", "identity", commandIdentity, "command", commandConfig.Name)
	}

	// Determine working directory for command execution.
	workDir, err := resolveWorkingDirectory(commandConfig.WorkingDirectory, atmosConfig.BasePath, currentDirPath)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	if commandConfig.WorkingDirectory != "" {
		log.Debug("Using working directory for custom command", "command", commandConfig.Name, "working_directory", workDir)
	}

	// Execute custom command's steps
	for i, step := range commandConfig.Steps {
		// Prepare template data for arguments
		argumentsData := map[string]string{}
		for ix, arg := range commandConfig.Arguments {
			argumentsData[arg.Name] = finalArgs[ix]
		}

		// Prepare template data for flags
		flags := cmd.Flags()
		flagsData := map[string]any{}
		for _, fl := range commandConfig.Flags {
			if fl.Type == "" || fl.Type == "string" {
				providedFlag, err := flags.GetString(fl.Name)
				errUtils.CheckErrorPrintAndExit(err, "", "")
				flagsData[fl.Name] = providedFlag
			} else if fl.Type == "bool" {
				boolFlag, err := flags.GetBool(fl.Name)
				errUtils.CheckErrorPrintAndExit(err, "", "")
				flagsData[fl.Name] = boolFlag
			}
		}

		// Prepare template data
		data := map[string]any{
			"Arguments":    argumentsData,
			"Flags":        flagsData,
			"TrailingArgs": trailingArgs,
		}

		// If the custom command defines 'component_config' section with 'component' and 'stack' attributes,
		// process the component stack config and expose it in {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		if commandConfig.ComponentConfig.Component != "" && commandConfig.ComponentConfig.Stack != "" {
			// Process Go templates in the command's 'component_config.component'
			component, err := e.ProcessTmpl(&atmosConfig, fmt.Sprintf("component-config-component-%d", i), commandConfig.ComponentConfig.Component, data, false)
			errUtils.CheckErrorPrintAndExit(err, "", "")
			if component == "" || component == "<no value>" {
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("the command defines an invalid 'component_config.component: %s' in '%s'",
					commandConfig.ComponentConfig.Component, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension), "", "")
			}

			// Process Go templates in the command's 'component_config.stack'
			stack, err := e.ProcessTmpl(&atmosConfig, fmt.Sprintf("component-config-stack-%d", i), commandConfig.ComponentConfig.Stack, data, false)
			errUtils.CheckErrorPrintAndExit(err, "", "")
			if stack == "" || stack == "<no value>" {
				errUtils.CheckErrorPrintAndExit(fmt.Errorf("the command defines an invalid 'component_config.stack: %s' in '%s'",
					commandConfig.ComponentConfig.Stack, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension), "", "")
			}

			// Get the config for the component in the stack
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
		// Start with current environment to inherit PATH and other variables.
		env := os.Environ()
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
				value, err = e.ProcessTmpl(&atmosConfig, fmt.Sprintf("env-var-%d", i), value, data, false)
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			// Add or update the environment variable in the env slice
			env = u.UpdateEnvVar(env, key, value)
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
			log.Debug("Prepared environment with identity for custom command step", "identity", commandIdentity, "command", commandConfig.Name, "step", i)
		}

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		commandToRun, err := e.ProcessTmpl(&atmosConfig, fmt.Sprintf("step-%d", i), step, data, false)
		errUtils.CheckErrorPrintAndExit(err, "", "")

		// Execute the command step
		commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)

		// Pass the prepared environment with custom variables to the subprocess
		err = e.ExecuteShell(commandToRun, commandName, workDir, env, false)
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
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
				WithHintf("Stacks directory not found:  \n%s", atmosConfig.StacksBaseAbsolutePath).
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
	fmt.Println()
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

	log.Debug("Resolved component from path",
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
	details := fmt.Sprintf("The command `%s` is not valid usage\n", commandPath)
	if len(arg) > 0 {
		details = fmt.Sprintf("Unknown command `%s` for `%s`\n", arg, commandPath)
	} else if len(cmd.Commands()) != 0 && arg == "" {
		details = fmt.Sprintf("The command `%s` requires a subcommand\n", commandPath)
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
	if exampleContent, ok := examples[contentName]; ok {
		suggestion = exampleContent.Suggestion
		details += "\n## Usage Examples:\n" + exampleContent.Content
	}
	errUtils.CheckErrorPrintAndExit(errors.New(details), "Incorrect Usage", suggestion)
}

func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
					log.Trace("Could not resolve path for stack completion, listing all stacks",
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
				log.Trace("Resolved path for stack completion",
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
	if cmd.Flag("stack") == nil {
		cmd.PersistentFlags().StringP("stack", "s", "", stackHint)
	}
	cmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)
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
	if cmd.Flag("identity") != nil {
		if err := cmd.RegisterFlagCompletionFunc("identity", identityFlagCompletion); err != nil {
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
