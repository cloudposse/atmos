package exec

import (
	"reflect"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"

	"github.com/go-viper/mapstructure/v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BuildAtlantisProjectNameFromComponentConfig builds an Atlantis project name from the component config.
func BuildAtlantisProjectNameFromComponentConfig(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.BuildAtlantisProjectNameFromComponentConfig")()

	var atlantisProjectTemplate schema.AtlantisProjectConfig
	var atlantisProjectName string

	if atlantisSettingsSection, ok := configAndStacksInfo.ComponentSettingsSection["atlantis"].(map[string]any); ok {
		// 'settings.atlantis.project_template' has higher priority than 'settings.atlantis.project_template_name'
		if atlantisSettingsProjectTemplate, ok := atlantisSettingsSection["project_template"].(map[string]any); ok {
			err := mapstructure.Decode(atlantisSettingsProjectTemplate, &atlantisProjectTemplate)
			if err != nil {
				return "", err
			}
		} else if atlantisSettingsProjectTemplateName, ok := atlantisSettingsSection["project_template_name"].(string); ok && atlantisSettingsProjectTemplateName != "" {
			if pt, ok := atmosConfig.Integrations.Atlantis.ProjectTemplates[atlantisSettingsProjectTemplateName]; ok {
				atlantisProjectTemplate = pt
			}
		}

		// If Atlantis project template is defined and has a name, replace tokens in the name and add the Atlantis project to the output
		if !reflect.ValueOf(atlantisProjectTemplate).IsZero() && atlantisProjectTemplate.Name != "" {
			context := cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
			context.Component = strings.Replace(configAndStacksInfo.ComponentFromArg, "/", "-", -1)
			atlantisProjectName = cfg.ReplaceContextTokens(context, atlantisProjectTemplate.Name)
		}
	}

	return atlantisProjectName, nil
}
