package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindAffected(t *testing.T) {
	tests := []struct {
		name                        string
		currentStacks               map[string]any
		remoteStacks                map[string]any
		atmosConfig                 *schema.AtmosConfiguration
		changedFiles                []string
		includeSpaceliftAdminStacks bool
		includeSettings             bool
		stackToFilter               string
		expectedAffected            []schema.Affected
		expectedError               bool
	}{
		{
			name:             "Empty stacks should return empty affected list",
			currentStacks:    map[string]any{},
			remoteStacks:     map[string]any{},
			atmosConfig:      &schema.AtmosConfiguration{},
			changedFiles:     []string{},
			expectedAffected: []schema.Affected{},
			expectedError:    false,
		},
		{
			name: "Stack filter should only process specified stack",
			currentStacks: map[string]any{
				"stack1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
				"stack2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			remoteStacks:     map[string]any{},
			atmosConfig:      &schema.AtmosConfiguration{},
			changedFiles:     []string{},
			stackToFilter:    "stack1",
			expectedAffected: []schema.Affected{},
			expectedError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				tt.atmosConfig,
				tt.changedFiles,
				tt.includeSpaceliftAdminStacks,
				tt.includeSettings,
				tt.stackToFilter,
			)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAffected, affected)
			}
		})
	}
}
