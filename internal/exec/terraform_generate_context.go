package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// refreshTerraformGeneratorContext derives output identity after configuration
// values have resolved, preserving the component identity supplied by the generator.
func refreshTerraformGeneratorContext(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, stackFileName string) error {
	vars, _ := info.ComponentSection[cfg.VarsSectionName].(map[string]any)
	context := cfg.GetContextFromVars(vars)
	context.Component = info.Context.Component
	context.ComponentPath = info.Context.ComponentPath

	var stackName string
	var err error
	if config.Stacks.NameTemplate != "" {
		stackName, err = ProcessTmpl(config, "terraform-generate-stack-name", config.Stacks.NameTemplate,
			info.ComponentSection, config.Templates.Settings.IgnoreMissingTemplateValues)
	} else {
		stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(config), stackFileName)
	}
	if err != nil {
		return err
	}
	info.ComponentVarsSection = vars
	info.Context = context
	info.Stack = stackName
	info.ComponentSection["atmos_stack"] = stackName
	info.ComponentSection["stack"] = stackName
	return nil
}
