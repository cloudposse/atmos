package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/generator/types"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
)

// LoadScaffoldConfigFromContent loads and validates an AtmosScaffoldConfig manifest from YAML content.
func LoadScaffoldConfigFromContent(content string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadScaffoldConfigFromContent")()

	scaffoldConfig, err := manifest.Load[ScaffoldSpec](ScaffoldKind, []byte(content))
	if err != nil {
		return nil, err
	}
	if err := validateFieldDefinitions(scaffoldConfig); err != nil {
		return nil, err
	}
	return scaffoldConfig, nil
}

// LoadScaffoldConfigFromFile loads and validates an AtmosScaffoldConfig manifest from the specified YAML file.
func LoadScaffoldConfigFromFile(configPath string) (*ScaffoldConfig, error) {
	defer perf.Track(nil, "config.LoadScaffoldConfigFromFile")()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read scaffold config: %w", err)
	}
	return LoadScaffoldConfigFromContent(string(data))
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

	// Reject nil configs and configs without a name: LoadProjectRecord will
	// fail to reload a record written without metadata.name, leaving the project
	// in a permanently broken state.
	if templateConfig == nil || templateConfig.Metadata.Name == "" {
		return errUtils.ErrTemplateConfigNameRequired
	}

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

// GetConfigPath returns the path where the config directory should be stored based on the user's home directory and returns an error if the user home directory cannot be determined.
func GetConfigPath() (string, error) {
	defer perf.Track(nil, "config.GetConfigPath")()

	homeDir, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos"), nil
}

// ReadScaffoldConfig reads scaffold configuration from atmos.yaml at the provided targetPath; returns an empty map and nil error when the file does not exist; returns a wrapped error when reading or parsing fails.
//
// Use yaml.v3 directly instead of Viper to preserve the original key casing.
// Viper's AllSettings() lowercases all keys, which mangles mixed-case fields such
// as projectName → projectname.
func ReadScaffoldConfig(targetPath string) (map[string]interface{}, error) {
	defer perf.Track(nil, "config.ReadScaffoldConfig")()

	configPath := filepath.Join(targetPath, "atmos.yaml")

	// Return empty config if file doesn't exist.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse atmos.yaml: %w", err)
	}
	if config == nil {
		return make(map[string]interface{}), nil
	}
	return config, nil
}

// ReadAtmosScaffoldSection reads only the scaffold section from atmos.yaml.
//
// NOTE: This is a temporary shim for the init experiment. In the full atmos CLI,
// this functionality will be integrated into the main atmos configuration handling
// system which has robust support for reading and validating atmos.yaml files.
//
// Use yaml.v3 directly instead of Viper to preserve the original key casing.
// Viper's AllSettings() lowercases all keys, which mangles mixed-case fields such
// as projectName → projectname.
func ReadAtmosScaffoldSection(targetPath string) (map[string]interface{}, error) {
	defer perf.Track(nil, "config.ReadAtmosScaffoldSection")()

	configPath := filepath.Join(targetPath, "atmos.yaml")

	// Return empty config if file doesn't exist.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &fullConfig); err != nil {
		return nil, fmt.Errorf("failed to parse atmos.yaml: %w", err)
	}
	if fullConfig == nil {
		return make(map[string]interface{}), nil
	}

	// Extract only the scaffold section.
	scaffoldSection, exists := fullConfig["scaffold"]
	if !exists || scaffoldSection == nil {
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
