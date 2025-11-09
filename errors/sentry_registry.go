package errors

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/getsentry/sentry-go"
	"github.com/mitchellh/mapstructure"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SentryClientRegistry manages Sentry clients per component configuration.
// It implements Option 3: Multiple Sentry Clients with caching and reuse.
type SentryClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]*sentry.Hub
	configs map[string]*schema.SentryConfig
}

// globalRegistry is the singleton registry instance.
var globalRegistry = &SentryClientRegistry{
	clients: make(map[string]*sentry.Hub),
	configs: make(map[string]*schema.SentryConfig),
}

// GetRegistry returns the global Sentry client registry.
func GetRegistry() *SentryClientRegistry {
	return globalRegistry
}

// configKey generates a unique key for a Sentry configuration.
// This allows reusing clients for identical configurations.
func configKey(config *schema.SentryConfig) (string, error) {
	if config == nil {
		return "nil", nil
	}

	// Create a normalized representation for hashing.
	normalized := struct {
		Enabled             bool
		DSN                 string
		Environment         string
		Release             string
		SampleRate          float64
		Debug               bool
		Tags                map[string]string
		CaptureStackContext bool
	}{
		Enabled:             config.Enabled,
		DSN:                 config.DSN,
		Environment:         config.Environment,
		Release:             config.Release,
		SampleRate:          config.SampleRate,
		Debug:               config.Debug,
		Tags:                config.Tags,
		CaptureStackContext: config.CaptureStackContext,
	}

	// Marshal to JSON for consistent hashing.
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config for hashing: %w", err)
	}

	// Generate SHA256 hash.
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:16]), nil // Use first 16 bytes for shorter key.
}

// GetOrCreateClient returns a Sentry hub for the given configuration.
// It reuses existing clients for identical configurations.
func (r *SentryClientRegistry) GetOrCreateClient(config *schema.SentryConfig) (*sentry.Hub, error) {
	// If Sentry is disabled, return nil.
	if config == nil || !config.Enabled {
		return nil, nil
	}

	// Generate config key for lookup/storage.
	key, err := configKey(config)
	if err != nil {
		return nil, err
	}

	// Check if client already exists (read lock).
	r.mu.RLock()
	if hub, exists := r.clients[key]; exists {
		r.mu.RUnlock()
		return hub, nil
	}
	r.mu.RUnlock()

	// Create new client (write lock).
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it).
	if hub, exists := r.clients[key]; exists {
		return hub, nil
	}

	// Create new Sentry client.
	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = 1.0
	}

	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:              config.DSN,
		Environment:      config.Environment,
		Release:          config.Release,
		Debug:            config.Debug,
		SampleRate:       sampleRate,
		AttachStacktrace: config.CaptureStackContext,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Sentry client: %w", err)
	}

	// Create new hub with this client.
	hub := sentry.NewHub(client, sentry.NewScope())

	// Set tags in the hub's scope.
	if len(config.Tags) > 0 {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			for key, value := range config.Tags {
				scope.SetTag(key, value)
			}
		})
	}

	// Store client and config.
	r.clients[key] = hub
	r.configs[key] = config

	return hub, nil
}

// CloseAll flushes and closes all Sentry clients in the registry.
func (r *SentryClientRegistry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, hub := range r.clients {
		hub.Flush(CloseSentryTimeout)
	}

	// Clear the registry.
	r.clients = make(map[string]*sentry.Hub)
	r.configs = make(map[string]*schema.SentryConfig)
}

// componentErrorConfigWithMetadata holds both the decoded config and metadata about which fields were explicitly set.
type componentErrorConfigWithMetadata struct {
	config                        *schema.ErrorsConfig
	sentryEnabledExplicitlySet    bool
	sentryDebugExplicitlySet      bool
	sentryCaptureStackExplicitlySet bool
}

// GetComponentErrorConfig extracts the merged error configuration from component settings.
// It follows the same pattern used by Spacelift, Atlantis, and Pro integrations.
func GetComponentErrorConfig(info *schema.ConfigAndStacksInfo) (*schema.ErrorsConfig, error) {
	if info == nil {
		return nil, nil
	}

	// Check if component has errors settings override.
	if info.ComponentSettingsSection != nil {
		if errorsSettings, ok := info.ComponentSettingsSection["errors"].(map[string]any); ok {
			metadata := getComponentErrorConfigWithMetadata(errorsSettings)
			if metadata.config != nil {
				// Store metadata in the config for use during merge.
				// Note: We use a package-level map to avoid modifying the schema.
				componentMetadataStore[metadata.config] = metadata
			}
			return metadata.config, nil
		}
	}

	// No component-level override, return nil (will use global config).
	return nil, nil
}

// componentMetadataStore maps component configs to their metadata.
// This is used to track which boolean fields were explicitly set in YAML.
var componentMetadataStore = make(map[*schema.ErrorsConfig]componentErrorConfigWithMetadata)

// getComponentErrorConfigWithMetadata decodes component error settings and tracks which boolean fields were explicitly set.
func getComponentErrorConfigWithMetadata(errorsSettings map[string]any) componentErrorConfigWithMetadata {
	var config schema.ErrorsConfig
	if err := mapstructure.Decode(errorsSettings, &config); err != nil {
		return componentErrorConfigWithMetadata{}
	}

	metadata := componentErrorConfigWithMetadata{
		config: &config,
	}

	// Check if Sentry section exists and has explicit boolean fields.
	if sentrySettings, ok := errorsSettings["sentry"].(map[string]any); ok {
		_, metadata.sentryEnabledExplicitlySet = sentrySettings["enabled"]
		_, metadata.sentryDebugExplicitlySet = sentrySettings["debug"]
		_, metadata.sentryCaptureStackExplicitlySet = sentrySettings["capture_stack_context"]
	}

	return metadata
}

// MergeErrorConfigs merges component-level error config with global config.
// Component settings override global settings where specified.
func MergeErrorConfigs(global *schema.ErrorsConfig, component *schema.ErrorsConfig) *schema.ErrorsConfig {
	if component == nil {
		return global
	}

	if global == nil {
		global = &schema.ErrorsConfig{}
	}

	// Start with a copy of global config.
	merged := &schema.ErrorsConfig{
		Format: global.Format,
		Sentry: global.Sentry,
	}

	// Check if component has any explicit Sentry configuration (non-boolean fields).
	// Boolean fields are handled separately to allow explicit true/false overrides.
	hasComponentSentry := component.Sentry.DSN != "" ||
		component.Sentry.Environment != "" ||
		component.Sentry.Release != "" ||
		component.Sentry.SampleRate > 0 ||
		len(component.Sentry.Tags) > 0

	// Get metadata about which boolean fields were explicitly set.
	metadata, hasMetadata := componentMetadataStore[component]

	// Determine which boolean fields have explicit overrides.
	var explicitBooleans booleanOverrides
	if hasMetadata {
		explicitBooleans.enabled = metadata.sentryEnabledExplicitlySet
		explicitBooleans.debug = metadata.sentryDebugExplicitlySet
		explicitBooleans.captureStackContext = metadata.sentryCaptureStackExplicitlySet
	} else if !hasComponentSentry {
		// Fallback: When no metadata AND no explicit non-boolean config,
		// check if boolean fields differ from global (explicit override).
		// This preserves the old behavior for tests.
		explicitBooleans.enabled = component.Sentry.Enabled != global.Sentry.Enabled
		explicitBooleans.debug = component.Sentry.Debug != global.Sentry.Debug
		explicitBooleans.captureStackContext = component.Sentry.CaptureStackContext != global.Sentry.CaptureStackContext
	}
	// Else: hasComponentSentry && !hasMetadata - assume zero values, don't override booleans.

	hasBooleanOverride := explicitBooleans.enabled || explicitBooleans.debug || explicitBooleans.captureStackContext

	// Override Sentry config if component specifies it.
	if hasComponentSentry || hasBooleanOverride {
		// Component has Sentry config - merge it.
		merged.Sentry = mergeSentryConfigs(&global.Sentry, &component.Sentry, hasComponentSentry, explicitBooleans)
	}

	// Override format config if component specifies it.
	if component.Format.Verbose != global.Format.Verbose {
		merged.Format.Verbose = component.Format.Verbose
	}

	// Clean up metadata after merge.
	if hasMetadata {
		delete(componentMetadataStore, component)
	}

	return merged
}

// booleanOverrides tracks which boolean fields were explicitly set in component config.
type booleanOverrides struct {
	enabled             bool
	debug               bool
	captureStackContext bool
}

// mergeSentryConfigs merges component Sentry config with global Sentry config.
// hasExplicitConfig indicates whether the component has explicit non-boolean Sentry config.
// explicitBooleans indicates which boolean fields were explicitly set in the component YAML.
func mergeSentryConfigs(global *schema.SentryConfig, component *schema.SentryConfig, hasExplicitConfig bool, explicitBooleans booleanOverrides) schema.SentryConfig {
	var merged schema.SentryConfig
	if global != nil {
		merged = *global // Start with global config.
	}

	// Copy global tags to avoid modifying the original.
	if global != nil && global.Tags != nil {
		merged.Tags = make(map[string]string)
		for key, value := range global.Tags {
			merged.Tags[key] = value
		}
	}

	// Override fields that are explicitly set in component config.
	if component.DSN != "" {
		merged.DSN = component.DSN
	}
	if component.Environment != "" {
		merged.Environment = component.Environment
	}
	if component.Release != "" {
		merged.Release = component.Release
	}
	if component.SampleRate > 0 {
		merged.SampleRate = component.SampleRate
	}

	// For boolean fields:
	// Apply component boolean values when they were explicitly set in YAML,
	// regardless of whether other explicit config (DSN/Environment/Tags) is present.
	// This allows components to set `enabled: false` alongside other Sentry fields.
	if !hasExplicitConfig && global != nil {
		// Component only has boolean overrides (no DSN/Environment/Tags).
		// Apply boolean differences - this allows `enabled: false` to disable Sentry.
		if component.Enabled != global.Enabled {
			merged.Enabled = component.Enabled
		}
		if component.Debug != global.Debug {
			merged.Debug = component.Debug
		}
		if component.CaptureStackContext != global.CaptureStackContext {
			merged.CaptureStackContext = component.CaptureStackContext
		}
	} else if global == nil {
		// No global config - use component values directly.
		merged.Enabled = component.Enabled
		merged.Debug = component.Debug
		merged.CaptureStackContext = component.CaptureStackContext
	} else if hasExplicitConfig {
		// Component has explicit non-boolean config (DSN/Environment/Tags).
		// Apply boolean overrides only if they were explicitly set in YAML.
		if explicitBooleans.enabled {
			merged.Enabled = component.Enabled
		}
		if explicitBooleans.debug {
			merged.Debug = component.Debug
		}
		if explicitBooleans.captureStackContext {
			merged.CaptureStackContext = component.CaptureStackContext
		}
	}

	// Merge tags (component tags override global tags with same key).
	if len(component.Tags) > 0 {
		if merged.Tags == nil {
			merged.Tags = make(map[string]string)
		}
		for key, value := range component.Tags {
			merged.Tags[key] = value
		}
	}

	return merged
}
