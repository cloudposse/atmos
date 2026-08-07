package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsProEnabled tests the Pro-enabled check logic.
// An instance is Pro-enabled when settings.pro.enabled is the boolean true.
func TestIsProEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		instance *schema.Instance
		expected bool
	}{
		{
			name: "pro enabled",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "pro enabled with drift_detection disabled",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
						"drift_detection": map[string]interface{}{
							"enabled": false,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pro disabled",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			// pro.enabled defaults to true when a pro block is present, matching
			// the Atmos Pro server-side default. A drift_detection block alone is
			// therefore enough for the instance to be pro-enabled.
			name: "pro enabled defaults true when pro block present (drift_detection only)",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "no pro settings",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings:  map[string]interface{}{},
			},
			expected: false,
		},
		{
			name: "pro settings not a map",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": "invalid",
				},
			},
			expected: false,
		},
		{
			// A non-boolean enabled value (malformed config) defaults to true,
			// matching the Atmos Pro server-side default.
			name: "enabled not a bool (string) defaults true",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": "true",
					},
				},
			},
			expected: true,
		},
		{
			// metadata.enabled: false forces pro off regardless of pro.enabled.
			name: "metadata.enabled false overrides pro.enabled true",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
				Metadata: map[string]interface{}{
					"enabled": false,
				},
			},
			expected: false,
		},
		{
			// metadata.enabled: true is the default and does not change anything.
			name: "metadata.enabled true keeps pro.enabled true",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
				Metadata: map[string]interface{}{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			// Regression: the pro subtree parsed from YAML can be
			// map[interface{}]interface{}. The effective-state helpers must
			// normalize it (sanitizeForJSON) before reading enabled, otherwise a
			// valid Pro config reads as disabled.
			name: "YAML-shaped pro block (map[interface{}]interface{}) is pro-enabled",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[interface{}]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isProEnabled(tc.instance)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCountEnabledDisabled verifies the tally used in the success toast.
// The tally uses the effective state (metadata.enabled > pro.enabled >
// drift_detection.enabled), so it matches exactly what is uploaded.
// "Disabled" covers explicit `pro.enabled: false`, `metadata.enabled: false`,
// and instances with no `pro` config. Drift counts instances where
// `pro.drift_detection.enabled: true` AND the instance is effectively pro-enabled.
func TestCountEnabledDisabled(t *testing.T) {
	enabledInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{"enabled": true}},
	}
	disabledInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{"enabled": false}},
	}
	noProInst := schema.Instance{
		Settings: map[string]any{},
	}
	nonBoolEnabledInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{"enabled": "true"}},
	}
	enabledWithDriftInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{
			"enabled":         true,
			"drift_detection": map[string]any{"enabled": true},
		}},
	}
	disabledWithDriftInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{
			"enabled":         false,
			"drift_detection": map[string]any{"enabled": true},
		}},
	}
	enabledDriftOffInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{
			"enabled":         true,
			"drift_detection": map[string]any{"enabled": false},
		}},
	}
	// metadata.enabled: false forces pro (and therefore drift) off even though
	// pro.enabled and drift_detection.enabled are both true.
	metadataDisabledInst := schema.Instance{
		Settings: map[string]any{"pro": map[string]any{
			"enabled":         true,
			"drift_detection": map[string]any{"enabled": true},
		}},
		Metadata: map[string]any{"enabled": false},
	}
	// Regression: pro and the nested drift_detection block parsed from YAML can
	// be map[interface{}]interface{}. The helpers must normalize these before
	// counting, otherwise this enabled+drift instance is miscounted as disabled.
	yamlShapedDriftInst := schema.Instance{
		Settings: map[string]any{"pro": map[interface{}]interface{}{
			"enabled":         true,
			"drift_detection": map[interface{}]interface{}{"enabled": true},
		}},
	}

	testCases := []struct {
		name             string
		instances        []schema.Instance
		expectedEnabled  int
		expectedDisabled int
		expectedDrift    int
	}{
		{
			name:             "empty slice",
			instances:        []schema.Instance{},
			expectedEnabled:  0,
			expectedDisabled: 0,
			expectedDrift:    0,
		},
		{
			name:             "all enabled",
			instances:        []schema.Instance{enabledInst, enabledInst, enabledInst},
			expectedEnabled:  3,
			expectedDisabled: 0,
			expectedDrift:    0,
		},
		{
			name:             "all explicitly disabled",
			instances:        []schema.Instance{disabledInst, disabledInst},
			expectedEnabled:  0,
			expectedDisabled: 2,
			expectedDrift:    0,
		},
		{
			name:             "no pro config counts as disabled",
			instances:        []schema.Instance{noProInst, noProInst},
			expectedEnabled:  0,
			expectedDisabled: 2,
			expectedDrift:    0,
		},
		{
			// A non-bool enabled value defaults to true (matches Atmos Pro).
			name:             "non-bool enabled defaults to enabled",
			instances:        []schema.Instance{nonBoolEnabledInst},
			expectedEnabled:  1,
			expectedDisabled: 0,
			expectedDrift:    0,
		},
		{
			name:             "mixed enabled/disabled/no-pro",
			instances:        []schema.Instance{enabledInst, disabledInst, noProInst, enabledInst},
			expectedEnabled:  2,
			expectedDisabled: 2,
			expectedDrift:    0,
		},
		{
			name:             "drift enabled on pro-enabled instance",
			instances:        []schema.Instance{enabledWithDriftInst, enabledInst},
			expectedEnabled:  2,
			expectedDisabled: 0,
			expectedDrift:    1,
		},
		{
			// Drift requires effective pro-enabled, so an instance with
			// pro.enabled: false is not counted as drift-enabled.
			name:             "drift not counted when pro disabled",
			instances:        []schema.Instance{disabledWithDriftInst},
			expectedEnabled:  0,
			expectedDisabled: 1,
			expectedDrift:    0,
		},
		{
			// metadata.enabled: false forces pro and drift off.
			name:             "metadata.enabled false disables pro and drift",
			instances:        []schema.Instance{metadataDisabledInst},
			expectedEnabled:  0,
			expectedDisabled: 1,
			expectedDrift:    0,
		},
		{
			name:             "drift_detection.enabled false not counted",
			instances:        []schema.Instance{enabledDriftOffInst},
			expectedEnabled:  1,
			expectedDisabled: 0,
			expectedDrift:    0,
		},
		{
			// disabledWithDriftInst is pro-disabled, so its drift does not count.
			name:             "mixed with drift",
			instances:        []schema.Instance{enabledWithDriftInst, disabledWithDriftInst, enabledInst, noProInst},
			expectedEnabled:  2,
			expectedDisabled: 2,
			expectedDrift:    1,
		},
		{
			// YAML-shaped (map[interface{}]interface{}) pro + drift blocks must be
			// counted as enabled with drift on.
			name:             "YAML-shaped pro and drift blocks counted correctly",
			instances:        []schema.Instance{yamlShapedDriftInst},
			expectedEnabled:  1,
			expectedDisabled: 0,
			expectedDrift:    1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enabled, disabled, drift := countEnabledDisabled(tc.instances)
			assert.Equal(t, tc.expectedEnabled, enabled, "enabled count")
			assert.Equal(t, tc.expectedDisabled, disabled, "disabled count")
			assert.Equal(t, tc.expectedDrift, drift, "drift count")
		})
	}
}

// proMap is a helper for reading the nested pro block from an extractProSettings result.
func proMap(t *testing.T, got map[string]any) map[string]any {
	t.Helper()
	require.NotNil(t, got, "expected a non-nil result")
	pro, ok := got["pro"].(map[string]any)
	require.True(t, ok, "expected result[\"pro\"] to be a map, got %T", got["pro"])
	return pro
}

// TestExtractProSettings verifies that the upload payload collapses the enabled
// hierarchy metadata.enabled > pro.enabled > drift_detection.enabled so the
// values Atmos Pro persists already reflect any outer disable.
func TestExtractProSettings(t *testing.T) {
	testCases := []struct {
		name          string
		settings      map[string]any
		metadata      map[string]any
		expectNil     bool
		expectEnabled bool
		// expectDrift is only asserted when the source had a drift_detection block.
		hasDrift    bool
		expectDrift bool
	}{
		{
			// The reported bug case: component disabled upstream via
			// metadata.enabled, but pro.enabled + drift both true.
			name: "metadata disabled forces pro and drift off",
			settings: map[string]any{"pro": map[string]any{
				"enabled":         true,
				"drift_detection": map[string]any{"enabled": true},
			}},
			metadata:      map[string]any{"enabled": false},
			expectEnabled: false,
			hasDrift:      true,
			expectDrift:   false,
		},
		{
			// metadata disabled, no explicit pro.enabled (defaults true), drift on.
			name: "metadata disabled with implicit pro.enabled and drift on",
			settings: map[string]any{"pro": map[string]any{
				"drift_detection": map[string]any{"enabled": true},
			}},
			metadata:      map[string]any{"enabled": false},
			expectEnabled: false,
			hasDrift:      true,
			expectDrift:   false,
		},
		{
			name: "enabled component keeps pro and drift on",
			settings: map[string]any{"pro": map[string]any{
				"enabled":         true,
				"drift_detection": map[string]any{"enabled": true},
			}},
			metadata:      map[string]any{"enabled": true},
			expectEnabled: true,
			hasDrift:      true,
			expectDrift:   true,
		},
		{
			name: "no metadata defaults to enabled",
			settings: map[string]any{"pro": map[string]any{
				"enabled":         true,
				"drift_detection": map[string]any{"enabled": true},
			}},
			metadata:      nil,
			expectEnabled: true,
			hasDrift:      true,
			expectDrift:   true,
		},
		{
			name: "pro disabled forces drift off",
			settings: map[string]any{"pro": map[string]any{
				"enabled":         false,
				"drift_detection": map[string]any{"enabled": true},
			}},
			metadata:      nil,
			expectEnabled: false,
			hasDrift:      true,
			expectDrift:   false,
		},
		{
			name: "implicit pro.enabled defaults true",
			settings: map[string]any{"pro": map[string]any{
				"drift_detection": map[string]any{"enabled": false},
			}},
			metadata:      nil,
			expectEnabled: true,
			hasDrift:      true,
			expectDrift:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractProSettings(nil, tc.settings, tc.metadata)
			pro := proMap(t, got)

			enabled, ok := pro["enabled"].(bool)
			require.True(t, ok, "expected pro.enabled to be a bool")
			assert.Equal(t, tc.expectEnabled, enabled, "pro.enabled")

			if tc.hasDrift {
				drift, ok := pro["drift_detection"].(map[string]any)
				require.True(t, ok, "expected drift_detection block to be preserved")
				driftEnabled, ok := drift["enabled"].(bool)
				require.True(t, ok, "expected drift_detection.enabled to be a bool")
				assert.Equal(t, tc.expectDrift, driftEnabled, "drift_detection.enabled")
			}
		})
	}
}

// TestExtractProSettingsEdgeCases covers nil/absent/malformed inputs.
func TestExtractProSettingsEdgeCases(t *testing.T) {
	t.Run("nil settings returns nil", func(t *testing.T) {
		assert.Nil(t, extractProSettings(nil, nil, nil))
	})

	t.Run("no pro key returns nil", func(t *testing.T) {
		assert.Nil(t, extractProSettings(nil, map[string]any{"spacelift": map[string]any{}}, nil))
	})

	t.Run("non-map pro is treated as absent", func(t *testing.T) {
		// A non-map "pro" value (malformed config, e.g. a stray string) can't be
		// resolved to a pro section with an enabled/drift_detection state, so
		// pro.ResolveSection treats it the same as absent rather than passing
		// the garbage value through unvalidated.
		got := extractProSettings(nil, map[string]any{"pro": "invalid"}, nil)
		assert.Nil(t, got)
	})

	t.Run("no drift_detection block is not synthesized", func(t *testing.T) {
		got := extractProSettings(nil, map[string]any{"pro": map[string]any{"enabled": true}}, nil)
		pro := proMap(t, got)
		_, hasDrift := pro["drift_detection"]
		assert.False(t, hasDrift, "drift_detection should not be synthesized when absent")
	})
}

// TestProSectionPrecedence verifies the top-level `pro:` component section takes whole-block
// precedence over the deprecated `settings.pro:` alias, that either alone still resolves
// correctly (backward compatibility), and that both directions of isolation hold: the result
// reflects only the winning section and mutating one input doesn't leak into the other's
// effective state.
func TestProSectionPrecedence(t *testing.T) {
	t.Run("only top-level pro: is used when settings.pro: is absent", func(t *testing.T) {
		instance := &schema.Instance{
			Pro:      map[string]any{"enabled": true},
			Settings: map[string]any{},
		}
		assert.True(t, isProEnabled(instance))
	})

	t.Run("only settings.pro: is used when top-level pro: is absent (legacy)", func(t *testing.T) {
		instance := &schema.Instance{
			Settings: map[string]any{"pro": map[string]any{"enabled": true}},
		}
		assert.True(t, isProEnabled(instance))
	})

	t.Run("both set: top-level pro: wins, whole block, not merged", func(t *testing.T) {
		instance := &schema.Instance{
			// Top-level says disabled.
			Pro: map[string]any{"enabled": false},
			// Legacy says enabled with drift on -- must be fully ignored, not merged in.
			Settings: map[string]any{"pro": map[string]any{
				"enabled":         true,
				"drift_detection": map[string]any{"enabled": true},
			}},
		}
		assert.False(t, isProEnabled(instance), "top-level pro: must win outright")
		assert.False(t, isDriftEnabled(instance), "legacy drift_detection must not leak through when top-level pro: wins")
	})

	t.Run("both set: extractProSettings upload payload reflects only the winning top-level section", func(t *testing.T) {
		got := extractProSettings(
			map[string]any{"enabled": true},
			map[string]any{"pro": map[string]any{"enabled": false, "drift_detection": map[string]any{"enabled": true}}},
			nil,
		)
		pro := proMap(t, got)
		assert.Equal(t, true, pro["enabled"], "upload payload must reflect the winning top-level pro:, not the legacy settings.pro:")
		_, hasDrift := pro["drift_detection"]
		assert.False(t, hasDrift, "drift_detection from the losing legacy section must not appear in the payload")
	})

	t.Run("mutating settings after resolution does not affect an already-resolved top-level result", func(t *testing.T) {
		settings := map[string]any{"pro": map[string]any{"enabled": true}}
		instance := &schema.Instance{
			Pro:      map[string]any{"enabled": true},
			Settings: settings,
		}
		assert.True(t, isProEnabled(instance))

		// Mutate the legacy settings after the fact -- since top-level pro: won, this
		// must have no effect on the instance's resolved state.
		settings["pro"].(map[string]any)["enabled"] = false
		assert.True(t, isProEnabled(instance), "resolved state must not be affected by later mutation of the losing legacy section")
	})
}

// TestExtractProSettingsIsolation verifies the result is a copy: mutating it does
// not affect the source instance, and the source is not aliased into the result.
func TestExtractProSettingsIsolation(t *testing.T) {
	settings := map[string]any{"pro": map[string]any{
		"enabled":         true,
		"drift_detection": map[string]any{"enabled": true},
	}}
	metadata := map[string]any{"enabled": false}

	got := extractProSettings(nil, settings, metadata)
	pro := proMap(t, got)

	// The collapse must not have mutated the source settings.
	srcPro := settings["pro"].(map[string]any)
	assert.Equal(t, true, srcPro["enabled"], "source pro.enabled must be untouched")
	srcDrift := srcPro["drift_detection"].(map[string]any)
	assert.Equal(t, true, srcDrift["enabled"], "source drift_detection.enabled must be untouched")

	// Mutating the result must not reach back into the source.
	pro["enabled"] = "mutated"
	pro["drift_detection"].(map[string]any)["enabled"] = "mutated"
	assert.Equal(t, true, srcPro["enabled"], "mutating result must not affect source pro.enabled")
	assert.Equal(t, true, srcDrift["enabled"], "mutating result must not affect source drift_detection.enabled")

	// Mutating the source after extraction must not affect the already-returned result.
	srcPro["enabled"] = "changed-after"
	assert.Equal(t, "mutated", pro["enabled"], "result must be independent of later source mutation")

	// Same reverse-direction check for the nested drift_detection block: mutating
	// the source drift map must not reach into the already-returned result.
	resultDrift := pro["drift_detection"].(map[string]any)
	srcDrift["enabled"] = "changed-after"
	assert.Equal(t, "mutated", resultDrift["enabled"], "result drift_detection must be independent of later source mutation")
}

// TestMetadataDisabledPro verifies detection of the metadata-caused collapse used
// for the upload debug log: it is true only when the pro block would be enabled
// but metadata.enabled is explicitly false.
func TestMetadataDisabledPro(t *testing.T) {
	testCases := []struct {
		name     string
		settings map[string]any
		metadata map[string]any
		expected bool
	}{
		{
			name:     "metadata false squashes enabled pro",
			settings: map[string]any{"pro": map[string]any{"enabled": true}},
			metadata: map[string]any{"enabled": false},
			expected: true,
		},
		{
			name:     "metadata false squashes defaulted pro (drift_detection only)",
			settings: map[string]any{"pro": map[string]any{"drift_detection": map[string]any{"enabled": true}}},
			metadata: map[string]any{"enabled": false},
			expected: true,
		},
		{
			name:     "YAML-shaped pro block still detected",
			settings: map[string]any{"pro": map[interface{}]interface{}{"enabled": true}},
			metadata: map[string]any{"enabled": false},
			expected: true,
		},
		{
			name:     "metadata enabled is not a squash",
			settings: map[string]any{"pro": map[string]any{"enabled": true}},
			metadata: map[string]any{"enabled": true},
			expected: false,
		},
		{
			name:     "no metadata is not a squash",
			settings: map[string]any{"pro": map[string]any{"enabled": true}},
			metadata: nil,
			expected: false,
		},
		{
			// pro.enabled is already false, so metadata is not the cause.
			name:     "pro already disabled is not a metadata squash",
			settings: map[string]any{"pro": map[string]any{"enabled": false}},
			metadata: map[string]any{"enabled": false},
			expected: false,
		},
		{
			name:     "no pro block is not a squash",
			settings: map[string]any{},
			metadata: map[string]any{"enabled": false},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, metadataDisabledPro(nil, tc.settings, tc.metadata))
		})
	}
}
