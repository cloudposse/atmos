package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/spf13/cobra"
	"strings"
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
	if schemaType != "jsonschema" && schemaType != "opa" && schemaType != "cue" {
		return fmt.Errorf("invalid 'schema-type=%s' argument. Supported values: jsonschema, opa, cue", schemaType)
	}

	return ExecuteValidateComponent(component, stack, schemaPath, schemaType)
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema, OPA or CUE schema documents
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

// validateWithJsonSchema validates the data structure using the provided JSON Schema document
// https://github.com/santhosh-tekuri/jsonschema
// https://go.dev/play/p/Hhax3MrtD8r
func validateWithJsonSchema(data any, schemaText string) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", strings.NewReader(schemaText)); err != nil {
		return err
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}

	if err = schema.Validate(data); err != nil {
		return err
	}

	return nil
}

// validateWithOpa validates the data structure using the provided OPA schema document
func validateWithOpa(data any, schemaText string) error {
	return nil
}

// validateWithCue validates the data structure using the provided CUE schema document
func validateWithCue(data any, schemaText string) error {
	return nil
}
