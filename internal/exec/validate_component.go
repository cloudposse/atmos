package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"

	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
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

	res, err := ExecuteValidateComponent(component, stack, schemaPath, schemaType)
	if err != nil {
		return err
	}

	fmt.Println(res)
	return nil
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema, OPA or CUE schema documents
func ExecuteValidateComponent(component string, stack string, schemaPath string, schemaType string) (string, error) {
	if schemaType == "" {
		schemaType = "jsonschema"
	}

	if schemaType != "jsonschema" && schemaType != "opa" && schemaType != "cue" {
		return "", fmt.Errorf("invalid 'schema-type=%s' argument. Supported values: jsonschema (default), opa, cue", schemaType)
	}

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
			return "", err
		}
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
			return "", fmt.Errorf("the file '%s' does not exist", schemaPath)
		}
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	schemaText := string(fileContent)
	componentSection := configAndStacksInfo.ComponentSection

	switch schemaType {
	case "jsonschema":
		{
			return ValidateWithJsonSchema(componentSection, filePath, schemaText)
		}
	case "opa":
		{
			return ValidateWithOpa(componentSection, filePath, schemaText)
		}
	case "cue":
		{
			return ValidateWithCue(componentSection, filePath, schemaText)
		}
	}

	return "", nil
}
