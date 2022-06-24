package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

func processCustomCommands(commands []c.Command, rootCommand *cobra.Command) error {
	for _, commandConfig := range commands {
		// Deep-copy the `commandConfig.Steps` slice because of the automatic closure in the `Run` function of the command
		// It will make a closure on the new local variable `steps` which are different in each iteration
		// https://www.calhoun.io/gotchas-and-common-mistakes-with-closures-in-go/
		steps := make([]string, len(commandConfig.Steps))
		copy(steps, commandConfig.Steps)

		var command = &cobra.Command{
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
		rootCommand.AddCommand(command)

		err := processCustomCommands(commandConfig.Commands, command)
		if err != nil {
			return err
		}
	}

	return nil
}
