package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// This map contains the existing atmos top-level commands
	// All custom top-level commands will be checked against this map in order to not override `atmos` top-level commands,
	// but just add subcommands to them
	existingTopLevelCommands = map[string]*cobra.Command{
		"atlantis":   atlantisCmd,
		"aws":        awsCmd,
		"completion": completionCmd,
		"describe":   describeCmd,
		"helmfile":   helmfileCmd,
		"terraform":  terraformCmd,
		"validate":   validateCmd,
		"vendor":     vendorCmd,
		"version":    versionCmd,
		"workflow":   workflowCmd,
	}
)

// processCustomCommands processes and executes custom commands
func processCustomCommands(commands []cfg.Command, parentCommand *cobra.Command, topLevel bool) error {
	var command *cobra.Command

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
					executeCustomCommand(cmd, args, parentCommand, commandConfig)
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

		err = processCustomCommands(commandConfig.Commands, command, false)
		if err != nil {
			return err
		}
	}

	return nil
}

// preCustomCommand is run before a custom command is executed
func preCustomCommand(cmd *cobra.Command, args []string, parentCommand *cobra.Command, commandConfig *cfg.Command) {
	var err error

	if len(args) != len(commandConfig.Arguments) {
		err = fmt.Errorf("invalid number of arguments, %d argument(s) required", len(commandConfig.Arguments))
		u.PrintErrorToStdErrorAndExit(err)
	}

	// no "steps" means a sub command should be specified
	if len(commandConfig.Steps) == 0 {
		_ = cmd.Help()
		os.Exit(0)
	}
}

// executeCustomCommand executes a custom command
func executeCustomCommand(cmd *cobra.Command, args []string, parentCommand *cobra.Command, commandConfig *cfg.Command) {
	var err error

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
					u.PrintErrorToStdErrorAndExit(err)
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
			component, err := u.ProcessTmpl(fmt.Sprintf("component-config-component-%d", i), commandConfig.ComponentConfig.Component, data)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
			if component == "" || component == "<no value>" {
				u.PrintErrorToStdErrorAndExit(fmt.Errorf("the command defines an invalid 'component_config.component: %s' in '%s'",
					commandConfig.ComponentConfig.Component, cfg.CliConfigFileName))
			}

			// Process Go templates in the command's 'component_config.stack'
			stack, err := u.ProcessTmpl(fmt.Sprintf("component-config-stack-%d", i), commandConfig.ComponentConfig.Stack, data)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
			if stack == "" || stack == "<no value>" {
				u.PrintErrorToStdErrorAndExit(fmt.Errorf("the command defines an invalid 'component_config.stack: %s' in '%s'",
					commandConfig.ComponentConfig.Stack, cfg.CliConfigFileName))
			}

			// Get the config for the component in the stack
			componentConfig, err := e.ExecuteDescribeComponent(component, stack)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
			data["ComponentConfig"] = componentConfig
		}

		// Prepare ENV vars
		// ENV var values support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		var envVarsList []string
		for _, v := range commandConfig.Env {
			key := v.Key
			value := v.Value
			valCommand := v.ValueCommand

			if value != "" && valCommand != "" {
				err = fmt.Errorf("either 'value' or 'valueCommand' can be specified for the ENV var, but not both.\n"+
					"Custom command '%s %s' defines 'value=%s' and 'valueCommand=%s' for the ENV var '%s'",
					parentCommand.Name(), commandConfig.Name, value, valCommand, key)
				u.PrintErrorToStdErrorAndExit(err)
			}

			// If the command to get the value for the ENV var is provided, execute it
			if valCommand != "" {
				valCommandName := fmt.Sprintf("env-var-%s-valcommand", key)
				res, err := e.ExecuteShellAndReturnOutput(valCommand, valCommandName, ".", nil, false, commandConfig.Verbose)
				if err != nil {
					u.PrintErrorToStdErrorAndExit(err)
				}
				value = res
			} else {
				// Process Go templates in the values of the command's ENV vars
				value, err = u.ProcessTmpl(fmt.Sprintf("env-var-%d", i), value, data)
				if err != nil {
					u.PrintErrorToStdErrorAndExit(err)
				}
			}

			envVarsList = append(envVarsList, fmt.Sprintf("%s=%s", key, value))
			err = os.Setenv(key, value)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
		}

		if len(envVarsList) > 0 && commandConfig.Verbose {
			u.PrintInfo("\nUsing ENV vars:")
			for _, v := range envVarsList {
				u.PrintMessage(v)
			}
		}

		// Process Go templates in the command's steps.
		// Steps support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
		commandToRun, err := u.ProcessTmpl(fmt.Sprintf("step-%d", i), step, data)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}

		// Execute the command step
		commandName := fmt.Sprintf("%s-step-%d", commandConfig.Name, i)
		err = e.ExecuteShell(commandToRun, commandName, ".", envVarsList, false, commandConfig.Verbose)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	}
}

// cloneCommand clones a custom command config into a new struct
func cloneCommand(orig *cfg.Command) (*cfg.Command, error) {
	origJSON, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}

	clone := cfg.Command{}
	if err = json.Unmarshal(origJSON, &clone); err != nil {
		return nil, err
	}

	return &clone, nil
}
