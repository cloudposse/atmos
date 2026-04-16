package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFilterProEnabledInstances ensures only instances with settings.pro.enabled == true are returned.
func TestFilterProEnabledInstances(t *testing.T) {
	instances := []schema.Instance{
		{
			Component: "vpc",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"enabled": true,
				},
			},
		},
		{
			Component: "app",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			Component: "db",
			Stack:     "stack1",
			Settings:  map[string]interface{}{},
		},
		{
			// drift_detection.enabled is no longer a gate; only pro.enabled matters.
			Component: "pro-with-drift-off",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"enabled":         true,
					"drift_detection": map[string]interface{}{"enabled": false},
				},
			},
		},
	}

	filtered := filterProEnabledInstances(instances)

	assert.Len(t, filtered, 2)
	assert.Equal(t, "vpc", filtered[0].Component)
	assert.Equal(t, "pro-with-drift-off", filtered[1].Component)
}

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

// TestFilterProEnabledInstancesEmpty tests filtering with no instances.
func TestFilterProEnabledInstancesEmpty(t *testing.T) {
	t.Run("empty input returns empty output", func(t *testing.T) {
		instances := []schema.Instance{}
		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})

	t.Run("no pro-enabled instances returns empty output", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "dev",
				Settings:  map[string]interface{}{},
			},
		}
		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})
}
