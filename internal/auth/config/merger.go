package config

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// merger implements the ConfigMerger interface
type merger struct{}

// NewConfigMerger creates a new config merger instance
func NewConfigMerger() types.ConfigMerger {
	return &merger{}
}

// MergeAuthConfig merges component auth config with global auth config
func (m *merger) MergeAuthConfig(global *schema.AuthConfig, component *schema.ComponentAuthConfig) (*schema.AuthConfig, error) {
	if global == nil {
		return nil, fmt.Errorf("global auth config cannot be nil")
	}

	merged := &schema.AuthConfig{
		Providers:  make(map[string]schema.Provider),
		Identities: make(map[string]schema.Identity),
	}

	// Start with global providers
	for name, provider := range global.Providers {
		merged.Providers[name] = provider
	}

	// Start with global identities
	for name, identity := range global.Identities {
		merged.Identities[name] = identity
	}

	// Apply component overrides if provided
	if component != nil {
		// Merge providers - component providers override global ones
		for name, provider := range component.Providers {
			if globalProvider, exists := merged.Providers[name]; exists {
				merged.Providers[name] = *m.MergeProvider(&globalProvider, &provider)
			} else {
				merged.Providers[name] = provider
			}
		}

		// Merge identities - component identities override global ones
		for name, identity := range component.Identities {
			if globalIdentity, exists := merged.Identities[name]; exists {
				merged.Identities[name] = *m.MergeIdentity(&globalIdentity, &identity)
			} else {
				merged.Identities[name] = identity
			}
		}
	}

	return merged, nil
}

// MergeIdentity merges component identity config with global identity config
func (m *merger) MergeIdentity(global *schema.Identity, component *schema.Identity) *schema.Identity {
	merged := &schema.Identity{
		Kind:        global.Kind,
		Default:     global.Default,
		Via:         global.Via,
		Principal:   make(map[string]interface{}),
		Credentials: make(map[string]interface{}),
		Alias:       global.Alias,
		Env:         global.Env,
	}

	// Copy global principal
	for k, v := range global.Principal {
		merged.Principal[k] = v
	}

	// Copy global credentials
	for k, v := range global.Credentials {
		merged.Credentials[k] = v
	}

	// Copy global environment variables
	copy(merged.Env, global.Env)

	// Apply component overrides
	if component.Default {
		merged.Default = component.Default
	}

	if component.Via != nil {
		merged.Via = component.Via
	}

	if component.Alias != "" {
		merged.Alias = component.Alias
	}

	// Merge principal - component values override global values
	for k, v := range component.Principal {
		merged.Principal[k] = v
	}

	// Merge credentials - component values override global values
	for k, v := range component.Credentials {
		merged.Credentials[k] = v
	}

	// Append component environment variables
	merged.Env = append(merged.Env, component.Env...)

	return merged
}

// MergeProvider merges component provider config with global provider config
func (m *merger) MergeProvider(global *schema.Provider, component *schema.Provider) *schema.Provider {
	merged := &schema.Provider{
		Kind:     global.Kind,
		StartURL: global.StartURL,
		Region:   global.Region,
		Session:  global.Session,
		Default:  global.Default,
		Spec:     make(map[string]interface{}),
	}

	// Copy global spec
	for k, v := range global.Spec {
		merged.Spec[k] = v
	}

	// Apply component overrides
	if component.StartURL != "" {
		merged.StartURL = component.StartURL
	}

	if component.Region != "" {
		merged.Region = component.Region
	}

	if component.Session != nil {
		merged.Session = component.Session
	}

	if component.Default {
		merged.Default = component.Default
	}

	// Merge spec - component values override global values
	for k, v := range component.Spec {
		merged.Spec[k] = v
	}

	return merged
}
