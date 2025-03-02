package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

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

// processCustomCommands processes and executes custom commands
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
			// TODO: we need to update this post https://github.com/cloudposse/atmos/pull/959 gets merged
			customCommand.PersistentFlags().Bool("", false, doubleDashHint)
			// Process and add flags to the command
			for _, flag := range commandConfig.Flags {
				if flag.Type == "bool" {
					if flag.Shorthand != "" {
						customCommand.PersistentFlags().BoolP(flag.Name, flag.Shorthand, false, flag.Usage)
					} else {
						customCommand.PersistentFlags().Bool(flag.Name, false, flag.Usage)
					}
				} else {
					if flag.Shorthand != "" {
						customCommand.PersistentFlags().StringP(flag.Name, flag.Shorthand, "", flag.Usage)
					} else {
						customCommand.PersistentFlags().String(flag.Name, "", flag.Usage)
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

// addCommandWithAlias adds a command hierarchy based on the full command
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
		u.LogErrorAndExit(fmt.Errorf("subcommand `%s` not found for alias `%s`", parts[0], alias))
	}

	// If there are more parts, recurse for the next level
	if len(parts) > 1 {
		addCommandWithAlias(cmd, alias, parts[1:])
	} else if !Contains(cmd.Aliases, alias) {
		// This is the last part of the command, add the alias
		cmd.Aliases = append(cmd.Aliases, alias)
	}
}

// processCommandAliases processes the command aliases
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
					if err != nil {
						u.LogErrorAndExit(err)
					}

					commandToRun := fmt.Sprintf("%s %s %s", os.Args[0], aliasCmd, strings.Join(args, " "))
					err = e.ExecuteShell(atmosConfig, commandToRun, commandToRun, ".", nil, false)
					if err != nil {
						u.LogErrorAndExit(err)
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

// preCustomCommand is run before a custom command is executed
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
			u.LogInfo(sb.String())
			os.Exit(1)
		} else {
			// truly invalid, nothing to do
			u.PrintErrorMarkdownAndExit("Invalid command", errors.New(
				fmt.Sprintf("The `%s` command has no steps or subcommands configured.", cmd.CommandPath()),
			), "https://atmos.tools/cli/configuration/commands")
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
		u.LogErrorAndExit(errors.New(sb.String()))
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
				u.LogErrorAndExit(errors.New(sb.String()))
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
		_ = cmd.Help()
		os.Exit(0)
	}
}

// getTopLevelCommands returns the top-level commands
func getTopLevelCommands() map[string]*cobra.Command {
	existingTopLevelCommands := make(map[string]*cobra.Command)

	for _, c := range RootCmd.Commands() {
		existingTopLevelCommands[c.Name()] = c
	}

	return existingTopLevelCommands
}

// executeCustomCommand executes a custom command
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
				if err != nil {
					u.LogErrorAndExit(err)
				}
				flagsData[fl.Name] = providedFlag
			} else if fl.Type == "bool" {
				boolFlag, err := flags.GetBool(fl.Name)
				if err != nil {
					u.LogErrorAndExit(err)
				}
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
			component, err := e.ProcessTmpl(fmt.Sprintf("component-config-component-%d", i), commandConfig.ComponentConfig.Component, data, false)
			if err != nil {
				u.LogErrorAndExit(err)
			}
			if component == "" || component == "<no value>" {
				u.LogErrorAndExit(fmt.Errorf("the command defines an invalid 'component_config.component: %s' in '%s'",
					commandConfig.ComponentConfig.Component, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension))
			}

			// Process Go templates in the command's 'component_config.stack'
			stack, err := e.ProcessTmpl(fmt.Sprintf("component-config-stack-%d", i), commandConfig.ComponentConfig.Stack, data, false)
			if err != nil {
				u.LogErrorAndExit(err)
			}
			if stack == "" || stack == "<no value>" {
				u.LogErrorAndExit(fmt.Errorf("the command defines an invalid 'component_config.stack: %s' in '%s'",
					commandConfig.ComponentConfig.Stack, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension))
			}

			// Get the config for the component in the stack
			componentConfig, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
			if err != nil {
				u.LogErrorAndExit(err)
			}
			data["ComponentConfig"] = componentConfig
		}

		// Prepare ENV vars
		// ENV var values support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		var envVarsList []string
		for _, v := range commandConfig.Env {
			key := strings.TrimSpace(v.Key)
			value := v.Value
			valCommand := v.ValueCommand

			if value != "" && valCommand != "" {
				err = fmt.Errorf("either 'value' or 'valueCommand' can be specified for the ENV var, but not both.\n"+
					"Custom command '%s %s' defines 'value=%s' and 'valueCommand=%s' for the ENV var '%s'",
					parentCommand.Name(), commandConfig.Name, value, valCommand, key)
				u.LogErrorAndExit(err)
			}

			// If the command to get the value for the ENV var is provided, execute it
			if valCommand != "" {
				valCommandName := fmt.Sprintf("env-var-%s-valcommand", key)
				res, err := e.ExecuteShellAndReturnOutput(atmosConfig, valCommand, valCommandName, ".", nil, false)
				if err != nil {
					u.LogErrorAndExit(err)
				}
				value = strings.TrimRight(res, "\r\n")
			} else {
				// Process Go templates in the values of the command's ENV vars
				value, err = e.ProcessTmpl(fmt.Sprintf("env-var-%d", i), value, data, false)
				if err != nil {
					u.LogErrorAndExit(err)
				}
			}

			envVarsList = append(envVarsList, fmt.Sprintf("%s=%s", key, value))
			err = os.Setenv(key, value)
			if err != nil {
				u.LogErrorAndExit(err)
			}
		}

		if len(envVarsList) > 0 && commandConfig.Verbose {
			u.LogDebug("\nUsing ENV vars:")
			for _, v := range envVarsList {
				u.LogDebug(v)
			}
		}

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		commandToRun, err := e.ProcessTmpl(fmt.Sprintf("step-%d", i), step, data, false)
		if err != nil {
			u.LogErrorAndExit(err)
		}

		// Execute the command step
		commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)
		err = e.ExecuteShell(atmosConfig, commandToRun, commandName, ".", envVarsList, false)
		if err != nil {
			u.LogErrorAndExit(err)
		}
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

// cloneCommand clones a custom command config into a new struct
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

// checkAtmosConfig checks Atmos config
func checkAtmosConfig(opts ...AtmosValidateOption) {
	vCfg := &ValidateConfig{
		CheckStack: true, // Default value true to check the stack
	}

	// Apply options
	for _, opt := range opts {
		opt(vCfg)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	if vCfg.CheckStack {
		atmosConfigExists, err := u.IsDirectory(atmosConfig.StacksBaseAbsolutePath)
		if !atmosConfigExists || err != nil {
			printMessageForMissingAtmosConfig(atmosConfig)
			os.Exit(1)
		}
	}
}

// printMessageForMissingAtmosConfig prints Atmos logo and instructions on how to configure and start using Atmos
func printMessageForMissingAtmosConfig(atmosConfig schema.AtmosConfiguration) {
	c1 := theme.Colors.Info
	c2 := theme.Colors.Success

	fmt.Println()
	err := tuiUtils.PrintStyledText("ATMOS")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	if atmosConfig.Default {
		// If Atmos did not find an `atmos.yaml` config file and is using the default config
		u.PrintMessageInColor("atmos.yaml", c1)
		fmt.Println(" CLI config file was not found.")
		fmt.Print("\nThe default Atmos stacks directory is set to ")

		u.PrintMessageInColor(filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath), c1)
		fmt.Println(",\nbut the directory does not exist in the current path.")
	} else {
		// If Atmos found an `atmos.yaml` config file, but it defines invalid paths to Atmos stacks and components
		u.PrintMessageInColor("atmos.yaml", c1)
		fmt.Print(" CLI config file specifies the directory for Atmos stacks as ")
		u.PrintMessageInColor(filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath), c1)
		fmt.Println(",\nbut the directory does not exist.")
	}

	u.PrintMessage("\nTo configure and start using Atmos, refer to the following documents:\n")

	u.PrintMessageInColor("Atmos CLI Configuration:\n", c2)
	u.PrintMessage("https://atmos.tools/cli/configuration\n")

	u.PrintMessageInColor("Atmos Components:\n", c2)
	u.PrintMessage("https://atmos.tools/core-concepts/components\n")

	u.PrintMessageInColor("Atmos Stacks:\n", c2)
	u.PrintMessage("https://atmos.tools/core-concepts/stacks\n")

	u.PrintMessageInColor("Quick Start:\n", c2)
	u.PrintMessage("https://atmos.tools/quick-start\n")
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
		u.LogWarning(fmt.Sprintf("Could not load cache: %s", err))
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
		u.LogWarning(fmt.Sprintf("Failed to retrieve latest Atmos release info: %s", err))
		return
	}

	if latestReleaseTag == "" {
		u.LogWarning("No release information available")
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
		u.LogWarning(fmt.Sprintf("Unable to save cache: %s", saveErr))
	}
}

// Check Atmos is version command
func isVersionCommand() bool {
	return len(os.Args) > 1 && os.Args[1] == "version"
}

// handleHelpRequest shows help content and exits only if the first argument is "help" or "--help" or "-h"
func handleHelpRequest(cmd *cobra.Command, args []string) {
	if (len(args) > 0 && args[0] == "help") || Contains(args, "--help") || Contains(args, "-h") {
		cmd.Help()
		os.Exit(0)
	}
}

// showUsageAndExit we display the markdown usage or fallback to our custom usage
// Markdown usage is not compatible with all outputs. We should therefore have fallback option.
func showUsageAndExit(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		showErrorExampleFromMarkdown(cmd, "")
	}
	if len(args) > 0 {
		showErrorExampleFromMarkdown(cmd, args[0])
	}
	os.Exit(1)
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
	os.Exit(1)
	return nil
}

// getConfigAndStacksInfo processes the CLI config and stacks
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
	if err != nil {
		u.LogErrorAndExit(err)
	}
	return info
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
	u.PrintInvalidUsageErrorAndExit(errors.New(details), suggestion)
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
