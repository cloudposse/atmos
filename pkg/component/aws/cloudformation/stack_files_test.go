package cloudformation

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveSpecAndTemplate composes preparation and loading for file-resolution tests.
func resolveSpecAndTemplate(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation) (*stackSpec, error) {
	spec, path, err := prepareStackSpec(ctx, config, info, operation)
	if err != nil {
		return nil, err
	}
	if err := loadStackFiles(path, spec, operation); err != nil {
		return nil, err
	}
	return spec, nil
}
