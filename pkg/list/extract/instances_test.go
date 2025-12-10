package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstances(t *testing.T) {
	testCases := []struct {
		name      string
		instances []schema.Instance
		expected  []map[string]any
	}{
		{
			name:      "empty instances",
			instances: []schema.Instance{},
			expected:  []map[string]any{},
		},
		{
			name: "single instance",
			instances: []schema.Instance{
				{
					Component:     "vpc",
					Stack:         "plat-ue2-dev",
					ComponentType: "terraform",
					Vars: map[string]any{
						"tenant":      "plat",
						"environment": "ue2",
						"stage":       "dev",
					},
					Settings: map[string]any{},
					Env:      map[string]any{},
					Backend:  map[string]any{},
					Metadata: map[string]any{},
				},
			},
			expected: []map[string]any{
				{
					"component":      "vpc",
					"stack":          "plat-ue2-dev",
					"component_type": "terraform",
					"vars": map[string]any{
						"tenant":      "plat",
						"environment": "ue2",
						"stage":       "dev",
					},
					"settings": map[string]any{},
					"env":      map[string]any{},
					"backend":  map[string]any{},
					"metadata": map[string]any{},
				},
			},
		},
		{
			name: "multiple instances",
			instances: []schema.Instance{
				{
					Component:     "vpc",
					Stack:         "plat-ue2-dev",
					ComponentType: "terraform",
					Vars: map[string]any{
						"tenant": "plat",
						"stage":  "dev",
					},
					Settings: map[string]any{},
					Env:      map[string]any{},
					Backend:  map[string]any{},
					Metadata: map[string]any{},
				},
				{
					Component:     "eks",
					Stack:         "plat-ue2-prod",
					ComponentType: "terraform",
					Vars: map[string]any{
						"tenant": "plat",
						"stage":  "prod",
					},
					Settings: map[string]any{},
					Env:      map[string]any{},
					Backend:  map[string]any{},
					Metadata: map[string]any{},
				},
			},
			expected: []map[string]any{
				{
					"component":      "vpc",
					"stack":          "plat-ue2-dev",
					"component_type": "terraform",
					"vars": map[string]any{
						"tenant": "plat",
						"stage":  "dev",
					},
					"settings": map[string]any{},
					"env":      map[string]any{},
					"backend":  map[string]any{},
					"metadata": map[string]any{},
				},
				{
					"component":      "eks",
					"stack":          "plat-ue2-prod",
					"component_type": "terraform",
					"vars": map[string]any{
						"tenant": "plat",
						"stage":  "prod",
					},
					"settings": map[string]any{},
					"env":      map[string]any{},
					"backend":  map[string]any{},
					"metadata": map[string]any{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := Instances(tc.instances)
			assert.Equal(t, tc.expected, result)
		})
	}
}
