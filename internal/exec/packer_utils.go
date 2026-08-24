package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// checkPackerConfig validates the packer configuration.
func checkPackerConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.checkPackerConfig")()

	if len(atmosConfig.Components.Packer.BasePath) < 1 {
		return fmt.Errorf("%w: must be provided in 'components.packer.base_path' config or 'ATMOS_COMPONENTS_PACKER_BASE_PATH' ENV variable",
			errUtils.ErrMissingPackerBasePath)
	}

	return nil
}

// GetPackerTemplateFromSettings returns a Packer template name from the `settings.packer.template` section in the Atmos component manifest.
func GetPackerTemplateFromSettings(settings *schema.AtmosSectionMapType) (string, error) {
	defer perf.Track(nil, "exec.GetPackerTemplateFromSettings")()

	if settings == nil {
		return "", nil
	}

	var packerSection schema.AtmosSectionMapType
	var packerTemplate string
	var ok bool

	if packerSection, ok = (*settings)[cfg.PackerSectionName].(map[string]any); !ok {
		return "", nil
	}
	if packerTemplate, ok = packerSection[cfg.PackerTemplateSectionName].(string); !ok {
		return "", nil
	}
	return packerTemplate, nil
}

// GetPackerManifestFromVars returns the Packer manifest filename from the `vars.manifest_file_name`.
func GetPackerManifestFromVars(vars *schema.AtmosSectionMapType) (string, error) {
	defer perf.Track(nil, "exec.GetPackerManifestFromVars")()

	if vars == nil {
		return "", nil
	}

	var packerManifest string
	var ok bool

	if packerManifest, ok = (*vars)["manifest_file_name"].(string); !ok {
		return "", nil
	}
	return packerManifest, nil
}
