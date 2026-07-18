package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// getBasePathToUse returns the appropriate base path for file resolution.
// It prefers BasePathAbsolute for correct resolution when base_path is relative
// (e.g., when using ATMOS_CLI_CONFIG_PATH), falling back to BasePath for backward compatibility.
func getBasePathToUse(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig.BasePathAbsolute != "" {
		return atmosConfig.BasePathAbsolute
	}
	return atmosConfig.BasePath
}

// enableProvenanceForRichOutput sets atmosConfig.TrackProvenance when the
// resolved output format is "rich". Provenance is enabled only for the rich
// invocation: it lets the command map JSON Schema fields back to the
// effective stack value without changing normal component-validation cost
// or behavior for other formats.
func enableProvenanceForRichOutput(atmosConfig *schema.AtmosConfiguration, outputFormat string) {
	if strings.EqualFold(strings.TrimSpace(outputFormat), "rich") {
		atmosConfig.TrackProvenance = true
	}
}

// validateComponentFlags holds the `validate component` flags
// ExecuteValidateComponentCmd needs to run validation.
type validateComponentFlags struct {
	stack       string
	schemaPath  string
	schemaType  string
	modulePaths []string
	timeout     int
}

// readValidateComponentFlags reads the flags ExecuteValidateComponentCmd needs
// from the command's flag set.
func readValidateComponentFlags(flags *pflag.FlagSet) (validateComponentFlags, error) {
	var f validateComponentFlags
	var err error
	if f.stack, err = flags.GetString("stack"); err != nil {
		return f, err
	}
	if f.schemaPath, err = flags.GetString("schema-path"); err != nil {
		return f, err
	}
	if f.schemaType, err = flags.GetString("schema-type"); err != nil {
		return f, err
	}
	if f.modulePaths, err = flags.GetStringSlice("module-paths"); err != nil {
		return f, err
	}
	if f.timeout, err = flags.GetInt("timeout"); err != nil {
		return f, err
	}
	return f, nil
}

// ExecuteValidateComponentCmd executes `validate component` command. The
// outputFormat argument is the already-resolved config -> env var -> CLI
// flag precedence value that the caller computes via its normal format
// selection, used to enable provenance tracking for "rich" output
// regardless of which source supplied it.
func ExecuteValidateComponentCmd(cmd *cobra.Command, args []string, outputFormat string) (string, string, error) {
	defer perf.Track(nil, "exec.ExecuteValidateComponentCmd")()

	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return "", "", err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return "", "", err
	}
	enableProvenanceForRichOutput(&atmosConfig, outputFormat)

	if len(args) != 1 {
		return "", "", errUtils.ErrInvalidComponentArgument
	}

	componentName := args[0]

	// Initialize spinner.
	s := spinner.New(fmt.Sprintf("Validating Atmos Component: %s", componentName))
	s.Start()

	flags, err := readValidateComponentFlags(cmd.Flags())
	if err != nil {
		s.Stop()
		return "", "", err
	}

	_, err = ExecuteValidateComponent(&atmosConfig, info, componentName, flags.stack, flags.schemaPath, flags.schemaType, flags.modulePaths, flags.timeout)
	if err != nil {
		s.Error("Component validation failed")
		return "", "", err
	}
	s.Success("Component validated successfully")
	log.Debug("Component validation completed", "component", componentName, "stack", flags.stack)

	return componentName, flags.stack, nil
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
	configAndStacksInfo, err := ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil, nil)
	if err != nil {
		configAndStacksInfo.ComponentType = cfg.HelmfileComponentType
		configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil, nil)
		if err != nil {
			configAndStacksInfo.ComponentType = cfg.PackerComponentType
			configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil, nil)
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
	var filePath string
	if u.FileExists(schemaPath) {
		filePath = schemaPath
	} else {
		basePathToUse := getBasePathToUse(atmosConfig)
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
	componentSection[cfg.ProcessEnvSectionName] = env.EnvironToMap()

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
			basePathToUse := getBasePathToUse(atmosConfig)
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
