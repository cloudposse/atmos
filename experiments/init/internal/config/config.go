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
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

// ProjectConfig represents the configuration for a project
type ProjectConfig struct {
	Name        string                     `yaml:"name"`
	Description string                     `yaml:"description"`
	Version     string                     `yaml:"version"`
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
	Author             string   `yaml:"author"`
	Year               string   `yaml:"year"`
	License            string   `yaml:"license"`
	CloudProvider      string   `yaml:"cloud_provider"`
	AWSRegions         []string `yaml:"aws_regions"`
	Environment        string   `yaml:"environment"`
	TerraformVersion   string   `yaml:"terraform_version"`
	ProjectName        string   `yaml:"project_name"`
	ProjectDescription string   `yaml:"project_description"`
	EnableMonitoring   bool     `yaml:"enable_monitoring"`
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

// LoadProjectConfigFromContent loads project configuration from YAML content
func LoadProjectConfigFromContent(content string) (*ProjectConfig, error) {
	var projectConfig ProjectConfig
	if err := yaml.Unmarshal([]byte(content), &projectConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal project config: %w", err)
	}

	return &projectConfig, nil
}

// LoadUserValues loads user values from a project directory
func LoadUserValues(projectPath string) (map[string]interface{}, error) {
	// Create .atmos directory path
	atmosDir := filepath.Join(projectPath, ".atmos")
	valuesPath := filepath.Join(atmosDir, "config.yaml")

	if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
		// File doesn't exist, return empty map
		return make(map[string]interface{}), nil
	}

	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user values: %w", err)
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user values: %w", err)
	}

	return values, nil
}

// SaveUserValues saves user values to a project directory
func SaveUserValues(projectPath string, values map[string]interface{}) error {
	// Create .atmos directory path
	atmosDir := filepath.Join(projectPath, ".atmos")
	valuesPath := filepath.Join(atmosDir, "config.yaml")

	// Ensure the .atmos directory exists
	if err := os.MkdirAll(atmosDir, 0755); err != nil {
		return fmt.Errorf("failed to create .atmos directory: %w", err)
	}

	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("failed to marshal user values: %w", err)
	}

	if err := os.WriteFile(valuesPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write user values: %w", err)
	}

	return nil
}

// DeepMerge merges project configuration defaults with user values
func DeepMerge(projectConfig *ProjectConfig, userValues map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Start with project defaults
	for key, field := range projectConfig.Fields {
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
	m.viper.SetDefault("project_name", "")
	m.viper.SetDefault("project_description", "")
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
	config.ProjectName = "my-atmos-project"
	config.ProjectDescription = "An Atmos project for managing infrastructure as code"
	config.EnableMonitoring = false

	return nil
}

// PromptForProjectConfig prompts the user for project configuration values using a form
// This creates a fully dynamic form based on the project-config.yaml structure
func PromptForProjectConfig(projectConfig *ProjectConfig, userValues map[string]interface{}) error {
	// Initialize form values with user values and defaults
	formValues := make(map[string]interface{})

	// Set defaults from project config
	for key, field := range projectConfig.Fields {
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

	for key, field := range projectConfig.Fields {
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

	// Process the configuration (following working pattern)
	time.Sleep(1 * time.Second)

	// Print configuration summary (following working pattern)
	{
		var sb strings.Builder
		keyword := func(s string) string {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(s)
		}
		fmt.Fprintf(&sb,
			"%s\n\n",
			lipgloss.NewStyle().Bold(true).Render("CONFIGURATION SUMMARY"),
		)

		// Add all configured values to summary using the updated values
		for key, getter := range valueGetters {
			value := getter()
			if value != nil && value != "" {
				switch v := value.(type) {
				case []string:
					if len(v) > 0 {
						fmt.Fprintf(&sb, "%s: %s\n", keyword(key), keyword(strings.Join(v, ", ")))
					}
				case bool:
					fmt.Fprintf(&sb, "%s: %t\n", keyword(key), v)
				default:
					fmt.Fprintf(&sb, "%s: %s\n", keyword(key), keyword(fmt.Sprintf("%v", v)))
				}
			}
		}

		fmt.Println(
			lipgloss.NewStyle().
				Width(50).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(1, 2).
				Render(sb.String()),
		)
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
		// Reject unknown field types instead of defaulting to input
		panic(fmt.Sprintf("unsupported field type '%s' for field '%s'. Supported types: input, text, string, select, multiselect, confirm, bool, boolean", field.Type, key))
	}
}
