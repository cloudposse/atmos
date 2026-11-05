package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAddCloudFormationSectionAffected_SharedSections(t *testing.T) {
	t.Parallel()
	sections := []struct{ name, reason string }{
		{sectionNameVars, affectedReasonStackVars},
		{sectionNameEnv, affectedReasonStackEnv},
		{sectionNameSource, affectedReasonStackSource},
		{sectionNameProvision, affectedReasonStackProvision},
	}
	for _, section := range sections {
		for _, change := range []string{"changed", "added", "removed", "unchanged", "absent"} {
			t.Run(section.name+"/"+change, func(t *testing.T) {
				t.Parallel()
				local := map[string]any{}
				remote := map[string]any{}
				if change != "removed" && change != "absent" {
					local[section.name] = map[string]any{"version": "v2"}
				}
				if change != "added" && change != "absent" {
					remote[section.name] = map[string]any{"version": "v1"}
				}
				if change == "unchanged" {
					remote[section.name] = local[section.name]
				}
				remoteStacks := cfnRemoteStacksWith(remote)
				currentStacks := cfnRemoteStacksWith(local)
				var affected []schema.Affected
				err := addCloudFormationSectionAffected(&affected, cfnAtmosConfig(), cfnTestComponent, cfnTestStack,
					&local, &remoteStacks, &currentStacks, false, false)
				require.NoError(t, err)
				if change == "unchanged" || change == "absent" {
					assert.Empty(t, affected)
					return
				}
				require.Len(t, affected, 1)
				assert.Equal(t, cfnTestComponent, affected[0].Component)
				assert.Equal(t, cfg.CloudFormationComponentType, affected[0].ComponentType)
				assert.Equal(t, section.reason, affected[0].Affected)
			})
		}
	}
}
