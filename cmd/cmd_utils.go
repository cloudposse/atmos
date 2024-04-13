package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
						u.LogErrorAndExit(err)
					}

					commandToRun := fmt.Sprintf("%s %s %s", os.Args[0], aliasCmd, strings.Join(args, " "))
					err = e.ExecuteShell(cliConfig, commandToRun, commandToRun, ".", nil, false)
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
	var err error

	if len(args) != len(commandConfig.Arguments) {
		err = fmt.Errorf("invalid number of arguments, %d argument(s) required", len(commandConfig.Arguments))
		u.LogErrorAndExit(err)
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
		flagsData := map[string]string{}
		for _, fl := range commandConfig.Flags {
			if fl.Type == "" || fl.Type == "string" {
				providedFlag, err := flags.GetString(fl.Name)
				if err != nil {
					u.LogErrorAndExit(err)
				}
				flagsData[fl.Name] = providedFlag
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
			component, err := u.ProcessTmpl(fmt.Sprintf("component-config-component-%d", i), commandConfig.ComponentConfig.Component, data, false)
			if err != nil {
				u.LogErrorAndExit(err)
			}
			if component == "" || component == "<no value>" {
				u.LogErrorAndExit(fmt.Errorf("the command defines an invalid 'component_config.component: %s' in '%s'",
					commandConfig.ComponentConfig.Component, cfg.CliConfigFileName))
			}

			// Process Go templates in the command's 'component_config.stack'
			stack, err := u.ProcessTmpl(fmt.Sprintf("component-config-stack-%d", i), commandConfig.ComponentConfig.Stack, data, false)
			if err != nil {
				u.LogErrorAndExit(err)
			}
			if stack == "" || stack == "<no value>" {
				u.LogErrorAndExit(fmt.Errorf("the command defines an invalid 'component_config.stack: %s' in '%s'",
					commandConfig.ComponentConfig.Stack, cfg.CliConfigFileName))
			}

			// Get the config for the component in the stack
			componentConfig, err := e.ExecuteDescribeComponent(component, stack)
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
				res, err := e.ExecuteShellAndReturnOutput(cliConfig, valCommand, valCommandName, ".", nil, false)
				if err != nil {
					u.LogErrorAndExit(err)
				}
				value = strings.TrimRight(res, "\r\n")
			} else {
				// Process Go templates in the values of the command's ENV vars
				value, err = u.ProcessTmpl(fmt.Sprintf("env-var-%d", i), value, data, false)
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
			u.LogDebug(cliConfig, "\nUsing ENV vars:")
			for _, v := range envVarsList {
				u.LogDebug(cliConfig, v)
			}
		}

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		commandToRun, err := u.ProcessTmpl(fmt.Sprintf("step-%d", i), step, data, false)
		if err != nil {
			u.LogErrorAndExit(err)
		}

		// Execute the command step
		commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)
		err = e.ExecuteShell(cliConfig, commandToRun, commandName, ".", envVarsList, false)
		if err != nil {
			u.LogErrorAndExit(err)
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
func checkAtmosConfig() {
	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	atmosConfigExists, err := u.IsDirectory(cliConfig.StacksBaseAbsolutePath)

	if !atmosConfigExists || err != nil {
		printMessageForMissingAtmosConfig(cliConfig)
		os.Exit(0)
	}
}

// printMessageForMissingAtmosConfig prints Atmos logo and instructions on how to configure and start using Atmos
func printMessageForMissingAtmosConfig(cliConfig schema.CliConfiguration) {
	c1 := color.New(color.FgCyan)
	c2 := color.New(color.FgGreen)

	fmt.Println()
	err := tuiUtils.PrintStyledText("ATMOS")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	fmt.Print("Atmos CLI config ")
	u.PrintMessageInColor("stacks.base_path ", c1)
	fmt.Print("points to the ")
	u.PrintMessageInColor(path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath), c1)
	fmt.Println(" directory.")

	u.PrintMessage("The directory does not exist or has no Atmos stack configurations.\n")

	u.PrintMessage("To configure and start using Atmos, refer to the following documents:\n")

	u.PrintMessageInColor("Atmos CLI Configuration:\n", c2)
	u.PrintMessage("https://atmos.tools/cli/configuration\n")

	u.PrintMessageInColor("Atmos Components:\n", c2)
	u.PrintMessage("https://atmos.tools/core-concepts/components\n")

	u.PrintMessageInColor("Atmos Stacks:\n", c2)
	u.PrintMessage("https://atmos.tools/core-concepts/stacks\n")

	u.PrintMessageInColor("Quick Start:\n", c2)
	u.PrintMessage("https://atmos.tools/quick-start\n")
}

// printMessageToUpgradeToAtmosLatestRelease prints info on how to upgrade Atmos to the latest version
func printMessageToUpgradeToAtmosLatestRelease(latestVersion string) {
	c1 := color.New(color.FgCyan)
	c2 := color.New(color.FgGreen)

	u.PrintMessageInColor(fmt.Sprintf("\nYour version of Atmos is out of date. The latest version is %s\n\n", latestVersion), c1)
	u.PrintMessage("To upgrade Atmos, refer to the following links and documents:\n")

	u.PrintMessageInColor("Atmos Releases:\n", c2)
	u.PrintMessage("https://github.com/cloudposse/atmos/releases\n")

	u.PrintMessageInColor("Install Atmos:\n", c2)
	u.PrintMessage("https://atmos.tools/quick-start/install-atmos\n")
}
