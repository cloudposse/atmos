package config

import (
	"fmt"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// decodeConfigForImports reads only the configuration needed to resolve imports.
// Other sections can still contain YAML functions returning maps or lists; decoding
// them into the full schema before preprocessing would prevent imports from loading.
// Keep authentication and downloader settings available without evaluating functions
// earlier than the normal configuration-loading phase.
func decodeConfigForImports(v *viper.Viper) (schema.AtmosConfiguration, error) {
	src := schema.AtmosConfiguration{BasePath: v.GetString("base_path")}
	// Remote import resolution can initialize the shared stack importer.
	src.Stacks.BasePath = v.GetString("stacks.base_path")
	var err error
	src.Import, err = NormalizeConfigImports(v.Get("import"))
	if err != nil {
		return src, err
	}
	if err = v.UnmarshalKey("imports", &src.Imports, atmosDecodeHook()); err != nil {
		return src, fmt.Errorf("%w: decode import options: %w", errUtils.ErrMergeConfiguration, err)
	}
	if err = v.UnmarshalKey("settings", &src.Settings, atmosDecodeHook()); err != nil {
		return src, fmt.Errorf("%w: decode import settings: %w", errUtils.ErrMergeConfiguration, err)
	}
	if err = v.UnmarshalKey("auth", &src.Auth, atmosDecodeHook()); err != nil {
		return src, fmt.Errorf("%w: decode import auth: %w", errUtils.ErrMergeConfiguration, err)
	}
	return src, nil
}
