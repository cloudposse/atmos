package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFilterProEnabledDeployments ensures only deployments with settings.pro.drift_detection.enabled == true are returned.
func TestFilterProEnabledDeployments(t *testing.T) {
	deployments := []schema.Deployment{
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

	filtered := filterProEnabledDeployments(deployments)

	assert.Len(t, filtered, 1)
	assert.Equal(t, "vpc", filtered[0].Component)
}
