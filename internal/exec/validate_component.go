package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
)

// ExecuteValidateComponentCmd executes `validate component` command
func ExecuteValidateComponentCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument 'componentName'")
	}

	componentName := args[0]

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

	err = ExecuteValidateComponent(componentName, stack, schemaPath, schemaType)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema, OPA or CUE schema documents
func ExecuteValidateComponent(componentName string, stack string, schemaPath string, schemaType string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = componentName
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

	componentSection := configAndStacksInfo.ComponentSection

	return ValidateComponent(componentName, componentSection, schemaPath, schemaType)
}

// ValidateComponent validates the component config using JsonSchema, OPA or CUE schema documents
func ValidateComponent(componentName string, componentSection any, schemaPath string, schemaType string) error {
	if schemaPath != "" && schemaType != "" {
		u.PrintInfo(fmt.Sprintf("Validating component '%s' using schema file '%s' of type '%s'", componentName, schemaPath, schemaType))

		err := validateComponentInternal(componentSection, schemaPath, schemaType)
		if err != nil {
			return err
		}
	} else {
		validations, err := FindValidationSection(componentSection.(map[string]any))
		if err != nil {
			return err
		}

		for _, v := range validations {
			schemaPath = v.SchemaPath
			schemaType = v.SchemaType

			if v.Description != "" {
				u.PrintInfo(v.Description)
			} else {
				u.PrintInfo(fmt.Sprintf("Validating component '%s' using schema file '%s' of type '%s'", componentName, schemaPath, schemaType))
			}

			err = validateComponentInternal(componentSection, schemaPath, schemaType)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func validateComponentInternal(componentSection any, schemaPath string, schemaType string) error {
	var msg string

	if schemaType != "jsonschema" && schemaType != "opa" && schemaType != "cue" {
		return fmt.Errorf("invalid schema type '%s'. Supported types: jsonschema, opa, cue", schemaType)
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
			return fmt.Errorf("the file '%s' does not exist for schema type '%s'", schemaPath, schemaType)
		}
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	schemaText := string(fileContent)

	switch schemaType {
	case "jsonschema":
		{
			_, msg, err = ValidateWithJsonSchema(componentSection, filePath, schemaText)
			if err != nil {
				return err
			}
		}
	case "opa":
		{
			_, msg, err = ValidateWithOpa(componentSection, filePath, schemaText)
			if err != nil {
				return err
			}
		}
	case "cue":
		{
			_, msg, err = ValidateWithCue(componentSection, filePath, schemaText)
			if err != nil {
				return err
			}
		}
	}

	u.PrintMessage(msg)
	fmt.Println()
	return nil
}

// FindValidationSection finds 'validation' section in the component config
func FindValidationSection(componentSection map[string]any) (c.Validation, error) {
	validationSection := map[any]any{}

	if i, ok := componentSection["settings"].(map[any]any); ok {
		if i2, ok2 := i["validation"].(map[any]any); ok2 {
			validationSection = i2
		}
	}

	var result c.Validation

	err := mapstructure.Decode(validationSection, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
