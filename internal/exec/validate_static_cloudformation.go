package exec

import (
	"context"

	cfnmanifest "github.com/cloudposse/atmos/pkg/component/aws/cloudformation/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StaticCloudFormationSettings resolves regions and timeouts through the production import and
// inheritance pipeline without evaluating templates, authenticating, or calling cloud APIs.
func StaticCloudFormationSettings(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) (map[string]any, error) {
	defer perf.Track(config, "exec.StaticCloudFormationSettings")()

	stacks, err := LoadStaticStacks(ctx, config, inputs)
	if err != nil {
		return nil, err
	}
	return cfnmanifest.SettingsByStack(stacks), nil
}
