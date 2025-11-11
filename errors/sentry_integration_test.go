package errors

import (
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIntegration_ComponentLevelSentryConfig demonstrates the full integration
// of component-level Sentry configuration with stack settings.
func TestIntegration_ComponentLevelSentryConfig(t *testing.T) {
	// Simulate global atmos.yaml configuration.
	globalConfig := &schema.ErrorsConfig{
		Sentry: schema.SentryConfig{
			Enabled:     true,
			DSN:         "https://global@sentry.io/123",
			Environment: "production",
			SampleRate:  0.1, // Default: 10% sampling.
			Tags: map[string]string{
				"service": "atmos",
			},
		},
	}

	tests := []struct {
		name                   string
		componentSettings      map[string]any
		expectedEnabled        bool
		expectedEnvironment    string
		expectedSampleRate     float64
		expectedTags           map[string]string
		expectedClientCount    int
		expectSameAsOther      bool
		otherComponentSettings map[string]any
	}{
		{
			name: "component with no error settings uses global",
			componentSettings: map[string]any{
				"spacelift": map[string]any{
					"enabled": true,
				},
			},
			expectedEnabled:     true,
			expectedEnvironment: "production",
			expectedSampleRate:  0.1,
			expectedTags: map[string]string{
				"service": "atmos",
			},
			expectedClientCount: 1,
		},
		{
			name: "component overrides environment",
			componentSettings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"environment": "staging",
					},
				},
			},
			expectedEnabled:     true,
			expectedEnvironment: "staging", // Component override.
			expectedSampleRate:  0.1,       // From global.
			expectedTags: map[string]string{
				"service": "atmos", // From global.
			},
			expectedClientCount: 1,
		},
		{
			name: "critical component with higher sample rate",
			componentSettings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"sample_rate": 1.0, // 100% sampling for critical components.
						"tags": map[string]string{
							"team":        "payments",
							"criticality": "critical",
						},
					},
				},
			},
			expectedEnabled:     true,
			expectedEnvironment: "production", // From global.
			expectedSampleRate:  1.0,          // Component override.
			expectedTags: map[string]string{
				"service":     "atmos",    // From global.
				"team":        "payments", // From component.
				"criticality": "critical", // From component.
			},
			expectedClientCount: 1,
		},
		{
			name: "component with team tag override",
			componentSettings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"tags": map[string]string{
							"team": "infrastructure",
						},
					},
				},
			},
			expectedEnabled:     true,
			expectedEnvironment: "production",
			expectedSampleRate:  0.1,
			expectedTags: map[string]string{
				"service": "atmos",
				"team":    "infrastructure", // From component.
			},
			expectedClientCount: 1,
		},
		{
			name: "two components with identical config share client",
			componentSettings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"environment": "prod-database",
						"tags": map[string]string{
							"team": "database",
						},
					},
				},
			},
			otherComponentSettings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"environment": "prod-database",
						"tags": map[string]string{
							"team": "database",
						},
					},
				},
			},
			expectedEnabled:     true,
			expectedEnvironment: "prod-database",
			expectedSampleRate:  0.1,
			expectedTags: map[string]string{
				"service": "atmos",
				"team":    "database",
			},
			expectedClientCount: 1,
			expectSameAsOther:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh registry for each test.
			registry := &SentryClientRegistry{
				clients: make(map[string]*sentry.Hub),
				configs: make(map[string]*schema.SentryConfig),
			}

			// Simulate ConfigAndStacksInfo from component processing.
			info := &schema.ConfigAndStacksInfo{
				ComponentSettingsSection: tt.componentSettings,
			}

			// Extract component error config.
			componentConfig, err := GetComponentErrorConfig(info)
			require.NoError(t, err)

			// Merge with global config.
			finalConfig := MergeErrorConfigs(globalConfig, componentConfig)

			// Verify merged configuration.
			assert.Equal(t, tt.expectedEnabled, finalConfig.Sentry.Enabled)
			assert.Equal(t, tt.expectedEnvironment, finalConfig.Sentry.Environment)
			assert.Equal(t, tt.expectedSampleRate, finalConfig.Sentry.SampleRate)
			assert.Equal(t, tt.expectedTags, finalConfig.Sentry.Tags)

			// Get or create Sentry client.
			hub, hubErr := registry.GetOrCreateClient(&finalConfig.Sentry)
			require.NoError(t, hubErr)
			require.NotNil(t, hub)

			// If we need to test client sharing, process the other component.
			if tt.expectSameAsOther {
				otherInfo := &schema.ConfigAndStacksInfo{
					ComponentSettingsSection: tt.otherComponentSettings,
				}

				otherComponentConfig, err := GetComponentErrorConfig(otherInfo)
				require.NoError(t, err)

				otherFinalConfig := MergeErrorConfigs(globalConfig, otherComponentConfig)

				otherHub, err := registry.GetOrCreateClient(&otherFinalConfig.Sentry)
				require.NoError(t, err)

				// Should reuse the same hub.
				assert.Equal(t, hub, otherHub, "Identical configs should share the same Sentry client")
			}

			// Verify client count.
			assert.Len(t, registry.clients, tt.expectedClientCount)
		})
	}
}

// TestIntegration_StackLevelSentryConfig demonstrates stack-level settings inheritance.
func TestIntegration_StackLevelSentryConfig(t *testing.T) {
	// This test demonstrates how settings can be defined at the stack level
	// and inherited by all components in that stack.

	// Global config (atmos.yaml).
	globalConfig := &schema.ErrorsConfig{
		Sentry: schema.SentryConfig{
			Enabled:     true,
			DSN:         "https://global@sentry.io/123",
			Environment: "production",
			SampleRate:  1.0,
		},
	}

	// Stack-level settings (stacks/prod/critical.yaml).
	// In practice, these would be merged by the stack processor before reaching
	// the component settings section.
	stackLevelSettings := map[string]any{
		"errors": map[string]any{
			"sentry": map[string]any{
				"tags": map[string]string{
					"team":        "payments",
					"criticality": "critical",
					"sla":         "99.99",
				},
			},
		},
	}

	// Component 1: payment-processor (inherits stack settings).
	component1Info := &schema.ConfigAndStacksInfo{
		Component:                "payment-processor",
		Stack:                    "prod/us-east-1",
		ComponentType:            "terraform",
		ComponentSettingsSection: stackLevelSettings,
	}

	// Component 2: payment-gateway (overrides sample_rate).
	component2Settings := map[string]any{
		"errors": map[string]any{
			"sentry": map[string]any{
				"sample_rate": 0.5, // Override for this component.
				"tags": map[string]string{
					"team":        "payments", // From stack.
					"criticality": "critical", // From stack.
					"sla":         "99.99",    // From stack.
					"subteam":     "gateway",  // Component-specific.
				},
			},
		},
	}

	component2Info := &schema.ConfigAndStacksInfo{
		Component:                "payment-gateway",
		Stack:                    "prod/us-east-1",
		ComponentType:            "terraform",
		ComponentSettingsSection: component2Settings,
	}

	// Process component 1.
	config1, err1 := GetComponentErrorConfig(component1Info)
	require.NoError(t, err1)
	final1 := MergeErrorConfigs(globalConfig, config1)

	assert.Equal(t, true, final1.Sentry.Enabled)
	assert.Equal(t, "production", final1.Sentry.Environment)
	assert.Equal(t, 1.0, final1.Sentry.SampleRate)
	assert.Equal(t, map[string]string{
		"team":        "payments",
		"criticality": "critical",
		"sla":         "99.99",
	}, final1.Sentry.Tags)

	// Process component 2.
	config2, err2 := GetComponentErrorConfig(component2Info)
	require.NoError(t, err2)
	final2 := MergeErrorConfigs(globalConfig, config2)

	assert.Equal(t, true, final2.Sentry.Enabled)
	assert.Equal(t, "production", final2.Sentry.Environment)
	assert.Equal(t, 0.5, final2.Sentry.SampleRate) // Overridden.
	assert.Equal(t, map[string]string{
		"team":        "payments",
		"criticality": "critical",
		"sla":         "99.99",
		"subteam":     "gateway", // Component-specific.
	}, final2.Sentry.Tags)
}

// TestIntegration_RealWorldExample demonstrates a real-world scenario with
// multiple components and different error configurations.
func TestIntegration_RealWorldExample(t *testing.T) {
	// Global configuration.
	globalConfig := &schema.ErrorsConfig{
		Sentry: schema.SentryConfig{
			Enabled:     true,
			DSN:         "https://example@sentry.io/project",
			Environment: "production",
			SampleRate:  0.1, // Default: 10% sampling.
			Tags: map[string]string{
				"service": "atmos",
			},
		},
	}

	scenarios := []struct {
		name         string
		component    string
		stack        string
		settings     map[string]any
		expectedTags map[string]string
		expectedRate float64
		expectedEnv  string
	}{
		{
			name:      "VPC component - standard monitoring",
			component: "vpc",
			stack:     "prod/us-east-1",
			settings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"tags": map[string]string{
							"team": "infrastructure",
						},
					},
				},
			},
			expectedTags: map[string]string{
				"service": "atmos",
				"team":    "infrastructure",
			},
			expectedRate: 0.1,
			expectedEnv:  "production",
		},
		{
			name:      "Payment processor - critical component with full sampling",
			component: "payment-processor",
			stack:     "prod/us-east-1",
			settings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"sample_rate": 1.0, // 100% sampling.
						"tags": map[string]string{
							"team":        "payments",
							"criticality": "critical",
							"pci":         "true",
						},
					},
				},
			},
			expectedTags: map[string]string{
				"service":     "atmos",
				"team":        "payments",
				"criticality": "critical",
				"pci":         "true",
			},
			expectedRate: 1.0,
			expectedEnv:  "production",
		},
		{
			name:      "Test database - staging environment",
			component: "test-db",
			stack:     "staging/us-west-2",
			settings: map[string]any{
				"errors": map[string]any{
					"sentry": map[string]any{
						"environment": "staging",
						"sample_rate": 0.5,
						"tags": map[string]string{
							"team": "database",
						},
					},
				},
			},
			expectedTags: map[string]string{
				"service": "atmos",
				"team":    "database",
			},
			expectedRate: 0.5,
			expectedEnv:  "staging",
		},
	}

	registry := &SentryClientRegistry{
		clients: make(map[string]*sentry.Hub),
		configs: make(map[string]*schema.SentryConfig),
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{
				Component:                scenario.component,
				Stack:                    scenario.stack,
				ComponentType:            "terraform",
				ComponentSettingsSection: scenario.settings,
			}

			// Get component error config.
			componentConfig, err := GetComponentErrorConfig(info)
			require.NoError(t, err)

			// Merge with global.
			finalConfig := MergeErrorConfigs(globalConfig, componentConfig)

			// Verify configuration.
			assert.Equal(t, scenario.expectedEnv, finalConfig.Sentry.Environment)
			assert.Equal(t, scenario.expectedRate, finalConfig.Sentry.SampleRate)
			assert.Equal(t, scenario.expectedTags, finalConfig.Sentry.Tags)

			// Get or create client.
			hub, err := registry.GetOrCreateClient(&finalConfig.Sentry)
			require.NoError(t, err)
			require.NotNil(t, hub)
		})
	}

	// Verify that we created multiple clients (different configs).
	// We should have 3 clients: one for each unique configuration.
	assert.Equal(t, 3, len(registry.clients), "Should have 3 different Sentry clients")
}
