package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

var (
	existingTopLevelCommands = map[string]*cobra.Command{
		"terraform": terraformCmd,
		"helmfile":  helmfileCmd,
		"describe":  describeCmd,
		"aws":       awsCmd,
		"validate":  validateCmd,
		"vendor":    vendorCmd,
		"workflow":  workflowCmd,
		"version":   versionCmd,
	}
)

func processCustomCommands(commands []c.Command, parentCommand *cobra.Command, topLevel bool) error {
	var command *cobra.Command

	for _, commandConfig := range commands {
		if _, exist := existingTopLevelCommands[commandConfig.Name]; exist && topLevel {
			command = existingTopLevelCommands[commandConfig.Name]
		} else {
			// Deep-copy the `commandConfig.Steps` slice because of the automatic closure in the `Run` function of the command
			// It will make a closure on the new local variable `steps` which are different in each iteration
			// https://www.calhoun.io/gotchas-and-common-mistakes-with-closures-in-go/
			steps := make([]string, len(commandConfig.Steps))
			copy(steps, commandConfig.Steps)

			var customCommand = &cobra.Command{
				Use:   commandConfig.Name,
				Short: commandConfig.Description,
				Long:  commandConfig.Description,
				Run: func(cmd *cobra.Command, args []string) {
					for _, step := range steps {
						stepArgs := strings.Fields(step)
						err := e.ExecuteShellCommand(stepArgs[0], stepArgs[1:], ".", nil, false)
						if err != nil {
							u.PrintErrorToStdErrorAndExit(err)
						}
					}
				},
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
