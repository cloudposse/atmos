package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetPackerTemplateFromSettings returns a Packer template name from the `settings.packer.template` section in the Atmos component manifest.
func GetPackerTemplateFromSettings(settings *schema.AtmosSectionMapType) (string, error) {
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
