package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDetectDeletedComponents_CloudFormation(t *testing.T) {
	t.Parallel()

	remoteStacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				cfg.CloudFormationComponentType: map[string]any{
					"network": map[string]any{
						"vars": map[string]any{"stage": "dev"},
					},
				},
			},
		},
	}

	for _, tt := range []struct {
		name         string
		current      map[string]any
		deletionType string
		reason       string
	}{
		{
			name: "component deleted",
			current: map[string]any{
				"dev": map[string]any{"components": map[string]any{}},
			},
			deletionType: deletionTypeComponent,
			reason:       affectedReasonDeleted,
		},
		{
			name: "components section removed",
			current: map[string]any{
				"dev": map[string]any{},
			},
			deletionType: deletionTypeComponent,
			reason:       affectedReasonDeleted,
		},
		{
			name:         "entire stack deleted",
			current:      map[string]any{},
			deletionType: deletionTypeStack,
			reason:       affectedReasonDeletedStack,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			deleted, err := detectDeletedComponents(&remoteStacks, &tt.current, &schema.AtmosConfiguration{}, "", AffectedFilter{})
			require.NoError(t, err)
			require.Len(t, deleted, 1)
			assert.Equal(t, cfg.CloudFormationComponentType, deleted[0].ComponentType)
			assert.Equal(t, "network", deleted[0].Component)
			assert.Equal(t, "dev", deleted[0].Stack)
			assert.Equal(t, tt.deletionType, deleted[0].DeletionType)
			assert.Equal(t, tt.reason, deleted[0].Affected)
			assert.True(t, deleted[0].Deleted)
		})
	}
}
