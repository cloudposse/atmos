package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteValidateComponentCmd executes `validate component` command.
func ExecuteValidateComponentCmd(cmd *cobra.Command, args []string) (string, string, error) {
	defer perf.Track(nil, "exec.ExecuteValidateComponentCmd")()

	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return "", "", err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return "", "", err
	}

	if len(args) != 1 {
		return "", "", errUtils.ErrInvalidComponentArgument
	}

	componentName := args[0]

	// Initialize spinner.
	message := fmt.Sprintf("Validating Atmos Component: %s", componentName)
	p := NewSpinner(message)
	spinnerDone := make(chan struct{})
	// Run spinner in a goroutine
	RunSpinner(p, spinnerDone, message)
	// Ensure the spinner is stopped before returning
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

	_, err = ExecuteValidateComponent(&atmosConfig, info, componentName, stack, schemaPath, schemaType, modulePaths, timeout)
	if err != nil {
		u.PrintfMessageToTUI("\r%s Component validation failed\n", theme.Styles.XMark)
		return "", "", err
	}
	u.PrintfMessageToTUI("\r%s Component validated successfully\n", theme.Styles.Checkmark)
	log.Debug("Component validation completed", "component", componentName, "stack", stack)

	return componentName, stack, nil
}

// ExecuteValidateComponent validates a component in a stack using JsonSchema or OPA schema documents.
func ExecuteValidateComponent(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	componentName string,
	stack string,
	schemaPath string,
	schemaType string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteValidateComponent")()

	configAndStacksInfo.ComponentFromArg = componentName
	configAndStacksInfo.Stack = stack

	configAndStacksInfo.ComponentType = cfg.TerraformComponentType
	configAndStacksInfo, err := ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
	if err != nil {
		configAndStacksInfo.ComponentType = cfg.HelmfileComponentType
		configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
		if err != nil {
			configAndStacksInfo.ComponentType = cfg.PackerComponentType
			configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
			if err != nil {
				return false, err
			}
		}
	}

	componentSection := configAndStacksInfo.ComponentSection

	return ValidateComponent(atmosConfig, componentName, componentSection, schemaPath, schemaType, modulePaths, timeoutSeconds)
}

// ValidateComponent validates the component config using JsonSchema or OPA schema documents.
func ValidateComponent(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	componentSection map[string]any,
	schemaPath string,
	schemaType string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	defer perf.Track(atmosConfig, "exec.ValidateComponent")()

	ok := true
	var err error

	if schemaPath != "" && schemaType != "" {
		log.Debug("Validating", "component", componentName, "schema", schemaType, "file", schemaPath)

		ok, err = validateComponentInternal(atmosConfig, componentSection, schemaPath, schemaType, modulePaths, timeoutSeconds)
		if err != nil {
			return false, err
		}
	} else {
		validations, err := FindValidationSection(componentSection)
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

			log.Debug("Validating", "component", componentName, "schema", finalSchemaType, "file", finalSchemaPath)

			if v.Description != "" {
				log.Debug(v.Description)
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
	atmosConfig *schema.AtmosConfiguration,
	componentSection map[string]any,
	schemaPath string,
	schemaType string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	if schemaType != "jsonschema" && schemaType != "opa" {
		return false, fmt.Errorf("invalid schema type '%s'. Supported types: jsonschema, opa", schemaType)
	}

	// Check if the file pointed to by 'schemaPath' exists.
	// If not, join it with the schemas `base_path` from the CLI config.
	// Use BasePathAbsolute instead of BasePath to ensure correct resolution
	// when base_path is relative (e.g., when using ATMOS_CLI_CONFIG_PATH).
	var filePath string
	if u.FileExists(schemaPath) {
		filePath = schemaPath
	} else {
		basePathToUse := atmosConfig.BasePathAbsolute
		if basePathToUse == "" {
			// Fallback to BasePath if BasePathAbsolute is not set (for backward compatibility).
			basePathToUse = atmosConfig.BasePath
		}
		switch schemaType {
		case "jsonschema":
			{
				filePath = filepath.Join(basePathToUse, atmosConfig.GetResourcePath("jsonschema").BasePath, schemaPath)
			}
		case "opa":
			{
				filePath = filepath.Join(basePathToUse, atmosConfig.GetResourcePath("opa").BasePath, schemaPath)
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

	// Add the process environment variables to the component section.
	componentSection[cfg.ProcessEnvSectionName] = u.EnvironToMap()

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
			// Use BasePathAbsolute for module path resolution to ensure correct paths
			// when base_path is relative (e.g., when using ATMOS_CLI_CONFIG_PATH).
			basePathToUse := atmosConfig.BasePathAbsolute
			if basePathToUse == "" {
				// Fallback to BasePath if BasePathAbsolute is not set (for backward compatibility).
				basePathToUse = atmosConfig.BasePath
			}
			modulePathsAbsolute, err := u.JoinPaths(filepath.Join(basePathToUse, atmosConfig.GetResourcePath("opa").BasePath), modulePaths)
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

// FindValidationSection finds the 'validation' section in the component config.
func FindValidationSection(componentSection map[string]any) (schema.Validation, error) {
	defer perf.Track(nil, "exec.FindValidationSection")()

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
