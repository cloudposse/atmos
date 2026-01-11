// Package config provides configuration management for the Atmos init experiment.
//
// NOTE: This package uses github.com/charmbracelet/huh v0.6.0 instead of v0.7.0
// due to a Terminal.app layout breaking issue in v0.7.0. See:
// https://github.com/charmbracelet/huh/issues/631
//
// The issue causes line duplication when navigating between fields in Mac OS Terminal.app
// with tab key or arrow keys. This was introduced around commit hash 310cd4a379ac
// and affects all versions up through 0.7.0. Version 0.6.0 and earlier work correctly.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/generator/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Color constants for consistent styling using lipgloss named colors.
const (
	ColorWhite  = "White"
	ColorBlack  = "Black"
	ColorBlue   = "Blue"
	ColorRed    = "Red"
	ColorGrey   = "Gray"
	ColorPurple = "Magenta"
)

// ScaffoldConfigFileName is the name of the scaffold configuration file.
const ScaffoldConfigFileName = "scaffold.yaml"

// dirPermissions is the file mode for creating directories.
const dirPermissions = 0o755

// ScaffoldConfigDir is the directory name for user scaffold configuration.
const ScaffoldConfigDir = ".atmos"

// ScaffoldConfig represents the configuration for a scaffold template.
type ScaffoldConfig struct {
	Name        string                     `yaml:"name"`
	Description string                     `yaml:"description"`
	Version     string                     `yaml:"version"`
	TemplateID  string                     `yaml:"template_id"`
	Fields      map[string]FieldDefinition `yaml:"fields"`
	Delimiters  []string                   `yaml:"delimiters"`
}

// FieldDefinition represents a single configurable field in a project scaffold configuration, defining its key, type, label, description, validation rules and default value.
type FieldDefinition struct {
	Key         string      `yaml:"key"`
	Type        string      `yaml:"type"`
	Label       string      `yaml:"label"`
	Description string      `yaml:"description"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default"`
	Options     []string    `yaml:"options"`
	Placeholder string      `yaml:"placeholder"`
}

// Config represents the user's configuration values as a generic map to support dynamic fields from scaffold.yaml.
type Config map[string]interface{}

// LoadScaffoldConfigFromContent loads scaffold configuration from YAML content.
func LoadScaffoldConfigFromContent(content string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadScaffoldConfigFromContent")()

	var scaffoldConfig ScaffoldConfig
	if err := yaml.Unmarshal([]byte(content), &scaffoldConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scaffold config: %w", err)
	}

	return &scaffoldConfig, nil
}

// LoadScaffoldConfigFromFile loads scaffold configuration schema from the specified YAML file using Viper, returning a pointer to ScaffoldConfig and an error on read or unmarshal failures.
func LoadScaffoldConfigFromFile(configPath string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadScaffoldConfigFromFile")()

	// Create a new Viper instance for this specific config file
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Try to read the config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read scaffold config: %w", err)
	}

	// Load as scaffold configuration schema
	var scaffoldConfig ScaffoldConfig
	if err := v.Unmarshal(&scaffoldConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scaffold config: %w", err)
	}

	return &scaffoldConfig, nil
}

// LoadUserValues loads user values from the .atmos/scaffold.yaml file within the provided scaffoldPath and returns them as a map along with any error encountered; returns an empty map and nil error if the file does not exist; returns a wrapped error on read or unmarshal failures.
func LoadUserValues(scaffoldPath string) (map[string]interface{}, error) {
	defer perf.Track(nil, "config.LoadUserValues")()

	// Create .atmos directory path
	atmosDir := filepath.Join(scaffoldPath, ScaffoldConfigDir)
	valuesPath := filepath.Join(atmosDir, ScaffoldConfigFileName)

	// Create a new Viper instance for this specific config file
	v := viper.New()
	v.SetConfigFile(valuesPath)
	v.SetConfigType("yaml")

	// Try to read the config file
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || os.IsNotExist(err) {
			// File doesn't exist, return empty map
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("failed to read user values: %w", err)
	}

	// Load as new format (with template_id and values)
	var userConfig UserConfig
	if err := v.Unmarshal(&userConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user config: %w", err)
	}

	return userConfig.Values, nil
}

// SaveUserValues saves the provided values to the .atmos/scaffold.yaml file within scaffoldPath using the new format structure and returns an error on failure.
func SaveUserValues(scaffoldPath string, values map[string]interface{}) error {
	defer perf.Track(nil, "config.SaveUserValues")()

	// Always save with new format structure, even if template_id is empty
	return SaveUserConfig(scaffoldPath, "", values)
}

// SaveUserConfig saves user configuration with the provided templateID and values to scaffoldPath and returns an error on failure.
func SaveUserConfig(scaffoldPath string, templateID string, values map[string]interface{}) error {
	defer perf.Track(nil, "config.SaveUserConfig")()

	return SaveUserConfigWithBaseRef(scaffoldPath, templateID, "", values)
}

// SaveUserConfigWithBaseRef saves user configuration with the provided templateID, baseRef, and values to scaffoldPath and returns an error on failure.
func SaveUserConfigWithBaseRef(scaffoldPath string, templateID string, baseRef string, values map[string]interface{}) error {
	defer perf.Track(nil, "config.SaveUserConfigWithBaseRef")()

	// Create .atmos directory path
	atmosDir := filepath.Join(scaffoldPath, ScaffoldConfigDir)
	valuesPath := filepath.Join(atmosDir, ScaffoldConfigFileName)

	// Ensure the .atmos directory exists
	if err := os.MkdirAll(atmosDir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create .atmos directory: %w", err)
	}

	// Create a new Viper instance for this specific config file
	v := viper.New()
	v.SetConfigFile(valuesPath)
	v.SetConfigType("yaml")

	// Set the values in Viper
	v.Set("template_id", templateID)
	if baseRef != "" {
		v.Set("base_ref", baseRef)
	}
	v.Set("values", values)

	// Write the config file
	// Check if the config file exists and use appropriate write method
	if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
		// File doesn't exist, use WriteConfigAs to create it
		if err := v.WriteConfigAs(valuesPath); err != nil {
			return fmt.Errorf("failed to write user config: %w", err)
		}
	} else {
		// File exists, use WriteConfig to update it
		if err := v.WriteConfig(); err != nil {
			return fmt.Errorf("failed to write user config: %w", err)
		}
	}

	return nil
}

// LoadUserConfig loads user configuration from .atmos/scaffold.yaml within the specified scaffoldPath, returning a pointer to UserConfig and an error; returns nil, nil if the config file does not exist.
func LoadUserConfig(scaffoldPath string) (*UserConfig, error) {
	defer perf.Track(nil, "config.LoadUserConfig")()

	atmosDir := filepath.Join(scaffoldPath, ScaffoldConfigDir)
	valuesPath := filepath.Join(atmosDir, ScaffoldConfigFileName)

	// Check if file exists
	if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
		return nil, nil // No config file yet - this is OK
	}

	// Read the file
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user config: %w", err)
	}

	// Parse YAML
	var config UserConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	return &config, nil
}

// DeepMerge merges scaffold configuration defaults with user values.
func DeepMerge(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) map[string]interface{} {
	defer perf.Track(nil, "config.DeepMerge")()

	merged := make(map[string]interface{})

	// Start with scaffold defaults
	for key := range scaffoldConfig.Fields {
		merged[key] = scaffoldConfig.Fields[key].Default
	}

	// Override with user values
	for key, value := range userValues {
		merged[key] = value
	}

	return merged
}

// GetConfigPath returns the path where the config directory should be stored based on the user's home directory and returns an error if the user home directory cannot be determined.
func GetConfigPath() (string, error) {
	defer perf.Track(nil, "config.GetConfigPath")()

	homeDir, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos"), nil
}

// PromptForScaffoldConfig prompts the user for scaffold configuration values using a dynamic form built from the provided ScaffoldConfig; userValues supplies initial values and is populated with results; returns an error on failure.
func PromptForScaffoldConfig(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) error {
	defer perf.Track(nil, "config.PromptForScaffoldConfig")()

	// Initialize form values with user values and defaults
	formValues := initializeFormValues(scaffoldConfig, userValues)

	// Build the form with grouped fields
	huhForm, valueGetters := buildConfigForm(scaffoldConfig, formValues)

	// Run the form interaction
	if err := runFormInteraction(huhForm); err != nil {
		return err
	}

	// Extract form values back to userValues
	extractFormValues(userValues, valueGetters)

	return nil
}

// initializeFormValues merges default values with user-provided values.
func initializeFormValues(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) map[string]interface{} {
	formValues := make(map[string]interface{})

	// Set defaults from scaffold config
	for key := range scaffoldConfig.Fields {
		if scaffoldConfig.Fields[key].Default != nil {
			formValues[key] = scaffoldConfig.Fields[key].Default
		}
	}

	// Override with user values
	for key, value := range userValues {
		formValues[key] = value
	}

	return formValues
}

// fieldItem represents a field with its key for grouping.
type fieldItem struct {
	key   string
	field FieldDefinition
}

// buildConfigForm builds the configuration form with grouped fields.
// Returns the form and value getters for extracting values after submission.
func buildConfigForm(scaffoldConfig *ScaffoldConfig, formValues map[string]interface{}) (*huh.Form, map[string]func() interface{}) {
	// Should we run in accessible mode?
	// Note: ACCESSIBLE is a standard environment variable used by the huh form library
	// to enable accessible mode for screen readers. Using viper for consistency.
	v := viper.New()
	_ = v.BindEnv("ACCESSIBLE")
	accessible := v.GetBool("ACCESSIBLE")

	// Group fields by type
	basicFields, configFields, advancedFields := groupFieldsByType(scaffoldConfig)

	// Store value getters for after form completion
	valueGetters := make(map[string]func() interface{})

	// Build form groups
	var groups []*huh.Group

	if len(basicFields) > 0 {
		groups = append(groups, createFormGroup(basicFields, formValues, valueGetters))
	}

	if len(configFields) > 0 {
		groups = append(groups, createFormGroup(configFields, formValues, valueGetters))
	}

	if len(advancedFields) > 0 {
		groups = append(groups, createFormGroup(advancedFields, formValues, valueGetters))
	}

	// Create the form
	huhForm := huh.NewForm(groups...).WithAccessible(accessible)

	return huhForm, valueGetters
}

// groupFieldsByType groups fields into basic, config, and advanced categories.
func groupFieldsByType(scaffoldConfig *ScaffoldConfig) ([]fieldItem, []fieldItem, []fieldItem) {
	var basicFields, configFields, advancedFields []fieldItem

	for key := range scaffoldConfig.Fields {
		field := scaffoldConfig.Fields[key]
		item := fieldItem{key: key, field: field}
		switch field.Type {
		case "input", "text":
			basicFields = append(basicFields, item)
		case "select":
			configFields = append(configFields, item)
		case "multiselect", "confirm":
			advancedFields = append(advancedFields, item)
		default:
			basicFields = append(basicFields, item)
		}
	}

	return basicFields, configFields, advancedFields
}

// createFormGroup creates a huh.Group from a list of fields.
func createFormGroup(items []fieldItem, formValues map[string]interface{}, valueGetters map[string]func() interface{}) *huh.Group {
	var groupFields []huh.Field

	for i := range items {
		field, getter := createField(items[i].key, items[i].field, formValues)
		groupFields = append(groupFields, field)
		valueGetters[items[i].key] = getter
	}

	return huh.NewGroup(groupFields...)
}

// runFormInteraction runs the form and handles user interaction.
func runFormInteraction(huhForm *huh.Form) error {
	err := huhForm.Run()
	if err != nil {
		return fmt.Errorf("user aborted the configuration: %w", err)
	}
	return nil
}

// extractFormValues copies form values back to userValues map using value getters.
func extractFormValues(userValues map[string]interface{}, valueGetters map[string]func() interface{}) {
	for key, getter := range valueGetters {
		userValues[key] = getter()
	}
}

// createField creates a huh field based on the field definition.
// It returns the field and a function to get the updated value.
//
//nolint:gocognit,revive,cyclop,funlen // complex TUI field factory handling multiple field types
func createField(key string, field FieldDefinition, values map[string]interface{}) (huh.Field, func() interface{}) {
	// Get current value or default
	currentValue := values[key]
	if currentValue == nil {
		currentValue = field.Default
	}

	switch field.Type {
	case "input", "text", "string":
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}
		input := huh.NewInput().
			Title(field.Label).
			Description(field.Description).
			Placeholder(field.Placeholder).
			Value(&value)

		if field.Required {
			input = input.Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, field.Label)
				}
				return nil
			})
		}

		return input, func() interface{} { return value }

	case "select":
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}

		var options []huh.Option[string]
		for _, option := range field.Options {
			options = append(options, huh.NewOption(option, option))
		}

		selectField := huh.NewSelect[string]().
			Title(field.Label).
			Description(field.Description).
			Options(options...).
			Value(&value)

		if field.Required {
			selectField = selectField.Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, field.Label)
				}
				return nil
			})
		}

		return selectField, func() interface{} { return value }

	case "multiselect":
		var value []string
		if slice, ok := currentValue.([]string); ok {
			value = slice
		} else if interfaceSlice, ok := currentValue.([]interface{}); ok {
			// Convert []interface{} to []string (common when loading from YAML)
			for _, item := range interfaceSlice {
				if str, ok := item.(string); ok {
					value = append(value, str)
				}
			}
		}

		var options []huh.Option[string]
		for _, option := range field.Options {
			options = append(options, huh.NewOption(option, option))
		}

		multiSelect := huh.NewMultiSelect[string]().
			Title(field.Label).
			Description(field.Description).
			Options(options...).
			Value(&value).
			Filterable(true)

		if field.Required {
			multiSelect = multiSelect.Validate(func(s []string) error {
				if len(s) == 0 {
					return fmt.Errorf("%w: at least one %s", errUtils.ErrGeneratorFieldRequired, field.Label)
				}
				return nil
			})
		}

		return multiSelect, func() interface{} { return value }

	case "confirm", "bool", "boolean":
		var value bool
		if b, ok := currentValue.(bool); ok {
			value = b
		}

		confirm := huh.NewConfirm().
			Title(field.Label).
			Description(field.Description).
			Value(&value).
			Affirmative("Yes").
			Negative("No")

		return confirm, func() interface{} { return value }

	default:
		// Panic for unsupported field types with a helpful message
		supportedTypes := []string{"input", "text", "string", "select", "multiselect", "confirm", "bool", "boolean"}
		panic(fmt.Sprintf("unsupported field type '%s' for field '%s'. Supported types: %s",
			field.Type, key, strings.Join(supportedTypes, ", ")))
	}
}

// GetConfigurationSummary returns table rows and header representing scaffold configuration, merged values, and their sources.
func GetConfigurationSummary(scaffoldConfig *ScaffoldConfig, mergedValues map[string]interface{}, valueSources map[string]string) ([][]string, []string) {
	defer perf.Track(nil, "config.GetConfigurationSummary")()

	// Prepare table rows
	var rows [][]string
	for key := range scaffoldConfig.Fields {
		value, exists := mergedValues[key]
		if !exists {
			continue
		}

		var valueStr string
		switch v := value.(type) {
		case []string:
			valueStr = strings.Join(v, ", ")
		case bool:
			valueStr = fmt.Sprintf("%t", v)
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		source := valueSources[key]
		if source == "" {
			source = "default"
		}

		rows = append(rows, []string{
			key,
			valueStr,
			source,
		})
	}

	header := []string{"Setting", "Value", "Source"}
	return rows, header
}

// ReadScaffoldConfig reads scaffold configuration from atmos.yaml at the provided targetPath; returns an empty map and nil error when the file does not exist; returns a wrapped error when reading or parsing fails.
func ReadScaffoldConfig(targetPath string) (map[string]interface{}, error) {
	defer perf.Track(nil, "config.ReadScaffoldConfig")()

	configPath := filepath.Join(targetPath, "atmos.yaml")

	// Check if atmos.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return empty config if file doesn't exist
		return make(map[string]interface{}), nil
	}

	// Read the configuration file
	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	// Get all settings as a map
	config := v.AllSettings()
	return config, nil
}

// ReadAtmosScaffoldSection reads only the scaffold section from atmos.yaml
//
// NOTE: This is a temporary shim for the init experiment. In the full atmos CLI,
// this functionality will be integrated into the main atmos configuration handling
// system which has robust support for reading and validating atmos.yaml files.
func ReadAtmosScaffoldSection(targetPath string) (map[string]interface{}, error) {
	defer perf.Track(nil, "config.ReadAtmosScaffoldSection")()

	configPath := filepath.Join(targetPath, "atmos.yaml")

	// Check if atmos.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return empty config if file doesn't exist
		return make(map[string]interface{}), nil
	}

	// Read the configuration file
	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	// Get only the scaffold section
	scaffoldSection := v.Get("scaffold")
	if scaffoldSection == nil {
		// Return empty map if no scaffold section
		return make(map[string]interface{}), nil
	}

	scaffoldMap, ok := scaffoldSection.(map[string]interface{})
	if !ok {
		return nil, errUtils.Build(errUtils.ErrInvalidScaffoldSection).
			WithExplanation("Scaffold section is not a valid configuration").
			Err()
	}

	return scaffoldMap, nil
}

// HasScaffoldConfig checks if a configuration contains a scaffold.yaml file.
func HasScaffoldConfig(files []types.File) bool {
	defer perf.Track(nil, "config.HasScaffoldConfig")()

	for _, file := range files {
		if file.Path == ScaffoldConfigFileName {
			return true
		}
	}
	return false
}

// HasUserConfig checks if a scaffold template directory at the specified scaffoldPath contains a user configuration file, returning true if the file exists.
func HasUserConfig(scaffoldPath string) bool {
	defer perf.Track(nil, "config.HasUserConfig")()

	userConfigPath := filepath.Join(scaffoldPath, ScaffoldConfigDir, ScaffoldConfigFileName)
	_, err := os.Stat(userConfigPath)
	return err == nil
}

// UserConfig represents the user's configuration with template metadata and values.
type UserConfig struct {
	TemplateID string                 `yaml:"template_id"`
	BaseRef    string                 `yaml:"base_ref,omitempty"`
	Values     map[string]interface{} `yaml:"values"`
}
