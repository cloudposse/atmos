package cmd

import (
	"bytes"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"text/template"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// This map contains the existing atmos top-level commands
	// All custom top-level commands will be checked against this map in order to not override `atmos` top-level commands,
	// but just add subcommands to them
	existingTopLevelCommands = map[string]*cobra.Command{
		"atlantis":  atlantisCmd,
		"aws":       awsCmd,
		"describe":  describeCmd,
		"helmfile":  helmfileCmd,
		"terraform": terraformCmd,
		"validate":  validateCmd,
		"vendor":    vendorCmd,
		"version":   versionCmd,
		"workflow":  workflowCmd,
	}
)

func processCustomCommands(commands []cfg.Command, parentCommand *cobra.Command, topLevel bool) error {
	var command *cobra.Command

	for _, commandConfig := range commands {
		if _, exist := existingTopLevelCommands[commandConfig.Name]; exist && topLevel {
			command = existingTopLevelCommands[commandConfig.Name]
		} else {
			// Deep-copy the slices because of the automatic closure in the `Run` function of the command.
			// https://www.calhoun.io/gotchas-and-common-mistakes-with-closures-in-go/
			// It will make a closure on the new local variables which are different in each iteration.
			customCommandSteps := make([]string, len(commandConfig.Steps))
			copy(customCommandSteps, commandConfig.Steps)
			customCommandArguments := make([]cfg.CommandArgument, len(commandConfig.Arguments))
			copy(customCommandArguments, commandConfig.Arguments)
			customCommandFlags := make([]cfg.CommandFlag, len(commandConfig.Flags))
			copy(customCommandFlags, commandConfig.Flags)
			customEnvVars := make([]cfg.CommandEnv, len(commandConfig.Env))
			copy(customEnvVars, commandConfig.Env)

			var customCommand = &cobra.Command{
				Use:   commandConfig.Name,
				Short: commandConfig.Description,
				Long:  commandConfig.Description,
				Run: func(cmd *cobra.Command, args []string) {
					var err error
					var t *template.Template

					if len(args) != len(customCommandArguments) {
						err = fmt.Errorf("invalid number of arguments, %d argument(s) required", len(customCommandArguments))
						u.PrintErrorToStdErrorAndExit(err)
					}

					// Execute custom command's steps
					for i, step := range customCommandSteps {
						// Prepare template data for arguments
						argumentsData := map[string]string{}
						for ix, arg := range customCommandArguments {
							argumentsData[arg.Name] = args[ix]
						}

						// Prepare template data for flags
						flags := cmd.Flags()
						flagsData := map[string]string{}
						for _, fl := range customCommandFlags {
							if fl.Type == "" || fl.Type == "string" {
								providedFlag, err := flags.GetString(fl.Name)
								if err != nil {
									u.PrintErrorToStdErrorAndExit(err)
								}
								flagsData[fl.Name] = providedFlag
							}
						}

						// Prepare full template data
						var data = map[string]map[string]string{
							"Arguments": argumentsData,
							"Flags":     flagsData,
						}

						// Parse and execute Go templates in the command's steps
						t, err = template.New(fmt.Sprintf("step-%d", i)).Parse(step)
						if err != nil {
							u.PrintErrorToStdErrorAndExit(err)
						}
						var tpl bytes.Buffer
						err = t.Execute(&tpl, data)
						if err != nil {
							u.PrintErrorToStdErrorAndExit(err)
						}

						// Prepare ENV vars
						var envVarsList []string
						for _, v := range customEnvVars {
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
								valCommandArgs := strings.Fields(valCommand)
								res, err := e.ExecuteShellCommandAndReturnOutput(valCommandArgs[0], valCommandArgs[1:], ".", nil, false, commandConfig.Verbose)
								if err != nil {
									u.PrintErrorToStdErrorAndExit(err)
								}
								value = res
							} else {
								// Parse and execute Go templates in the values of the command's ENV vars
								t, err = template.New(fmt.Sprintf("env-var-%d", i)).Parse(value)
								if err != nil {
									u.PrintErrorToStdErrorAndExit(err)
								}
								var tplEnvVarValue bytes.Buffer
								err = t.Execute(&tplEnvVarValue, data)
								if err != nil {
									u.PrintErrorToStdErrorAndExit(err)
								}
								value = tplEnvVarValue.String()
							}

							envVarsList = append(envVarsList, fmt.Sprintf("%s=%s", key, value))
							err = os.Setenv(key, value)
							if err != nil {
								u.PrintErrorToStdErrorAndExit(err)
							}
						}

						if len(envVarsList) > 0 {
							u.PrintInfo("\nUsing ENV vars:")
							for _, v := range envVarsList {
								fmt.Println(v)
							}
						}

						commandToRun := os.ExpandEnv(tpl.String())

						// Execute the command step
						stepArgs := strings.Fields(commandToRun)
						err = e.ExecuteShellCommand(stepArgs[0], stepArgs[1:], ".", envVarsList, false, commandConfig.Verbose)
						if err != nil {
							u.PrintErrorToStdErrorAndExit(err)
						}
					}
				},
			}

			// Add customCommandFlags
			for _, flag := range customCommandFlags {
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

			parentCommand.AddCommand(customCommand)
			command = customCommand
		}

		err := processCustomCommands(commandConfig.Commands, command, false)
		if err != nil {
			return err
		}
	}

	return nil
}
