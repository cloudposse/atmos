package exec

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BuildSpaceliftStackName builds a Spacelift stack name from the provided context and stack name pattern.
func BuildSpaceliftStackName(spaceliftSettings map[string]any, context schema.Context, contextPrefix string) (string, string, error) {
	defer perf.Track(nil, "exec.BuildSpaceliftStackName")()

	if spaceliftStackNamePattern, ok := spaceliftSettings["stack_name_pattern"].(string); ok {
		return cfg.ReplaceContextTokens(context, spaceliftStackNamePattern), spaceliftStackNamePattern, nil
	} else if spaceliftStackName, ok := spaceliftSettings["stack_name"].(string); ok {
		return spaceliftStackName, contextPrefix, nil
	} else {
		defaultSpaceliftStackNamePattern := fmt.Sprintf("%s-%s", contextPrefix, context.Component)
		return strings.Replace(defaultSpaceliftStackNamePattern, "/", "-", -1), contextPrefix, nil
	}
}

// SpaceliftStackNaming bundles the two (mutually preferred) stack-naming schemes so callers that
// need to resolve a stack's logical name don't have to pass them as separate positional arguments.
type SpaceliftStackNaming struct {
	// NameTemplate is `stacks.name_template` (Go template, recommended).
	NameTemplate string
	// NamePattern is `stacks.name_pattern` (deprecated, token-based). Only consulted when
	// NameTemplate is empty.
	NamePattern string
}

// ResolveSpaceliftContextPrefix computes the logical stack name (context prefix) used for
// Spacelift stack naming. Precedence: naming.NameTemplate (Go template, recommended) >
// naming.NamePattern (deprecated, token-based) > the raw stack name. Callers decide which of the
// two configured naming schemes (if any) applies — e.g. explicit-file-path invocations
// intentionally pass an empty SpaceliftStackNaming to preserve raw path-based naming.
func ResolveSpaceliftContextPrefix(
	atmosConfig *schema.AtmosConfiguration,
	stackName string,
	context *schema.Context,
	componentVars map[string]any,
	naming SpaceliftStackNaming,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ResolveSpaceliftContextPrefix")()

	switch {
	case naming.NameTemplate != "":
		return ProcessTmpl(
			atmosConfig,
			"spacelift-stack-name-template",
			naming.NameTemplate,
			map[string]any{cfg.VarsSectionName: componentVars},
			atmosConfig.Templates.Settings.IgnoreMissingTemplateValues,
		)
	case naming.NamePattern != "":
		return cfg.GetContextPrefix(stackName, *context, naming.NamePattern, stackName)
	default:
		return strings.ReplaceAll(stackName, "/", "-"), nil
	}
}

// buildSpaceliftStackNameForComponent resolves a single Terraform component's Spacelift stack name.
func buildSpaceliftStackNameForComponent(
	atmosConfig *schema.AtmosConfiguration,
	stackName string,
	component string,
	componentMap map[string]any,
	naming SpaceliftStackNaming,
) (string, error) {
	componentVars := map[string]any{}
	if i, ok := componentMap["vars"]; ok {
		componentVars = i.(map[string]any)
	}

	spaceliftSettings := map[string]any{}
	if i, ok := componentMap["settings"]; ok {
		componentSettings := i.(map[string]any)
		if i2, ok2 := componentSettings["spacelift"]; ok2 {
			spaceliftSettings = i2.(map[string]any)
		}
	}

	context := cfg.GetContextFromVars(componentVars)

	contextPrefix, err := ResolveSpaceliftContextPrefix(atmosConfig, stackName, &context, componentVars, naming)
	if err != nil {
		return "", err
	}

	context.Component = component

	spaceliftStackName, _, err := BuildSpaceliftStackName(spaceliftSettings, context, contextPrefix)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(spaceliftStackName, "/", "-"), nil
}

// BuildSpaceliftStackNames builds Spacelift stack names.
func BuildSpaceliftStackNames(atmosConfig *schema.AtmosConfiguration, stacks map[string]any, naming SpaceliftStackNaming) ([]string, error) {
	defer perf.Track(atmosConfig, "exec.BuildSpaceliftStackNames")()

	var allStackNames []string

	for stackName, stackConfig := range stacks {
		config := stackConfig.(map[string]any)

		i, ok := config["components"]
		if !ok {
			continue
		}
		componentsSection := i.(map[string]any)

		terraformComponents, ok := componentsSection["terraform"]
		if !ok {
			continue
		}
		terraformComponentsMap := terraformComponents.(map[string]any)

		for component, v := range terraformComponentsMap {
			spaceliftStackName, err := buildSpaceliftStackNameForComponent(atmosConfig, stackName, component, v.(map[string]any), naming)
			if err != nil {
				return nil, err
			}
			allStackNames = append(allStackNames, spaceliftStackName)
		}
	}

	return allStackNames, nil
}

// BuildSpaceliftStackNameFromComponentConfig builds Spacelift stack name from the component config.
func BuildSpaceliftStackNameFromComponentConfig(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.BuildSpaceliftStackNameFromComponentConfig")()

	var spaceliftStackName string
	var spaceliftSettingsSection map[string]any
	var contextPrefix string
	var err error

	if i, ok2 := configAndStacksInfo.ComponentSettingsSection["spacelift"]; ok2 {
		spaceliftSettingsSection = i.(map[string]any)
	}

	// Spacelift stack
	if spaceliftWorkspaceEnabled, ok := spaceliftSettingsSection["workspace_enabled"].(bool); ok && spaceliftWorkspaceEnabled {
		context := cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		context.Component = strings.Replace(configAndStacksInfo.ComponentFromArg, "/", "-", -1)

		if atmosConfig.Stacks.NameTemplate != "" {
			contextPrefix, err = ProcessTmpl(atmosConfig, "name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
			if err != nil {
				return "", err
			}
		} else {
			contextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack, context, GetStackNamePattern(atmosConfig), configAndStacksInfo.Stack)
			if err != nil {
				return "", err
			}
		}

		spaceliftStackName, _, err = BuildSpaceliftStackName(spaceliftSettingsSection, context, contextPrefix)
		if err != nil {
			return "", err
		}
	}

	return spaceliftStackName, nil
}
