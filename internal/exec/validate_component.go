package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteValidateComponentCmd executes `validate component` command
func ExecuteValidateComponentCmd(cmd *cobra.Command, args []string) (string, string, error) {
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return "", "", err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return "", "", err
	}

	if len(args) != 1 {
		return "", "", errors.New("invalid arguments. The command requires one argument 'componentName'")
	}

	componentName := args[0]

	// Initialize spinner
	message := fmt.Sprintf("Validating Atmos Component: %s", componentName)
	p := NewSpinner(message)
	spinnerDone := make(chan struct{})
	// Run spinner in a goroutine
	RunSpinner(p, spinnerDone, message)
	// Ensure spinner is stopped before returning
	defer StopSpinner(p, spinnerDone)

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return "", "", err
	}

	schemaPath, err := flags.GetString("schema-path")
	if err != nil {
		return "", "", err
	}

	schemaType, err := flags.GetString("schema-type")
	if err != nil {
		return "", "", err
	}

	modulePaths, err := flags.GetStringSlice("module-paths")
	if err != nil {
		return "", "", err
	}

	timeout, err := flags.GetInt("timeout")
	if err != nil {
		return "", "", err
	}

	_, err = ExecuteValidateComponent(atmosConfig, info, componentName, stack, schemaPath, schemaType, modulePaths, timeout)
	if err != nil {
		return "", "", err
	}

	return componentName, stack, nil
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema, OPA or CUE schema documents
func ExecuteValidateComponent(
	atmosConfig schema.AtmosConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	componentName string,
	stack string,
	schemaPath string,
	schemaType string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	configAndStacksInfo.ComponentFromArg = componentName
	configAndStacksInfo.Stack = stack

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err := ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
		if err != nil {
			return false, err
		}
	}

	componentSection := configAndStacksInfo.ComponentSection

	return ValidateComponent(atmosConfig, componentName, componentSection, schemaPath, schemaType, modulePaths, timeoutSeconds)
}

// ValidateComponent validates the component config using JsonSchema, OPA or CUE schema documents
func ValidateComponent(
	atmosConfig schema.AtmosConfiguration,
	componentName string,
	componentSection any,
	schemaPath string,
	schemaType string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	ok := true
	var err error

	if schemaPath != "" && schemaType != "" {
		u.LogDebug(fmt.Sprintf("\nValidating the component '%s' using '%s' file '%s'", componentName, schemaType, schemaPath))

		ok, err = validateComponentInternal(atmosConfig, componentSection, schemaPath, schemaType, modulePaths, timeoutSeconds)
		if err != nil {
			return false, err
		}
	} else {
		validations, err := FindValidationSection(componentSection.(map[string]any))
		if err != nil {
			return false, err
		}

		for _, v := range validations {
			if v.Disabled {
				continue
			}

			// Command line parameters override the validation config in YAML
			var finalSchemaPath string
			var finalSchemaType string
			var finalModulePaths []string
			var finalTimeoutSeconds int

			if schemaPath != "" {
				finalSchemaPath = schemaPath
			} else {
				finalSchemaPath = v.SchemaPath
			}

			if schemaType != "" {
				finalSchemaType = schemaType
			} else {
				finalSchemaType = v.SchemaType
			}

			if len(modulePaths) > 0 {
				finalModulePaths = modulePaths
			} else {
				finalModulePaths = v.ModulePaths
			}

			if timeoutSeconds > 0 {
				finalTimeoutSeconds = timeoutSeconds
			} else {
				finalTimeoutSeconds = v.Timeout
			}

			u.LogDebug(fmt.Sprintf("\nValidating the component '%s' using '%s' file '%s'", componentName, finalSchemaType, finalSchemaPath))

			if v.Description != "" {
				u.LogDebug(v.Description)
			}

			ok2, err := validateComponentInternal(atmosConfig, componentSection, finalSchemaPath, finalSchemaType, finalModulePaths, finalTimeoutSeconds)
			if err != nil {
				return false, err
			}
			if !ok2 {
				ok = false
			}
		}
	}

	return ok, nil
}

func validateComponentInternal(
	atmosConfig schema.AtmosConfiguration,
	componentSection any,
	schemaPath string,
	schemaType string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	if schemaType != "jsonschema" && schemaType != "opa" {
		return false, fmt.Errorf("invalid schema type '%s'. Supported types: jsonschema, opa", schemaType)
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
				filePath = filepath.Join(atmosConfig.BasePath, atmosConfig.GetResourcePath("jsonschema").BasePath, schemaPath)
			}
		case "opa":
			{
				filePath = filepath.Join(atmosConfig.BasePath, atmosConfig.GetResourcePath("opa").BasePath, schemaPath)
			}
		}

		if !u.FileExists(filePath) {
			return false, fmt.Errorf("the file '%s' does not exist for schema type '%s'", schemaPath, schemaType)
		}
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	schemaText := string(fileContent)
	var ok bool

	switch schemaType {
	case "jsonschema":
		{
			ok, err = ValidateWithJsonSchema(componentSection, filePath, schemaText)
			if err != nil {
				return false, err
			}
		}
	case "opa":
		{
			modulePathsAbsolute, err := u.JoinAbsolutePathWithPaths(filepath.Join(atmosConfig.BasePath, atmosConfig.GetResourcePath("opa").BasePath), modulePaths)
			if err != nil {
				return false, err
			}

			ok, err = ValidateWithOpa(componentSection, filePath, modulePathsAbsolute, timeoutSeconds)
			if err != nil {
				return false, err
			}
		}
	case "opa_legacy":
		{
			ok, err = ValidateWithOpaLegacy(componentSection, filePath, schemaText, timeoutSeconds)
			if err != nil {
				return false, err
			}
		}
	}

	return ok, nil
}

// FindValidationSection finds 'validation' section in the component config
func FindValidationSection(componentSection map[string]any) (schema.Validation, error) {
	validationSection := map[string]any{}

	if i, ok := componentSection["settings"].(map[string]any); ok {
		if i2, ok2 := i["validation"].(map[string]any); ok2 {
			validationSection = i2
		}
	}

	var result schema.Validation

	err := mapstructure.Decode(validationSection, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
