package cloudformation

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveExecutionPolicy loads only the configured policy for a named execution.
// The reviewed changeset already stores its template; a local template is unnecessary.
func resolveExecutionPolicy(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, spec *stackSpec) (*stackSpec, error) {
	componentPath, err := prepareComponentFiles(ctx, atmosConfig, info)
	if err != nil {
		return nil, err
	}
	spec.StackPolicyBody, err = loadStackPolicyBody(componentPath, spec)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
