package devcontainer

import (
	"fmt"

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

// LoadConfig loads a devcontainer configuration by name from atmos.yaml components.devcontainer section.
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
	log.Debug("LoadConfig called", logKeyName, name, "devcontainer_nil", atmosConfig.Components.Devcontainer == nil)

	if atmosConfig.Components.Devcontainer == nil {
		log.Debug("No devcontainers configured in Components.Devcontainer")
		return nil, fmt.Errorf("%w: no devcontainers configured", errUtils.ErrDevcontainerNotFound)
	}

	log.Debug("Devcontainer field populated", "count", len(atmosConfig.Components.Devcontainer))

	rawDevcontainer, exists := atmosConfig.Components.Devcontainer[name]
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
		return nil, fmt.Errorf("%w: failed to marshal settings for %s: %v", errUtils.ErrInvalidDevcontainerConfig, name, err)
	}

	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal settings for %s: %v", errUtils.ErrInvalidDevcontainerConfig, name, err)
	}

	return &settings, nil
}

func extractAndValidateSpec(devcontainerMap map[string]any, name string) (*Config, error) {
	specRaw, hasSpec := devcontainerMap["spec"]
	if !hasSpec {
		return nil, fmt.Errorf("%w: devcontainer '%s' missing 'spec' section", errUtils.ErrInvalidDevcontainerConfig, name)
	}

	specMap, ok := specRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: devcontainer spec must be a map", errUtils.ErrInvalidDevcontainerConfig)
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

// LoadAllConfigs loads all devcontainer configurations from atmos.yaml.
func LoadAllConfigs(atmosConfig *schema.AtmosConfiguration) (map[string]*Config, error) {
	defer perf.Track(atmosConfig, "devcontainer.LoadAllConfigs")()

	if atmosConfig.Components.Devcontainer == nil {
		log.Debug("Devcontainers field is nil in atmosConfig")
		return map[string]*Config{}, nil
	}

	log.Debug("Loading devcontainer configs", "count", len(atmosConfig.Components.Devcontainer))

	configs := make(map[string]*Config)

	for name := range atmosConfig.Components.Devcontainer {
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
func filterUnsupportedFields(data map[string]any, name string) {
	defer perf.Track(nil, "devcontainer.filterUnsupportedFields")()

	supportedFields := map[string]bool{
		"name":            true,
		"image":           true,
		"build":           true,
		"workspaceFolder": true,
		"workspaceMount":  true,
		"mounts":          true,
		"forwardPorts":    true,
		"portsAttributes": true,
		"containerEnv":    true,
		"runArgs":         true,
		"remoteUser":      true,
	}

	// Log unsupported fields at debug level.
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
