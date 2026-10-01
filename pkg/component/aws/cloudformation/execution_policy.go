package cloudformation

import (
	"context"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveExecutionPolicy loads only the configured policy for a named execution.
// The reviewed changeset already stores its template; a local template is unnecessary.
func resolveExecutionPolicy(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, spec *stackSpec) (*stackSpec, error) {
	componentPath, err := resolveComponentPath(atmosConfig, info)
	if err != nil {
		return nil, err
	}
	componentPath, _, err = provisionAndResolveComponentPath(ctx, provisioner.OutputWriters{}, atmosConfig, info, cfg.CloudFormationComponentType, componentPath)
	if err != nil {
		return nil, err
	}
	spec.StackPolicyBody, err = loadStackPolicyBody(componentPath, spec)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
