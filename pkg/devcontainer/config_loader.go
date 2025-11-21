package devcontainer

import (
	"fmt"
	"sort"
	"strings"

	"dario.cat/mergo"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	logKeyName = "name"
)

// Settings represents Atmos-specific devcontainer settings.
type Settings struct {
	Runtime string `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
}

// deserializeSpec explicitly maps the Viper lowercase keys to the Config struct.
// This handles the case where Viper lowercases all keys (forwardports instead of forwardPorts).
func deserializeSpec(specMap map[string]any, name string) (*Config, error) {
	defer perf.Track(nil, "devcontainer.deserializeSpec")()

	config := &Config{}

	deserializeStringFields(specMap, config)
	deserializeBuildSection(specMap, config)
	deserializeArrayFields(specMap, config)
	deserializeBooleanFields(specMap, config)
	deserializeForwardPorts(specMap, config)
	deserializePortsAttributes(specMap, config)
	deserializeContainerEnv(specMap, config)

	filterUnsupportedFields(specMap, name)

	return config, nil
}

func deserializeStringFields(specMap map[string]any, config *Config) {
	if v, ok := specMap["name"].(string); ok {
		config.Name = v
	}
	if v, ok := specMap["image"].(string); ok {
		config.Image = v
	}
	if v, ok := specMap["workspacefolder"].(string); ok {
		config.WorkspaceFolder = v
	}
	if v, ok := specMap["workspacemount"].(string); ok {
		config.WorkspaceMount = v
	}
	if v, ok := specMap["remoteuser"].(string); ok {
		config.RemoteUser = v
	}
	if v, ok := specMap["userenvprobe"].(string); ok {
		config.UserEnvProbe = v
	}
}

func deserializeBuildSection(specMap map[string]any, config *Config) {
	buildRaw, ok := specMap["build"].(map[string]any)
	if !ok {
		return
	}

	config.Build = &Build{
		Dockerfile: getString(buildRaw, "dockerfile"),
		Context:    getString(buildRaw, "context"),
		Args:       getStringMap(buildRaw, "args"),
	}
}

func deserializeArrayFields(specMap map[string]any, config *Config) {
	if mounts, ok := specMap["mounts"].([]any); ok {
		config.Mounts = toStringSlice(mounts)
	}
	if runArgs, ok := specMap["runargs"].([]any); ok {
		config.RunArgs = toStringSlice(runArgs)
	}
	if capAdd, ok := specMap["capadd"].([]any); ok {
		config.CapAdd = toStringSlice(capAdd)
	}
	if securityOpt, ok := specMap["securityopt"].([]any); ok {
		config.SecurityOpt = toStringSlice(securityOpt)
	}
}

func deserializeBooleanFields(specMap map[string]any, config *Config) {
	if v, ok := specMap["overridecommand"].(bool); ok {
		config.OverrideCommand = v
	}
	if v, ok := specMap["init"].(bool); ok {
		config.Init = v
	}
	if v, ok := specMap["privileged"].(bool); ok {
		config.Privileged = v
	}
}

func deserializeForwardPorts(specMap map[string]any, config *Config) {
	if ports, ok := specMap["forwardports"].([]any); ok {
		config.ForwardPorts = ports
	}
}

func deserializePortsAttributes(specMap map[string]any, config *Config) {
	portsAttrs, ok := specMap["portsattributes"].(map[string]any)
	if !ok {
		return
	}

	config.PortsAttributes = make(map[string]PortAttributes)
	for port, attrsRaw := range portsAttrs {
		if attrs, ok := attrsRaw.(map[string]any); ok {
			config.PortsAttributes[port] = PortAttributes{
				Label:    getString(attrs, "label"),
				Protocol: getString(attrs, "protocol"),
			}
		}
	}
}

func deserializeContainerEnv(specMap map[string]any, config *Config) {
	containerEnv, ok := specMap["containerenv"].(map[string]any)
	if !ok {
		return
	}

	config.ContainerEnv = make(map[string]string)
	for k, v := range containerEnv {
		if strVal, ok := v.(string); ok {
			config.ContainerEnv[k] = strVal
		}
	}
}

// Helper functions for type conversion.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringMap(m map[string]any, key string) map[string]string {
	result := make(map[string]string)
	if mapRaw, ok := m[key].(map[string]any); ok {
		for k, v := range mapRaw {
			if strVal, ok := v.(string); ok {
				result[k] = strVal
			}
		}
	}
	return result
}

func toStringSlice(slice []any) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

// LoadConfig loads a devcontainer configuration by name from atmos.yaml devcontainer section.
func LoadConfig(
	atmosConfig *schema.AtmosConfiguration,
	name string,
) (*Config, *Settings, error) {
	defer perf.Track(atmosConfig, "devcontainer.LoadConfig")()

	devcontainerMap, err := getDevcontainerMap(atmosConfig, name)
	if err != nil {
		return nil, nil, err
	}

	settings, err := extractSettings(devcontainerMap, name)
	if err != nil {
		return nil, nil, err
	}

	config, err := extractAndValidateSpec(devcontainerMap, name)
	if err != nil {
		return nil, nil, err
	}

	log.Debug("Loaded devcontainer configuration", logKeyName, name, "image", config.Image, "runtime", settings.Runtime)

	return config, settings, nil
}

func getDevcontainerMap(atmosConfig *schema.AtmosConfiguration, name string) (map[string]any, error) {
	log.Debug("LoadConfig called", logKeyName, name, "devcontainer_nil", atmosConfig.Devcontainer == nil)

	if atmosConfig.Devcontainer == nil {
		log.Debug("No devcontainers configured")
		return nil, fmt.Errorf("%w: no devcontainers configured", errUtils.ErrDevcontainerNotFound)
	}

	log.Debug("Devcontainer field populated", "count", len(atmosConfig.Devcontainer))

	rawDevcontainer, exists := atmosConfig.Devcontainer[name]
	if !exists {
		return nil, fmt.Errorf("%w: devcontainer '%s' not found", errUtils.ErrDevcontainerNotFound, name)
	}

	devcontainerMap, ok := rawDevcontainer.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: devcontainer '%s' must be a map", errUtils.ErrInvalidDevcontainerConfig, name)
	}

	return devcontainerMap, nil
}

func extractSettings(devcontainerMap map[string]any, name string) (*Settings, error) {
	var settings Settings

	settingsRaw, hasSettings := devcontainerMap["settings"]
	if !hasSettings {
		return &settings, nil
	}

	data, err := yaml.Marshal(settingsRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal settings for %s: %w", errUtils.ErrInvalidDevcontainerConfig, name, err)
	}

	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal settings for %s: %w", errUtils.ErrInvalidDevcontainerConfig, name, err)
	}

	return &settings, nil
}

func extractAndValidateSpec(devcontainerMap map[string]any, name string) (*Config, error) {
	specRaw, hasSpec := devcontainerMap["spec"]
	if !hasSpec {
		return nil, fmt.Errorf("%w: devcontainer '%s' missing 'spec' section", errUtils.ErrInvalidDevcontainerConfig, name)
	}

	// Handle list-based spec (sequence of maps to merge).
	// When using list syntax, Viper stores the list AND creates indexed keys (spec[0], spec[1], etc.)
	// We need to collect the indexed items and merge them.
	if specList, isList := specRaw.([]any); isList {
		merged, err := collectAndMergeSpecList(specList, devcontainerMap, name)
		if err != nil {
			return nil, err
		}
		specRaw = merged
	} else {
		log.Debug("Spec type", logKeyName, name, "type", fmt.Sprintf("%T", specRaw))
	}

	specMap, ok := specRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: devcontainer spec must be a map (got %T)", errUtils.ErrInvalidDevcontainerConfig, specRaw)
	}

	config, err := deserializeSpec(specMap, name)
	if err != nil {
		return nil, err
	}

	if config.Name == "" {
		config.Name = name
	}

	if _, hasOverride := specMap["overridecommand"]; !hasOverride {
		config.OverrideCommand = true
	}

	if specMap, ok := specRaw.(map[string]any); ok {
		filterUnsupportedFields(specMap, name)
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// collectAndMergeSpecList collects indexed spec items and merges them.
// Returns the merged map or an error if the list is empty or invalid.
func collectAndMergeSpecList(specList []any, devcontainerMap map[string]any, name string) (map[string]any, error) {
	log.Debug("Detected list-based spec, collecting indexed items", logKeyName, name, "items", len(specList))
	maps := make([]map[string]any, 0, len(specList))

	// Collect spec[0], spec[1], etc. which contain the processed YAML functions
	for i := 0; i < len(specList); i++ {
		key := fmt.Sprintf("spec[%d]", i)
		item, exists := devcontainerMap[key]
		if !exists {
			continue
		}
		itemMap, ok := item.(map[string]any)
		if !ok {
			log.Warn("Spec list item is not a map", logKeyName, name, "index", i, "type", fmt.Sprintf("%T", item))
			continue
		}
		maps = append(maps, itemMap)
	}

	if len(maps) == 0 {
		return nil, fmt.Errorf("%w: devcontainer `%s` has empty spec list", errUtils.ErrInvalidDevcontainerConfig, name)
	}

	return mergeSpecMaps(maps, name)
}

// mergeSpecMaps merges a list of maps into a single map.
// This supports the pattern where spec is defined as a sequence of maps to be merged:
//
//	spec:
//	  - !include devcontainer.json
//	  - forwardPorts:
//	      - !random 8080 8099
func mergeSpecMaps(maps []map[string]any, name string) (map[string]any, error) {
	defer perf.Track(nil, "devcontainer.mergeSpecMaps")()

	if len(maps) == 0 {
		return nil, errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Devcontainer `%s` spec list cannot be empty", name).
			WithHint("Either use a map directly or provide at least one item in the list").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithContext("devcontainer_name", name).
			WithExitCode(2).
			Err()
	}

	// Merge all maps in order using mergo with override.
	result := make(map[string]any)
	for k, v := range maps[0] {
		result[k] = v
	}

	log.Debug("Starting merge", logKeyName, name, "map_count", len(maps), "initial_keys", len(result))

	for i := 1; i < len(maps); i++ {
		log.Debug("Merging map", logKeyName, name, "index", i, "keys", len(maps[i]))
		if err := mergo.Merge(&result, maps[i], mergo.WithOverride); err != nil {
			return nil, errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
				WithCause(err).
				WithExplanationf("Failed to merge devcontainer `%s` spec list", name).
				WithHint("Check that all items in the spec list are valid maps").
				WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
				WithContext("devcontainer_name", name).
				WithExitCode(2).
				Err()
		}
		log.Debug("After merge", logKeyName, name, "result_keys", len(result))
	}

	log.Debug("Final merged result", logKeyName, name, "keys", fmt.Sprintf("%v", getKeys(result)))

	// Fix indexed array keys (e.g., forwardports[0], forwardports[1] → forwardports: [value0, value1])
	result = consolidateIndexedKeys(result)

	return result, nil
}

func getKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// consolidateIndexedKeys converts Viper's indexed keys (e.g., "ports[0]", "ports[1]") back into arrays.
// Viper stores both the raw list AND indexed keys for processed values.
// We need to collect the indexed values and replace the raw list.
func consolidateIndexedKeys(m map[string]any) map[string]any {
	defer perf.Track(nil, "devcontainer.consolidateIndexedKeys")()

	result := make(map[string]any)
	indexedArrays := make(map[string][]any)

	// First pass: identify indexed keys and collect their values
	for k, v := range m {
		// Check if key has format "name[index]"
		idx := strings.Index(k, "[")
		if idx <= 0 || !strings.HasSuffix(k, "]") {
			// Not an indexed key
			result[k] = v
			continue
		}

		baseName := k[:idx]
		indexStr := k[idx+1 : len(k)-1]
		var index int
		if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
			// Not a numeric index, keep as-is
			result[k] = v
			continue
		}

		// Valid indexed key
		if _, exists := indexedArrays[baseName]; !exists {
			indexedArrays[baseName] = make([]any, 0)
		}
		// Store with index for later sorting
		indexedArrays[baseName] = append(indexedArrays[baseName], indexedValue{index: index, value: v})
	}

	// Second pass: convert indexed arrays back to proper arrays
	for baseName, indexedVals := range indexedArrays {
		// Sort by index
		sortIndexedValues(indexedVals)

		// Extract just the values in order
		array := make([]any, len(indexedVals))
		for i, iv := range indexedVals {
			array[i] = iv.(indexedValue).value
		}

		// Replace the base key with the sorted array
		result[baseName] = array
	}

	return result
}

type indexedValue struct {
	index int
	value any
}

func sortIndexedValues(vals []any) {
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].(indexedValue).index < vals[j].(indexedValue).index
	})
}

// LoadAllConfigs loads all devcontainer configurations from atmos.yaml.
func LoadAllConfigs(atmosConfig *schema.AtmosConfiguration) (map[string]*Config, error) {
	defer perf.Track(atmosConfig, "devcontainer.LoadAllConfigs")()

	if atmosConfig.Devcontainer == nil {
		log.Debug("Devcontainers field is nil in atmosConfig")
		return map[string]*Config{}, nil
	}

	log.Debug("Loading devcontainer configs", "count", len(atmosConfig.Devcontainer))

	configs := make(map[string]*Config)

	for name := range atmosConfig.Devcontainer {
		log.Debug("Loading devcontainer", logKeyName, name)
		config, _, err := LoadConfig(atmosConfig, name)
		if err != nil {
			log.Debug("Failed to load devcontainer", logKeyName, name, "error", err)
			return nil, err
		}
		configs[name] = config
	}

	log.Debug("Successfully loaded devcontainer configs", "count", len(configs))

	return configs, nil
}

// filterUnsupportedFields logs debug messages for unsupported devcontainer fields.
// Note: Viper lowercases all keys, so we use lowercase field names for comparison.
func filterUnsupportedFields(data map[string]any, name string) {
	defer perf.Track(nil, "devcontainer.filterUnsupportedFields")()

	// All field names in lowercase because Viper lowercases keys.
	supportedFields := map[string]bool{
		// Core fields.
		"name":  true,
		"image": true,
		"build": true,
		// Workspace configuration.
		"workspacefolder": true,
		"workspacemount":  true,
		"mounts":          true,
		// Port configuration.
		"forwardports":    true,
		"portsattributes": true,
		// Environment and user.
		"containerenv": true,
		"remoteuser":   true,
		// Runtime configuration.
		"runargs":         true,
		"overridecommand": true,
		"init":            true,
		"privileged":      true,
		"capadd":          true,
		"securityopt":     true,
		"userenvprobe":    true,
	}

	// Log unsupported fields at debug level.
	// Keys from data map are already lowercase due to Viper.
	for key := range data {
		if !supportedFields[key] {
			log.Debug("Ignoring unsupported devcontainer field",
				"field", key,
				"devcontainer", name)
		}
	}
}

// validateConfig validates that the devcontainer configuration has required fields.
func validateConfig(config *Config) error {
	defer perf.Track(nil, "devcontainer.validateConfig")()

	if config.Name == "" {
		return fmt.Errorf("%w: devcontainer name is required", errUtils.ErrInvalidDevcontainerConfig)
	}

	// Must have either image or build configuration
	if config.Image == "" && config.Build == nil {
		return fmt.Errorf("%w: devcontainer '%s' must specify either 'image' or 'build'", errUtils.ErrInvalidDevcontainerConfig, config.Name)
	}

	// If build is specified, validate it
	if config.Build != nil {
		if config.Build.Dockerfile == "" {
			return fmt.Errorf("%w: devcontainer '%s' build.dockerfile is required", errUtils.ErrInvalidDevcontainerConfig, config.Name)
		}
		if config.Build.Context == "" {
			config.Build.Context = "." // Default context
		}
	}

	return nil
}
