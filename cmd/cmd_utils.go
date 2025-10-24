package cmd

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
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

// processCustomCommands processes and executes custom commands.
func processCustomCommands(
	atmosConfig schema.AtmosConfiguration,
	commands []schema.Command,
	parentCommand *cobra.Command,
	topLevel bool,
) error {
	var command *cobra.Command
	existingTopLevelCommands := make(map[string]*cobra.Command)

	// Build commands and their hierarchy from the alias map
	for alias, fullCmd := range atmosConfig.CommandAliases {
		parts := strings.Fields(fullCmd)
		addCommandWithAlias(RootCmd, alias, parts)
	}

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
			// Process and add flags to the command
			for _, flag := range commandConfig.Flags {
				if flag.Type == "bool" {
					defaultVal := false
					if flag.Shorthand != "" {
						customCommand.PersistentFlags().BoolP(flag.Name, flag.Shorthand, defaultVal, flag.Usage)
					} else {
						customCommand.PersistentFlags().Bool(flag.Name, defaultVal, flag.Usage)
					}
				} else {
					defaultVal := ""
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

// addCommandWithAlias adds a command hierarchy based on the full command.
func addCommandWithAlias(parentCmd *cobra.Command, alias string, parts []string) {
	if len(parts) == 0 {
		return
	}

	// Check if a command with the current part already exists
	var cmd *cobra.Command
	for _, c := range parentCmd.Commands() {
		if c.Use == parts[0] {
			cmd = c
			break
		}
	}

	// If the command doesn't exist, create it
	if cmd == nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("subcommand `%s` not found for alias `%s`", parts[0], alias), "", "")
	}

	// If there are more parts, recurse for the next level
	if len(parts) > 1 {
		addCommandWithAlias(cmd, alias, parts[1:])
	} else if !Contains(cmd.Aliases, alias) {
		// This is the last part of the command, add the alias
		cmd.Aliases = append(cmd.Aliases, alias)
	}
}

// processCommandAliases processes the command aliases.
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
				Run: func(cmd *cobra.Command, args []string) {
					err := cmd.ParseFlags(args)
					errUtils.CheckErrorPrintAndExit(err, "", "")

					commandToRun := fmt.Sprintf("%s %s %s", os.Args[0], aliasCmd, strings.Join(args, " "))
					err = e.ExecuteShell(commandToRun, commandToRun, currentDirPath, nil, false)
					errUtils.CheckErrorPrintAndExit(err, "", "")
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
	args, trailingArgs := extractTrailingArgs(args, os.Args)
	if commandConfig.Verbose {
		atmosConfig.Logs.Level = u.LogLevelTrace
	}

	mergedArgsStr := cmd.Annotations["resolvedArgs"]
	finalArgs := strings.Split(mergedArgsStr, ",")
	if mergedArgsStr == "" {
		// If for some reason no annotation was set, just fallback
		finalArgs = args
	}

	// Resolve and install command dependencies
	resolver := dependencies.NewResolver(&atmosConfig)
	deps, err := resolver.ResolveCommandDependencies(commandConfig)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", fmt.Sprintf("Failed to resolve dependencies for command '%s'", commandConfig.Name))
	}

	if len(deps) > 0 {
		log.Debug("Installing command dependencies", "command", commandConfig.Name, "tools", deps)
		installer := dependencies.NewInstaller(&atmosConfig)
		if err := installer.EnsureTools(deps); err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", fmt.Sprintf("Failed to install dependencies for command '%s'", commandConfig.Name))
		}

		// Update PATH to include installed tools
		if err := dependencies.UpdatePathForTools(&atmosConfig, deps); err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", fmt.Sprintf("Failed to update PATH for command '%s'", commandConfig.Name))
		}
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
			componentConfig, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
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
				res, err := u.ExecuteShellAndReturnOutput(valCommand, valCommandName, currentDirPath, env, false)
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

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		commandToRun, err := e.ProcessTmpl(&atmosConfig, fmt.Sprintf("step-%d", i), step, data, false)
		errUtils.CheckErrorPrintAndExit(err, "", "")

		// Execute the command step
		commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)

		// Pass the prepared environment with custom variables to the subprocess
		err = e.ExecuteShell(commandToRun, commandName, currentDirPath, env, false)
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}

// Extracts native arguments (everything after "--") signifying the end of Atmos-specific arguments.
// Because of the flag hint for double dash, args is already consumed by Cobra.
// So we need to perform manual parsing of os.Args to extract the "trailing args" after the "--" end of args marker.
func extractTrailingArgs(args []string, osArgs []string) ([]string, string) {
	doubleDashIndex := lo.IndexOf(osArgs, "--")
	mainArgs := args
	trailingArgs := ""
	if doubleDashIndex > 0 {
		mainArgs = lo.Slice(osArgs, 0, doubleDashIndex)
		trailingArgs = strings.Join(lo.Slice(osArgs, doubleDashIndex+1, len(osArgs)), " ")
		result := []string{}
		lookup := make(map[string]bool)

		// Populate a lookup map for quick existence check
		for _, val := range mainArgs {
			lookup[val] = true
		}

		// Iterate over leftArr and collect matching elements in order
		for _, val := range args {
			if lookup[val] {
				result = append(result, val)
			}
		}
		mainArgs = result
	}
	return mainArgs, trailingArgs
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

// checkAtmosConfig checks Atmos config.
func checkAtmosConfig(opts ...AtmosValidateOption) {
	vCfg := &ValidateConfig{
		CheckStack: true, // Default value true to check the stack
	}

	// Apply options
	for _, opt := range opts {
		opt(vCfg)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	if vCfg.CheckStack {
		atmosConfigExists, err := u.IsDirectory(atmosConfig.StacksBaseAbsolutePath)
		if !atmosConfigExists || err != nil {
			printMessageForMissingAtmosConfig(atmosConfig)
			errUtils.Exit(1)
		}
	}
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
	errUtils.Exit(1)
	return nil
}

// getConfigAndStacksInfo processes the CLI config and stacks.
func getConfigAndStacksInfo(commandName string, cmd *cobra.Command, args []string) schema.ConfigAndStacksInfo {
	// Check Atmos configuration
	checkAtmosConfig()

	var argsAfterDoubleDash []string
	finalArgs := args

	doubleDashIndex := lo.IndexOf(args, "--")
	if doubleDashIndex > 0 {
		finalArgs = lo.Slice(args, 0, doubleDashIndex)
		argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
	}

	info, err := e.ProcessCommandLineArgs(commandName, cmd, finalArgs, argsAfterDoubleDash)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	return info
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
	output, err := listStacks(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
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
