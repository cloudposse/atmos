package exec

import (
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BuildAtlantisProjectName builds an Atlantis project name from the provided context and project name pattern
func BuildAtlantisProjectName(context schema.Context, projectNameTemplate string) string {
	return cfg.ReplaceContextTokens(context, projectNameTemplate)
}

// BuildAtlantisProjectNameFromComponentConfig builds an Atlantis project name from the component config
func BuildAtlantisProjectNameFromComponentConfig(
	cliConfig schema.CliConfiguration,
	componentName string,
	componentSettingsSection map[any]any,
	componentVarsSection map[any]any,
) (string, error) {

	var atlantisProjectTemplate schema.AtlantisProjectConfig
	var atlantisProjectName string

	if atlantisSettingsSection, ok := componentSettingsSection["atlantis"].(map[any]any); ok {
		// 'settings.atlantis.project_template' has higher priority than 'settings.atlantis.project_template_name'
		if atlantisSettingsProjectTemplate, ok := atlantisSettingsSection["project_template"].(map[any]any); ok {
			err := mapstructure.Decode(atlantisSettingsProjectTemplate, &atlantisProjectTemplate)
			if err != nil {
				return "", err
			}
		} else if atlantisSettingsProjectTemplateName, ok := atlantisSettingsSection["project_template_name"].(string); ok && atlantisSettingsProjectTemplateName != "" {
			if pt, ok := cliConfig.Integrations.Atlantis.ProjectTemplates[atlantisSettingsProjectTemplateName]; ok {
				atlantisProjectTemplate = pt
			}
		}

		context := cfg.GetContextFromVars(componentVarsSection)
		context.Component = strings.Replace(componentName, "/", "-", -1)

		// If Atlantis project template is defined and has a name, replace tokens in the name and add the Atlantis project to the output
		if !reflect.ValueOf(atlantisProjectTemplate).IsZero() && atlantisProjectTemplate.Name != "" {
			atlantisProjectName = BuildAtlantisProjectName(context, atlantisProjectTemplate.Name)
		}
	}

	return atlantisProjectName, nil
}
