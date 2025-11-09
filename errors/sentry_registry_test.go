package errors

import (
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigKey(t *testing.T) {
	tests := []struct {
		name     string
		config1  *schema.SentryConfig
		config2  *schema.SentryConfig
		expected string // "same" or "different"
	}{
		{
			name:     "nil config",
			config1:  nil,
			config2:  nil,
			expected: "same",
		},
		{
			name: "identical configs",
			config1: &schema.SentryConfig{
				Enabled:     true,
				DSN:         "https://test@sentry.io/123",
				Environment: "production",
				SampleRate:  1.0,
			},
			config2: &schema.SentryConfig{
				Enabled:     true,
				DSN:         "https://test@sentry.io/123",
				Environment: "production",
				SampleRate:  1.0,
			},
			expected: "same",
		},
		{
			name: "different DSN",
			config1: &schema.SentryConfig{
				Enabled: true,
				DSN:     "https://test1@sentry.io/123",
			},
			config2: &schema.SentryConfig{
				Enabled: true,
				DSN:     "https://test2@sentry.io/123",
			},
			expected: "different",
		},
		{
			name: "different environment",
			config1: &schema.SentryConfig{
				Enabled:     true,
				DSN:         "https://test@sentry.io/123",
				Environment: "production",
			},
			config2: &schema.SentryConfig{
				Enabled:     true,
				DSN:         "https://test@sentry.io/123",
				Environment: "staging",
			},
			expected: "different",
		},
		{
			name: "different sample rate",
			config1: &schema.SentryConfig{
				Enabled:    true,
				DSN:        "https://test@sentry.io/123",
				SampleRate: 0.5,
			},
			config2: &schema.SentryConfig{
				Enabled:    true,
				DSN:        "https://test@sentry.io/123",
				SampleRate: 1.0,
			},
			expected: "different",
		},
		{
			name: "different tags",
			config1: &schema.SentryConfig{
				Enabled: true,
				DSN:     "https://test@sentry.io/123",
				Tags: map[string]string{
					"team": "platform",
				},
			},
			config2: &schema.SentryConfig{
				Enabled: true,
				DSN:     "https://test@sentry.io/123",
				Tags: map[string]string{
					"team": "infrastructure",
				},
			},
			expected: "different",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1, err1 := configKey(tt.config1)
			require.NoError(t, err1)

			key2, err2 := configKey(tt.config2)
			require.NoError(t, err2)

			if tt.expected == "same" {
				assert.Equal(t, key1, key2, "Config keys should be identical for same configs")
			} else {
				assert.NotEqual(t, key1, key2, "Config keys should be different for different configs")
			}
		})
	}
}

func TestGetComponentErrorConfig(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected *schema.ErrorsConfig
		wantErr  bool
	}{
		{
			name:     "nil info",
			info:     nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name: "no component settings",
			info: &schema.ConfigAndStacksInfo{
				ComponentSettingsSection: nil,
			},
			expected: nil,
			wantErr:  false,
		},
		{
			name: "no errors in settings",
			info: &schema.ConfigAndStacksInfo{
				ComponentSettingsSection: map[string]any{
					"spacelift": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: nil,
			wantErr:  false,
		},
		{
			name: "errors settings present",
			info: &schema.ConfigAndStacksInfo{
				ComponentSettingsSection: map[string]any{
					"errors": map[string]any{
						"sentry": map[string]any{
							"enabled":     true,
							"environment": "production",
							"sample_rate": 0.5,
							"tags": map[string]string{
								"team": "platform",
							},
						},
					},
				},
			},
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					Environment: "production",
					SampleRate:  0.5,
					Tags: map[string]string{
						"team": "platform",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetComponentErrorConfig(tt.info)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Sentry.Enabled, result.Sentry.Enabled)
				assert.Equal(t, tt.expected.Sentry.Environment, result.Sentry.Environment)
				assert.Equal(t, tt.expected.Sentry.SampleRate, result.Sentry.SampleRate)
				assert.Equal(t, tt.expected.Sentry.Tags, result.Sentry.Tags)
			}
		})
	}
}

func TestMergeErrorConfigs(t *testing.T) {
	tests := []struct {
		name      string
		global    *schema.ErrorsConfig
		component *schema.ErrorsConfig
		expected  *schema.ErrorsConfig
	}{
		{
			name: "component is nil - use global",
			global: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					SampleRate:  1.0,
				},
			},
			component: nil,
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					SampleRate:  1.0,
				},
			},
		},
		{
			name: "component overrides environment",
			global: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					SampleRate:  1.0,
				},
			},
			component: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Environment: "staging",
				},
			},
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "staging", // Component override.
					SampleRate:  1.0,
				},
			},
		},
		{
			name: "component overrides sample rate",
			global: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					SampleRate:  1.0,
				},
			},
			component: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					SampleRate: 0.5,
				},
			},
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					SampleRate:  0.5, // Component override.
				},
			},
		},
		{
			name: "component adds tags",
			global: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					Tags: map[string]string{
						"service": "atmos",
					},
				},
			},
			component: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Tags: map[string]string{
						"team": "platform",
					},
				},
			},
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					Tags: map[string]string{
						"service": "atmos",    // From global.
						"team":    "platform", // From component.
					},
				},
			},
		},
		{
			name: "component overrides existing tag",
			global: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					Tags: map[string]string{
						"team": "infrastructure",
					},
				},
			},
			component: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Tags: map[string]string{
						"team": "platform", // Override global tag.
					},
				},
			},
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled:     true,
					DSN:         "https://global@sentry.io/123",
					Environment: "production",
					Tags: map[string]string{
						"team": "platform", // Component override wins.
					},
				},
			},
		},
		{
			name: "component enables sentry when global is disabled",
			global: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled: false,
					DSN:     "https://global@sentry.io/123",
				},
			},
			component: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled: true,
				},
			},
			expected: &schema.ErrorsConfig{
				Sentry: schema.SentryConfig{
					Enabled: true, // Component enables it.
					DSN:     "https://global@sentry.io/123",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeErrorConfigs(tt.global, tt.component)

			assert.Equal(t, tt.expected.Sentry.Enabled, result.Sentry.Enabled)
			assert.Equal(t, tt.expected.Sentry.DSN, result.Sentry.DSN)
			assert.Equal(t, tt.expected.Sentry.Environment, result.Sentry.Environment)
			assert.Equal(t, tt.expected.Sentry.SampleRate, result.Sentry.SampleRate)
			assert.Equal(t, tt.expected.Sentry.Tags, result.Sentry.Tags)
		})
	}
}

func TestSentryClientRegistryReuse(t *testing.T) {
	// Create a new registry for testing.
	registry := &SentryClientRegistry{
		clients: make(map[string]*sentry.Hub),
		configs: make(map[string]*schema.SentryConfig),
	}

	config1 := &schema.SentryConfig{
		Enabled:     true,
		DSN:         "https://test@sentry.io/123",
		Environment: "production",
		SampleRate:  1.0,
	}

	config2 := &schema.SentryConfig{
		Enabled:     true,
		DSN:         "https://test@sentry.io/123",
		Environment: "production",
		SampleRate:  1.0,
	}

	// Get client for config1.
	hub1, err1 := registry.GetOrCreateClient(config1)
	require.NoError(t, err1)
	require.NotNil(t, hub1)

	// Get client for config2 (identical to config1).
	hub2, err2 := registry.GetOrCreateClient(config2)
	require.NoError(t, err2)
	require.NotNil(t, hub2)

	// Should reuse the same hub.
	assert.Equal(t, hub1, hub2, "Identical configs should reuse the same Sentry hub")

	// Verify only one client was created.
	assert.Len(t, registry.clients, 1, "Should have exactly one client")
}

func TestSentryClientRegistryDifferentConfigs(t *testing.T) {
	// Create a new registry for testing.
	registry := &SentryClientRegistry{
		clients: make(map[string]*sentry.Hub),
		configs: make(map[string]*schema.SentryConfig),
	}

	config1 := &schema.SentryConfig{
		Enabled:     true,
		DSN:         "https://test@sentry.io/123",
		Environment: "production",
		SampleRate:  1.0,
	}

	config2 := &schema.SentryConfig{
		Enabled:     true,
		DSN:         "https://test@sentry.io/123",
		Environment: "staging", // Different environment.
		SampleRate:  1.0,
	}

	// Get client for config1.
	hub1, err1 := registry.GetOrCreateClient(config1)
	require.NoError(t, err1)
	require.NotNil(t, hub1)

	// Get client for config2 (different environment).
	hub2, err2 := registry.GetOrCreateClient(config2)
	require.NoError(t, err2)
	require.NotNil(t, hub2)

	// Should create different hubs.
	assert.NotEqual(t, hub1, hub2, "Different configs should create different Sentry hubs")

	// Verify two clients were created.
	assert.Len(t, registry.clients, 2, "Should have exactly two clients")
}

func TestSentryClientRegistryDisabled(t *testing.T) {
	registry := &SentryClientRegistry{
		clients: make(map[string]*sentry.Hub),
		configs: make(map[string]*schema.SentryConfig),
	}

	config := &schema.SentryConfig{
		Enabled: false,
		DSN:     "https://test@sentry.io/123",
	}

	hub, err := registry.GetOrCreateClient(config)
	assert.NoError(t, err)
	assert.Nil(t, hub, "Disabled Sentry config should return nil hub")
}
