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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/experiments/init/internal/types"
)

// Color constants for consistent styling using lipgloss named colors
const (
	ColorWhite  = "White"
	ColorBlack  = "Black"
	ColorBlue   = "Blue"
	ColorRed    = "Red"
	ColorGrey   = "Gray"
	ColorPurple = "Magenta"
)

// ScaffoldConfigFileName is the name of the scaffold configuration file
const ScaffoldConfigFileName = "scaffold.yaml"

// ScaffoldConfigDir is the directory name for user scaffold configuration
const ScaffoldConfigDir = ".atmos"

// ScaffoldConfig represents the configuration for a scaffold template
type ScaffoldConfig struct {
	Name        string                     `yaml:"name"`
	Description string                     `yaml:"description"`
	Version     string                     `yaml:"version"`
	TemplateID  string                     `yaml:"template_id"`
	Fields      map[string]FieldDefinition `yaml:"fields"`
}

// FieldDefinition represents a single field in the configuration
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

// Config represents the user's configuration values
type Config struct {
	Author           string   `yaml:"author"`
	Year             string   `yaml:"year"`
	License          string   `yaml:"license"`
	CloudProvider    string   `yaml:"cloud_provider"`
	AWSRegions       []string `yaml:"aws_regions"`
	Environment      string   `yaml:"environment"`
	TerraformVersion string   `yaml:"terraform_version"`
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	EnableMonitoring bool     `yaml:"enable_monitoring"`
}

// ProjectType represents the type of project
type ProjectType string

const (
	BasicProject    ProjectType = "basic"
	AdvancedProject ProjectType = "advanced"
)

func (p ProjectType) String() string {
	switch p {
	case BasicProject:
		return "Basic Project"
	case AdvancedProject:
		return "Advanced Project"
	default:
		return string(p)
	}
}

// DynamicForm represents the dynamic form structure (following working pattern)
type DynamicForm struct {
	Type   ProjectType
	Values map[string]interface{}
}

// Manager handles configuration loading, saving, and prompting
type Manager struct {
	viper *viper.Viper
	path  string
}

// NewManager creates a new configuration manager
func NewManager(configPath string) *Manager {
	v := viper.New()
	v.SetConfigName(".config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)

	return &Manager{
		viper: v,
		path:  configPath,
	}
}

// LoadScaffoldConfigFromContent loads scaffold configuration from YAML content
func LoadScaffoldConfigFromContent(content string) (*ScaffoldConfig, error) {
	var scaffoldConfig ScaffoldConfig
	if err := yaml.Unmarshal([]byte(content), &scaffoldConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scaffold config: %w", err)
	}

	return &scaffoldConfig, nil
}

// LoadScaffoldConfigFromFile loads scaffold configuration schema from a file
func LoadScaffoldConfigFromFile(configPath string) (*ScaffoldConfig, error) {
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

// LoadUserValues loads user values from a scaffold template directory
func LoadUserValues(scaffoldPath string) (map[string]interface{}, error) {
	// Create .atmos directory path
	atmosDir := filepath.Join(scaffoldPath, ScaffoldConfigDir)
	valuesPath := filepath.Join(atmosDir, ScaffoldConfigFileName)

	// Create a new Viper instance for this specific config file
	v := viper.New()
	v.SetConfigFile(valuesPath)
	v.SetConfigType("yaml")

	// Try to read the config file
	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
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

// SaveUserValues saves user values to a scaffold template directory
func SaveUserValues(scaffoldPath string, values map[string]interface{}) error {
	// Always save with new format structure, even if template_id is empty
	return SaveUserConfig(scaffoldPath, "", values)
}

// SaveUserConfig saves user configuration with template ID and values
func SaveUserConfig(scaffoldPath string, templateID string, values map[string]interface{}) error {
	// Create .atmos directory path
	atmosDir := filepath.Join(scaffoldPath, ScaffoldConfigDir)
	valuesPath := filepath.Join(atmosDir, ScaffoldConfigFileName)

	// Ensure the .atmos directory exists
	if err := os.MkdirAll(atmosDir, 0755); err != nil {
		return fmt.Errorf("failed to create .atmos directory: %w", err)
	}

	// Create a new Viper instance for this specific config file
	v := viper.New()
	v.SetConfigFile(valuesPath)
	v.SetConfigType("yaml")

	// Set the values in Viper
	v.Set("template_id", templateID)
	v.Set("values", values)

	// Write the config file
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write user config: %w", err)
	}

	return nil
}

// DeepMerge merges scaffold configuration defaults with user values
func DeepMerge(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Start with scaffold defaults
	for key, field := range scaffoldConfig.Fields {
		merged[key] = field.Default
	}

	// Override with user values
	for key, value := range userValues {
		merged[key] = value
	}

	return merged
}

// GetConfigPath returns the path where the config file should be stored
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos"), nil
}

// Load loads the configuration from file
func (m *Manager) Load() (*Config, error) {
	config := &Config{}

	// Set defaults
	m.viper.SetDefault("author", "")
	m.viper.SetDefault("year", "")
	m.viper.SetDefault("license", "Apache Software License 2.0")
	m.viper.SetDefault("cloud_provider", "aws")
	m.viper.SetDefault("aws_regions", []string{"us-east-1"})
	m.viper.SetDefault("environment", "dev")
	m.viper.SetDefault("terraform_version", "1.5.0")
	m.viper.SetDefault("name", "")
	m.viper.SetDefault("description", "")
	m.viper.SetDefault("enable_monitoring", false)

	// Read config file
	if err := m.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file doesn't exist, that's okay - we'll use defaults
	}

	// Unmarshal into struct
	if err := m.viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

// Save saves the configuration to file
func (m *Manager) Save(config *Config) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(m.path, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	configFile := filepath.Join(m.path, ".config.yaml")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// PromptUser prompts the user for configuration values using a form
func (m *Manager) PromptUser(config *Config) error {
	// This should use the same dynamic approach as PromptForProjectConfig
	// For now, just set some basic defaults
	config.Author = "Your Name"
	config.Year = "2024"
	config.License = "Apache Software License 2.0"
	config.CloudProvider = "aws"
	config.AWSRegions = []string{"us-east-1"}
	config.Environment = "dev"
	config.TerraformVersion = "1.5.0"
	config.Name = "my-atmos-project"
	config.Description = "An Atmos scaffold template for managing infrastructure as code"
	config.EnableMonitoring = false

	return nil
}

// PromptForScaffoldConfig prompts the user for scaffold configuration values using a form
// This creates a fully dynamic form based on the scaffold.yaml structure
func PromptForScaffoldConfig(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) error {
	// Initialize form values with user values and defaults
	formValues := make(map[string]interface{})

	// Set defaults from scaffold config
	for key, field := range scaffoldConfig.Fields {
		if field.Default != nil {
			formValues[key] = field.Default
		}
	}

	// Override with user values
	for key, value := range userValues {
		formValues[key] = value
	}

	// Should we run in accessible mode?
	accessible, _ := strconv.ParseBool(os.Getenv("ACCESSIBLE"))

	// Build form groups dynamically
	var groups []*huh.Group

	// Group fields by type for better UX
	basicFields := []struct {
		key   string
		field FieldDefinition
	}{}
	configFields := []struct {
		key   string
		field FieldDefinition
	}{}
	advancedFields := []struct {
		key   string
		field FieldDefinition
	}{}

	for key, field := range scaffoldConfig.Fields {
		switch field.Type {
		case "input", "text":
			basicFields = append(basicFields, struct {
				key   string
				field FieldDefinition
			}{key, field})
		case "select":
			configFields = append(configFields, struct {
				key   string
				field FieldDefinition
			}{key, field})
		case "multiselect", "confirm":
			advancedFields = append(advancedFields, struct {
				key   string
				field FieldDefinition
			}{key, field})
		default:
			basicFields = append(basicFields, struct {
				key   string
				field FieldDefinition
			}{key, field})
		}
	}

	// Store value getters for after form completion
	valueGetters := make(map[string]func() interface{})

	// Add basic fields group
	if len(basicFields) > 0 {
		var basicGroupFields []huh.Field
		for _, item := range basicFields {
			field, getter := createField(item.key, item.field, formValues)
			basicGroupFields = append(basicGroupFields, field)
			valueGetters[item.key] = getter
		}
		groups = append(groups, huh.NewGroup(basicGroupFields...))
	}

	// Add config fields group
	if len(configFields) > 0 {
		var configGroupFields []huh.Field
		for _, item := range configFields {
			field, getter := createField(item.key, item.field, formValues)
			configGroupFields = append(configGroupFields, field)
			valueGetters[item.key] = getter
		}
		groups = append(groups, huh.NewGroup(configGroupFields...))
	}

	// Add advanced fields group
	if len(advancedFields) > 0 {
		var advancedGroupFields []huh.Field
		for _, item := range advancedFields {
			field, getter := createField(item.key, item.field, formValues)
			advancedGroupFields = append(advancedGroupFields, field)
			valueGetters[item.key] = getter
		}
		groups = append(groups, huh.NewGroup(advancedGroupFields...))
	}

	// Create the form
	huhForm := huh.NewForm(groups...).WithAccessible(accessible)

	err := huhForm.Run()
	if err != nil {
		fmt.Println("Uh oh:", err)
		return fmt.Errorf("user aborted the configuration")
	}

	// Copy form values back to userValues map using the value getters
	for key, getter := range valueGetters {
		userValues[key] = getter()
	}

	return nil
}

// createField creates a huh field based on the field definition
// It returns the field and a function to get the updated value
func createField(key string, field FieldDefinition, values map[string]interface{}) (huh.Field, func() interface{}) {
	// Get current value or default
	currentValue := values[key]
	if currentValue == nil {
		currentValue = field.Default
	}

	switch field.Type {
	case "input", "text":
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
					return fmt.Errorf("%s is required", field.Label)
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
					return fmt.Errorf("%s is required", field.Label)
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
					return fmt.Errorf("at least one %s is required", field.Label)
				}
				return nil
			})
		}

		return multiSelect, func() interface{} { return value }

	case "confirm":
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
		// Default to input
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}
		input := huh.NewInput().
			Title(field.Label).
			Description(field.Description).
			Placeholder(field.Placeholder).
			Value(&value)

		return input, func() interface{} { return value }
	}
}

// GetConfigurationSummary returns the configuration values as table data
func GetConfigurationSummary(scaffoldConfig *ScaffoldConfig, mergedValues map[string]interface{}, valueSources map[string]string) ([][]string, []string) {
	// Prepare table rows
	var rows [][]string
	for key := range scaffoldConfig.Fields {
		if value, exists := mergedValues[key]; exists {
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
	}

	header := []string{"Setting", "Value", "Source"}
	return rows, header
}

// ReadScaffoldConfig reads the scaffold configuration from atmos.yaml
func ReadScaffoldConfig(targetPath string) (map[string]interface{}, error) {
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
		return nil, fmt.Errorf("scaffold section is not a valid configuration")
	}

	return scaffoldMap, nil
}

// HasScaffoldConfig checks if a configuration contains a scaffold.yaml file
func HasScaffoldConfig(files []types.File) bool {
	for _, file := range files {
		if file.Path == ScaffoldConfigFileName {
			return true
		}
	}
	return false
}

// HasUserConfig checks if a scaffold template directory has user configuration
func HasUserConfig(scaffoldPath string) bool {
	userConfigPath := filepath.Join(scaffoldPath, ScaffoldConfigDir, ScaffoldConfigFileName)
	_, err := os.Stat(userConfigPath)
	return err == nil
}

// LoadUserConfiguration loads user configuration and prompts if needed
func LoadUserConfiguration() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	manager := NewManager(configPath)
	userConfig, err := manager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// If we don't have user configuration, prompt for it
	if userConfig.Author == "" || userConfig.Year == "" {
		fmt.Println("Please provide some configuration details:")
		fmt.Println()

		if err := manager.PromptUser(userConfig); err != nil {
			return nil, fmt.Errorf("failed to prompt user: %w", err)
		}

		// Save the configuration for future use
		if err := manager.Save(userConfig); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}

		fmt.Println()
	}

	return userConfig, nil
}

// ValidateTargetDirectory checks if the target directory exists and validates the operation
func ValidateTargetDirectory(targetPath string, force, update bool) error {
	// Check if target directory exists
	if _, err := os.Stat(targetPath); err == nil {
		// Directory exists, check if it has any files that would conflict
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read target directory: %w", err)
		}

		// Filter out hidden files and directories
		var visibleEntries []string
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") {
				visibleEntries = append(visibleEntries, entry.Name())
			}
		}

		if len(visibleEntries) > 0 {
			if !force && !update {
				return fmt.Errorf("target directory '%s' already exists and contains files: %s (use --force to overwrite or --update to merge)",
					targetPath, strings.Join(visibleEntries, ", "))
			}
		}
	}

	return nil
}

// UserConfig represents the user's configuration with template metadata and values
type UserConfig struct {
	TemplateID string                 `yaml:"template_id"`
	Values     map[string]interface{} `yaml:"values"`
}
