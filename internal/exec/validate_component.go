package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
)

// ExecuteValidateComponentCmd executes `validate component` command
func ExecuteValidateComponentCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument 'component'")
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

	res, msg, err := ExecuteValidateComponent(component, stack, schemaPath, schemaType)
	if err != nil {
		return err
	}

	if res {
		u.PrintMessage(msg)
	} else {
		u.PrintError(errors.New(msg))
	}
	return nil
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema, OPA or CUE schema documents
func ExecuteValidateComponent(component string, stack string, schemaPath string, schemaType string) (bool, string, error) {
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
			return false, "", err
		}
	}

	componentSection := configAndStacksInfo.ComponentSection
	searchForValidationInComponentSettings := false

	if schemaPath == "" {
		searchForValidationInComponentSettings = true
	}
	if schemaType == "" {
		searchForValidationInComponentSettings = true
	}

	if searchForValidationInComponentSettings {

	}

	return ExecuteValidateComponentSection(componentSection, schemaPath, schemaType)
}

// ExecuteValidateComponentSection validates the component config using JsonSchema, OPA or CUE schema documents
func ExecuteValidateComponentSection(componentSection any, schemaPath string, schemaType string) (bool, string, error) {
	if schemaType != "jsonschema" && schemaType != "opa" && schemaType != "cue" {
		return false, "", fmt.Errorf("invalid 'schema-type=%s' argument. Supported values: jsonschema (default), opa, cue", schemaType)
	}

	// Check if the file pointed to by 'schemaPath' exists.
	// If not, join it with the schemas `base_path` from the CLI config
	var filePath string
	if u.FileExists(schemaPath) {
		filePath = schemaPath
	} else {
		switch schemaType {
		case "jsonschema":
			{
				filePath = path.Join(c.Config.BasePath, c.Config.Schemas.JsonSchema.BasePath, schemaPath)
			}
		case "opa":
			{
				filePath = path.Join(c.Config.BasePath, c.Config.Schemas.Opa.BasePath, schemaPath)
			}
		case "cue":
			{
				filePath = path.Join(c.Config.BasePath, c.Config.Schemas.Cue.BasePath, schemaPath)
			}
		}

		if !u.FileExists(filePath) {
			return false, "", fmt.Errorf("the file '%s' does not exist for schema type '%s'", schemaPath, schemaType)
		}
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return false, "", err
	}

	schemaText := string(fileContent)

	return ValidateComponentConfig(componentSection, schemaType, filePath, schemaText)
}

// ValidateComponentConfig validates the component configuration using the provided schema document
func ValidateComponentConfig(componentConfig any, schemaType string, schemaName string, schemaText string) (bool, string, error) {
	switch schemaType {
	case "jsonschema":
		{
			return ValidateWithJsonSchema(componentConfig, schemaName, schemaText)
		}
	case "opa":
		{
			return ValidateWithOpa(componentConfig, schemaName, schemaText)
		}
	case "cue":
		{
			return ValidateWithCue(componentConfig, schemaName, schemaText)
		}
	}

	return false, "", fmt.Errorf("invalid 'schema type '%s'. Supported values: jsonschema (default), opa, cue", schemaType)
}
