package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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
	cliConfig schema.CliConfiguration,
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
			var customCommand = &cobra.Command{
				Use:   commandConfig.Name,
				Short: commandConfig.Description,
				Long:  commandConfig.Description,
				PreRun: func(cmd *cobra.Command, args []string) {
					preCustomCommand(cmd, args, parentCommand, commandConfig)
				},
				Run: func(cmd *cobra.Command, args []string) {
					executeCustomCommand(cliConfig, cmd, args, parentCommand, commandConfig)
				},
			}

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

		err = processCustomCommands(cliConfig, commandConfig.Commands, command, false)
		if err != nil {
			return err
		}
	}

	return nil
}

// processCommandAliases processes the command aliases
func processCommandAliases(
	cliConfig schema.CliConfiguration,
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
			aliasFor := fmt.Sprintf("alias for '%s'", aliasCmd)

			var aliasCommand = &cobra.Command{
				Use:                alias,
				Short:              aliasFor,
				Long:               aliasFor,
				FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
				Run: func(cmd *cobra.Command, args []string) {
					err := cmd.ParseFlags(args)
					if err != nil {
						u.LogErrorAndExit(cliConfig, err)
					}

					commandToRun := fmt.Sprintf("%s %s %s", os.Args[0], aliasCmd, strings.Join(args, " "))
					err = e.ExecuteShell(cliConfig, commandToRun, commandToRun, ".", nil, false)
					if err != nil {
						u.LogErrorAndExit(cliConfig, err)
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
	if len(args) != len(commandConfig.Arguments) {
		if len(commandConfig.Arguments) == 0 {
			u.LogError(schema.CliConfiguration{}, errors.New("invalid command"))
			sb.WriteString("Available command(s):\n")
			for i, c := range commandConfig.Commands {
				sb.WriteString(fmt.Sprintf("%d. %s %s %s\n", i+1, parentCommand.Use, commandConfig.Name, c.Name))
			}
			u.LogInfo(schema.CliConfiguration{}, sb.String())
			os.Exit(1)
		}
		sb.WriteString(fmt.Sprintf("Command requires %d argument(s):\n", len(commandConfig.Arguments)))
		for i, arg := range commandConfig.Arguments {
			if arg.Name == "" {
				u.LogErrorAndExit(schema.CliConfiguration{}, errors.New("invalid argument configuration: empty argument name"))
			}
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, arg.Name))
		}
		if len(args) > 0 {
			sb.WriteString(fmt.Sprintf("\nReceived %d argument(s): %s", len(args), strings.Join(args, ", ")))
		}
		u.LogErrorAndExit(schema.CliConfiguration{}, errors.New(sb.String()))
	}

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
	cliConfig schema.CliConfiguration,
	cmd *cobra.Command,
	args []string,
	parentCommand *cobra.Command,
	commandConfig *schema.Command,
) {
	var err error

	if commandConfig.Verbose {
		cliConfig.Logs.Level = u.LogLevelTrace
	}

	// Execute custom command's steps
	for i, step := range commandConfig.Steps {
		// Prepare template data for arguments
		argumentsData := map[string]string{}
		for ix, arg := range commandConfig.Arguments {
			argumentsData[arg.Name] = args[ix]
		}

		// Prepare template data for flags
		flags := cmd.Flags()
		flagsData := map[string]any{}
		for _, fl := range commandConfig.Flags {
			if fl.Type == "" || fl.Type == "string" {
				providedFlag, err := flags.GetString(fl.Name)
				if err != nil {
					u.LogErrorAndExit(cliConfig, err)
				}
				flagsData[fl.Name] = providedFlag
			} else if fl.Type == "bool" {
				boolFlag, err := flags.GetBool(fl.Name)
				if err != nil {
					u.LogErrorAndExit(cliConfig, err)
				}
				flagsData[fl.Name] = boolFlag
			}
		}

		// Prepare template data
		var data = map[string]any{
			"Arguments": argumentsData,
			"Flags":     flagsData,
		}

		// If the custom command defines 'component_config' section with 'component' and 'stack' attributes,
		// process the component stack config and expose it in {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		if commandConfig.ComponentConfig.Component != "" && commandConfig.ComponentConfig.Stack != "" {
			// Process Go templates in the command's 'component_config.component'
			component, err := e.ProcessTmpl(fmt.Sprintf("component-config-component-%d", i), commandConfig.ComponentConfig.Component, data, false)
			if err != nil {
				u.LogErrorAndExit(cliConfig, err)
			}
			if component == "" || component == "<no value>" {
				u.LogErrorAndExit(cliConfig, fmt.Errorf("the command defines an invalid 'component_config.component: %s' in '%s'",
					commandConfig.ComponentConfig.Component, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension))
			}

			// Process Go templates in the command's 'component_config.stack'
			stack, err := e.ProcessTmpl(fmt.Sprintf("component-config-stack-%d", i), commandConfig.ComponentConfig.Stack, data, false)
			if err != nil {
				u.LogErrorAndExit(cliConfig, err)
			}
			if stack == "" || stack == "<no value>" {
				u.LogErrorAndExit(cliConfig, fmt.Errorf("the command defines an invalid 'component_config.stack: %s' in '%s'",
					commandConfig.ComponentConfig.Stack, cfg.CliConfigFileName+u.DefaultStackConfigFileExtension))
			}

			// Get the config for the component in the stack
			componentConfig, err := e.ExecuteDescribeComponent(component, stack, true)
			if err != nil {
				u.LogErrorAndExit(cliConfig, err)
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
				u.LogErrorAndExit(cliConfig, err)
			}

			// If the command to get the value for the ENV var is provided, execute it
			if valCommand != "" {
				valCommandName := fmt.Sprintf("env-var-%s-valcommand", key)
				res, err := e.ExecuteShellAndReturnOutput(cliConfig, valCommand, valCommandName, ".", nil, false)
				if err != nil {
					u.LogErrorAndExit(cliConfig, err)
				}
				value = strings.TrimRight(res, "\r\n")
			} else {
				// Process Go templates in the values of the command's ENV vars
				value, err = e.ProcessTmpl(fmt.Sprintf("env-var-%d", i), value, data, false)
				if err != nil {
					u.LogErrorAndExit(cliConfig, err)
				}
			}

			envVarsList = append(envVarsList, fmt.Sprintf("%s=%s", key, value))
			err = os.Setenv(key, value)
			if err != nil {
				u.LogErrorAndExit(cliConfig, err)
			}
		}

		if len(envVarsList) > 0 && commandConfig.Verbose {
			u.LogDebug(cliConfig, "\nUsing ENV vars:")
			for _, v := range envVarsList {
				u.LogDebug(cliConfig, v)
			}
		}

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		commandToRun, err := e.ProcessTmpl(fmt.Sprintf("step-%d", i), step, data, false)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}

		// Execute the command step
		commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)
		err = e.ExecuteShell(cliConfig, commandToRun, commandName, ".", envVarsList, false)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}
	}
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

	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	if vCfg.CheckStack {
		atmosConfigExists, err := u.IsDirectory(cliConfig.StacksBaseAbsolutePath)
		if !atmosConfigExists || err != nil {
			printMessageForMissingAtmosConfig(cliConfig)
			os.Exit(0)
		}
	}
}

// printMessageForMissingAtmosConfig prints Atmos logo and instructions on how to configure and start using Atmos
func printMessageForMissingAtmosConfig(cliConfig schema.CliConfiguration) {
	c1 := color.New(color.FgCyan)
	c2 := color.New(color.FgGreen)

	fmt.Println()
	err := tuiUtils.PrintStyledText("ATMOS")
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	if cliConfig.Default {
		// If Atmos did not find an `atmos.yaml` config file and is using the default config
		u.PrintMessageInColor("atmos.yaml", c1)
		fmt.Println(" CLI config file was not found.")
		fmt.Print("\nThe default Atmos stacks directory is set to ")
		u.PrintMessageInColor(path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath), c1)
		fmt.Println(",\nbut the directory does not exist in the current path.")
	} else {
		// If Atmos found an `atmos.yaml` config file, but it defines invalid paths to Atmos stacks and components
		u.PrintMessageInColor("atmos.yaml", c1)
		fmt.Print(" CLI config file specifies the directory for Atmos stacks as ")
		u.PrintMessageInColor(path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath), c1)
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
func CheckForAtmosUpdateAndPrintMessage(cliConfig schema.CliConfiguration) {
	// If version checking is disabled in the configuration, do nothing
	if !cliConfig.Version.Check.Enabled {
		return
	}

	// Load the cache
	cacheCfg, err := cfg.LoadCache()
	if err != nil {
		u.LogWarning(cliConfig, fmt.Sprintf("Could not load cache: %s", err))
		return
	}

	// Determine if it's time to check for updates based on frequency and last_checked
	if !cfg.ShouldCheckForUpdates(cacheCfg.LastChecked, cliConfig.Version.Check.Frequency) {
		// Not due for another check yet, so return without printing anything
		return
	}

	// Get the latest Atmos release from GitHub
	latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
	if err != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("Failed to retrieve latest Atmos release info: %s", err))
		return
	}

	if latestReleaseTag == "" {
		// No releases found or empty string, return silently
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
		u.LogWarning(cliConfig, fmt.Sprintf("Unable to save cache: %s", saveErr))

	}
}

func customHelpMessageToUpgradeToAtmosLatestRelease(cmd *cobra.Command, args []string) {
	originalHelpFunc(cmd, args)
	CheckForAtmosUpdateAndPrintMessage(cliConfig)
}

// Check Atmos is version command
func isVersionCommand() bool {
	return len(os.Args) > 1 && os.Args[1] == "version"
}
