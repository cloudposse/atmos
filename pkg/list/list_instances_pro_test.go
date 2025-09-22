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
