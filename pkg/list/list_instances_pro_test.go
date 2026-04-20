package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
			name: "pro enabled missing (drift_detection alone is not enough)",
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
			expected: false,
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
			name: "enabled not a bool (string)",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": "true",
					},
				},
			},
			expected: false,
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
// "Disabled" covers both explicit `pro.enabled: false` and instances with no `pro` config.
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

	testCases := []struct {
		name             string
		instances        []schema.Instance
		expectedEnabled  int
		expectedDisabled int
	}{
		{
			name:             "empty slice",
			instances:        []schema.Instance{},
			expectedEnabled:  0,
			expectedDisabled: 0,
		},
		{
			name:             "all enabled",
			instances:        []schema.Instance{enabledInst, enabledInst, enabledInst},
			expectedEnabled:  3,
			expectedDisabled: 0,
		},
		{
			name:             "all explicitly disabled",
			instances:        []schema.Instance{disabledInst, disabledInst},
			expectedEnabled:  0,
			expectedDisabled: 2,
		},
		{
			name:             "no pro config counts as disabled",
			instances:        []schema.Instance{noProInst, noProInst},
			expectedEnabled:  0,
			expectedDisabled: 2,
		},
		{
			name:             "non-bool enabled counts as disabled (strict bool)",
			instances:        []schema.Instance{nonBoolEnabledInst},
			expectedEnabled:  0,
			expectedDisabled: 1,
		},
		{
			name:             "mixed enabled/disabled/no-pro",
			instances:        []schema.Instance{enabledInst, disabledInst, noProInst, enabledInst},
			expectedEnabled:  2,
			expectedDisabled: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enabled, disabled := countEnabledDisabled(tc.instances)
			assert.Equal(t, tc.expectedEnabled, enabled, "enabled count")
			assert.Equal(t, tc.expectedDisabled, disabled, "disabled count")
		})
	}
}
