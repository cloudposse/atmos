package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// ExecuteValidateComponentCmd executes `validate component` command
func ExecuteValidateComponentCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	component := args[0]

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	schemaPath, err := flags.GetString("schema-path")
	if err != nil {
		return err
	}

	schemaType, err := flags.GetString("schema-type")
	if err != nil {
		return err
	}

	return ExecuteValidateComponent(component, stack, schemaPath, schemaType)
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema, OPA or CUE schema
func ExecuteValidateComponent(component string, stack string, schemaPath string, schemaType string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err := ProcessStacks(configAndStacksInfo, true)
	if err != nil {
		u.PrintErrorVerbose(err)
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = ProcessStacks(configAndStacksInfo, true)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	err = u.PrintAsYAML(configAndStacksInfo.ComponentSection)
	if err != nil {
		return err
	}

	return nil
}
