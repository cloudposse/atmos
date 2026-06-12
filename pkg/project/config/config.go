// Package config provides scaffold configuration management: the
// AtmosScaffoldConfig manifest kind, the interactive setup form, and the
// project record written to generated projects.
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
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/generator/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/manifest"
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

// ScaffoldKind is the manifest kind for scaffold templates and project records.
const ScaffoldKind = "AtmosScaffoldConfig"

// ScaffoldConfigFileName is the name of the scaffold configuration file.
const ScaffoldConfigFileName = "scaffold.yaml"

// dirPermissions is the file mode for creating directories.
const dirPermissions = 0o755

// filePermissions is the file mode for writing the project record.
const filePermissions = 0o644

// ScaffoldConfigDir is the directory name for user scaffold configuration.
const ScaffoldConfigDir = ".atmos"

// SourceEmbedded marks a project record generated from a template embedded in the Atmos binary.
const SourceEmbedded = "embedded"

//go:generate go run gen_schema.go

func init() {
	manifest.MustRegister(ScaffoldKind, manifest.DefaultAPIVersion, &ScaffoldSpec{})
}

// ScaffoldConfig is the AtmosScaffoldConfig manifest. The same document kind
// serves two roles:
//   - In a template: metadata plus spec.fields describe the questionnaire.
//   - In a generated project (.atmos/scaffold.yaml): the same document with
//     spec.values (the user's answers) and provenance (spec.source,
//     spec.baseRef) merged in, making the project self-describing for
//     future updates.
type ScaffoldConfig = manifest.Manifest[ScaffoldSpec]

// ScaffoldSpec is the spec of an AtmosScaffoldConfig manifest.
type ScaffoldSpec struct {
	// Source records where the template came from (e.g. "embedded", a local
	// path, or a git URL once remote sources are supported). Written to
	// project records; ignored in template manifests.
	Source string `yaml:"source,omitempty" json:"source,omitempty" jsonschema:"description=Where the template came from (written to project records)"`
	// BaseRef records the git ref used as the three-way merge base when the
	// project was generated. Written to project records.
	BaseRef string `yaml:"baseRef,omitempty" json:"baseRef,omitempty" jsonschema:"description=Git ref used as the three-way merge base"`
	// Delimiters optionally overrides the Go template delimiters used when
	// rendering template files (exactly two entries: left and right).
	Delimiters []string `yaml:"delimiters,omitempty" json:"delimiters,omitempty" jsonschema:"description=Template delimiters as a two-element list,minItems=2,maxItems=2"`
	// Fields defines the questionnaire shown when generating from this
	// template. Order is preserved: fields prompt in the order listed.
	Fields []FieldDefinition `yaml:"fields,omitempty" json:"fields,omitempty" jsonschema:"description=Ordered questionnaire fields"`
	// Values holds the user's answers. Written to project records; may also
	// provide preset values in template manifests.
	Values map[string]any `yaml:"values,omitempty" json:"values,omitempty" jsonschema:"description=Field values (answers) keyed by field name"`
}

// FieldDefinition defines a single questionnaire field: its name, prompt
// type, presentation, validation, and default value.
type FieldDefinition struct {
	Name        string   `yaml:"name" json:"name" jsonschema:"description=Field name used as the template variable"`
	Type        string   `yaml:"type,omitempty" json:"type,omitempty" jsonschema:"description=Prompt type,enum=input,enum=text,enum=string,enum=select,enum=multiselect,enum=confirm,enum=bool,enum=boolean"`
	Label       string   `yaml:"label,omitempty" json:"label,omitempty" jsonschema:"description=Short prompt label"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty" jsonschema:"description=Longer help text shown with the prompt"`
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty" jsonschema:"description=Whether a value must be provided"`
	Default     any      `yaml:"default,omitempty" json:"default,omitempty" jsonschema:"description=Default value"`
	Options     []string `yaml:"options,omitempty" json:"options,omitempty" jsonschema:"description=Choices for select and multiselect fields"`
	Placeholder string   `yaml:"placeholder,omitempty" json:"placeholder,omitempty" jsonschema:"description=Placeholder text for input fields"`
}

// Config represents the user's configuration values as a generic map to support dynamic fields from scaffold.yaml.
type Config map[string]interface{}

// LoadScaffoldConfigFromContent loads and validates an AtmosScaffoldConfig manifest from YAML content.
func LoadScaffoldConfigFromContent(content string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadScaffoldConfigFromContent")()

	return manifest.Load[ScaffoldSpec](ScaffoldKind, []byte(content))
}

// LoadScaffoldConfigFromFile loads and validates an AtmosScaffoldConfig manifest from the specified YAML file.
func LoadScaffoldConfigFromFile(configPath string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadScaffoldConfigFromFile")()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read scaffold config: %w", err)
	}
	return manifest.Load[ScaffoldSpec](ScaffoldKind, data)
}

// projectRecordPath returns the path of the project record within targetPath.
func projectRecordPath(targetPath string) string {
	return filepath.Join(targetPath, ScaffoldConfigDir, ScaffoldConfigFileName)
}

// LoadProjectRecord loads the AtmosScaffoldConfig project record from
// .atmos/scaffold.yaml within targetPath. Returns nil without error if no
// record exists.
func LoadProjectRecord(targetPath string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadProjectRecord")()

	recordPath := projectRecordPath(targetPath)
	data, err := os.ReadFile(recordPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No record yet - this is OK.
		}
		return nil, fmt.Errorf("failed to read project record: %w", err)
	}

	record, err := manifest.Load[ScaffoldSpec](ScaffoldKind, data)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestValidation).
			WithCause(err).
			WithExplanationf("The project record `%s` is not a valid `%s` manifest", recordPath, ScaffoldKind).
			WithHint("The record is written automatically on generation; restore it from version control if it was edited by hand").
			WithContext("path", recordPath).
			Err()
	}
	return record, nil
}

// LoadUserValues loads previously saved answers from the project record in
// targetPath. Returns an empty map if no record exists.
func LoadUserValues(targetPath string) (map[string]interface{}, error) {
	defer perf.Track(nil, "config.LoadUserValues")()

	record, err := LoadProjectRecord(targetPath)
	if err != nil {
		return nil, err
	}
	if record == nil || record.Spec.Values == nil {
		return make(map[string]interface{}), nil
	}
	return record.Spec.Values, nil
}

// SaveProjectRecord writes the AtmosScaffoldConfig project record to
// .atmos/scaffold.yaml within targetPath. The record is the template's own
// manifest with the user's answers and provenance merged in:
//   - metadata identifies the template (name, version) at generation time
//   - spec.fields snapshots the questionnaire so the project is self-describing
//   - spec.values holds the answers
//   - spec.source and spec.baseRef record provenance for future updates
//
// The record is marshaled directly to YAML (never through viper) so field
// name casing is preserved exactly.
func SaveProjectRecord(targetPath string, templateConfig *ScaffoldConfig, source, baseRef string, values map[string]interface{}) error {
	defer perf.Track(nil, "config.SaveProjectRecord")()

	record := ScaffoldConfig{
		APIVersion: manifest.DefaultAPIVersion,
		Kind:       ScaffoldKind,
	}
	if templateConfig != nil {
		record.Metadata = templateConfig.Metadata
		record.Spec.Fields = templateConfig.Spec.Fields
		record.Spec.Delimiters = templateConfig.Spec.Delimiters
	}
	if source != "" {
		record.Spec.Source = source
	}
	if baseRef != "" {
		record.Spec.BaseRef = baseRef
	}
	record.Spec.Values = values

	atmosDir := filepath.Join(targetPath, ScaffoldConfigDir)
	if err := os.MkdirAll(atmosDir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create .atmos directory: %w", err)
	}

	data, err := yaml.Marshal(&record)
	if err != nil {
		return fmt.Errorf("failed to marshal project record: %w", err)
	}

	if err := os.WriteFile(projectRecordPath(targetPath), data, filePermissions); err != nil {
		return fmt.Errorf("failed to write project record: %w", err)
	}
	return nil
}

// DeepMerge merges scaffold field defaults with user values. Field order is
// irrelevant for the merge itself, but defaults come from the ordered field
// list.
func DeepMerge(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) map[string]interface{} {
	defer perf.Track(nil, "config.DeepMerge")()

	merged := make(map[string]interface{})

	// Start with scaffold defaults.
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		merged[field.Name] = field.Default
	}

	// Preset values declared in the template override field defaults.
	for key, value := range scaffoldConfig.Spec.Values {
		merged[key] = value
	}

	// Override with user values.
	for key, value := range userValues {
		merged[key] = value
	}

	return merged
}

// MissingRequiredValues returns the names of required fields that have no
// usable (non-nil, non-empty-string) value in the provided values map. Used
// to fail fast in non-interactive mode instead of generating a broken
// project.
func MissingRequiredValues(scaffoldConfig *ScaffoldConfig, values map[string]interface{}) []string {
	defer perf.Track(nil, "config.MissingRequiredValues")()

	var missing []string
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if !field.Required {
			continue
		}
		value, exists := values[field.Name]
		if !exists || value == nil {
			missing = append(missing, field.Name)
			continue
		}
		if str, ok := value.(string); ok && str == "" {
			missing = append(missing, field.Name)
		}
	}
	return missing
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
	if huhForm == nil {
		return nil // No fields to prompt for.
	}

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
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Default != nil {
			formValues[field.Name] = field.Default
		}
	}

	// Override with user values
	for key, value := range userValues {
		formValues[key] = value
	}

	return formValues
}

// buildConfigForm builds the configuration form preserving the field order
// declared in the template.
// Returns the form and value getters for extracting values after submission.
// Returns a nil form when the template declares no fields.
func buildConfigForm(scaffoldConfig *ScaffoldConfig, formValues map[string]interface{}) (*huh.Form, map[string]func() interface{}) {
	if len(scaffoldConfig.Spec.Fields) == 0 {
		return nil, nil
	}

	// Should we run in accessible mode?
	// Note: ACCESSIBLE is a standard environment variable used by the huh form library
	// to enable accessible mode for screen readers. Using viper for consistency.
	v := viper.New()
	_ = v.BindEnv("ACCESSIBLE")
	accessible := v.GetBool("ACCESSIBLE")

	// Store value getters for after form completion
	valueGetters := make(map[string]func() interface{})

	var groupFields []huh.Field
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		huhField, getter := createField(field.Name, field, formValues)
		groupFields = append(groupFields, huhField)
		valueGetters[field.Name] = getter
	}

	huhForm := huh.NewForm(huh.NewGroup(groupFields...)).WithAccessible(accessible)

	return huhForm, valueGetters
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
func createField(key string, field *FieldDefinition, values map[string]interface{}) (huh.Field, func() interface{}) {
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
			Title(fieldTitle(field)).
			Description(field.Description).
			Placeholder(field.Placeholder).
			Value(&value)

		if field.Required {
			input = input.Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
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
			Title(fieldTitle(field)).
			Description(field.Description).
			Options(options...).
			Value(&value)

		if field.Required {
			selectField = selectField.Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
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
			Title(fieldTitle(field)).
			Description(field.Description).
			Options(options...).
			Value(&value).
			Filterable(true)

		if field.Required {
			multiSelect = multiSelect.Validate(func(s []string) error {
				if len(s) == 0 {
					return fmt.Errorf("%w: at least one %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
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
			Title(fieldTitle(field)).
			Description(field.Description).
			Value(&value).
			Affirmative("Yes").
			Negative("No")

		return confirm, func() interface{} { return value }

	default:
		// Unknown types are rejected by schema validation before the form is
		// built; fall back to a plain input as defense in depth rather than
		// panicking.
		log.Warn("Unknown scaffold field type, falling back to input", "field", key, "type", field.Type)
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}
		input := huh.NewInput().
			Title(fieldTitle(field)).
			Description(field.Description).
			Value(&value)
		return input, func() interface{} { return value }
	}
}

// fieldTitle returns the display title for a field: its label when set,
// otherwise its name.
func fieldTitle(field *FieldDefinition) string {
	if field.Label != "" {
		return field.Label
	}
	return field.Name
}

// GetConfigurationSummary returns table rows and header representing scaffold
// configuration, merged values, and their sources. Rows follow the declared
// field order.
func GetConfigurationSummary(scaffoldConfig *ScaffoldConfig, mergedValues map[string]interface{}, valueSources map[string]string) ([][]string, []string) {
	defer perf.Track(nil, "config.GetConfigurationSummary")()

	// Prepare table rows
	var rows [][]string
	for i := range scaffoldConfig.Spec.Fields {
		key := scaffoldConfig.Spec.Fields[i].Name
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

// HasUserConfig checks if a generated project at the specified targetPath contains a project record, returning true if the file exists.
func HasUserConfig(targetPath string) bool {
	defer perf.Track(nil, "config.HasUserConfig")()

	_, err := os.Stat(projectRecordPath(targetPath))
	return err == nil
}
