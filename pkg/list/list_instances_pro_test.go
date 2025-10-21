package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFilterProEnabledInstances ensures only instances with settings.pro.drift_detection.enabled == true are returned.
func TestFilterProEnabledInstances(t *testing.T) {
	instances := []schema.Instance{
		{
			Component: "vpc",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"drift_detection": map[string]interface{}{"enabled": true},
				},
			},
		},
		{
			Component: "app",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"drift_detection": map[string]interface{}{"enabled": false},
				},
			},
		},
		{
			Component: "db",
			Stack:     "stack1",
			Settings:  map[string]interface{}{},
		},
		{
			Component: "disabled-pro",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"enabled":         false,
					"drift_detection": map[string]interface{}{"enabled": true},
				},
			},
		},
	}

	filtered := filterProEnabledInstances(instances)

	assert.Len(t, filtered, 1)
	assert.Equal(t, "vpc", filtered[0].Component)
}

// TestIsProDriftDetectionEnabled tests the drift detection check logic.
func TestIsProDriftDetectionEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		instance *schema.Instance
		expected bool
	}{
		{
			name: "drift detection enabled",
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
			name: "drift detection disabled",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pro explicitly disabled",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": false,
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
			name: "drift_detection not a map",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": "invalid",
					},
				},
			},
			expected: false,
		},
		{
			name: "enabled not a bool",
			instance: &schema.Instance{
				Component: "vpc",
				Stack:     "dev",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": "true",
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isProDriftDetectionEnabled(tc.instance)
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
