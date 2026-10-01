package cloudformation

import (
	"maps"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateDryRun validates known values without evaluating deferred expressions.
// Copies are used only for static validation; no placeholder reaches an API.
func validateDryRun(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, flags map[string]any, operation Operation) error {
	section := maps.Clone(info.ComponentSection)
	if body, ok := section["template"].(string); ok && deferredExpression(atmosConfig, body) {
		section["template"] = "Resources: {}"
	}
	if err := validateComponentConfig(section); err != nil {
		return err
	}
	if _, err := buildStackSpec(section); err != nil {
		return err
	}
	if operation == OperationStackSetCreate || operation == OperationStackSetUpdate {
		provision := staticStackSetProvision(atmosConfig, section)
		target, _ := flags["target"].(string)
		if _, err := resolveStackSetTarget(provision, target); err != nil {
			return err
		}
	}
	ui.Info("Dry-run: static configuration validated; templates, YAML functions and remote validation are deferred.")
	return nil
}

// deferredExpression recognizes configured template delimiters and Atmos YAML tags without parsing or
// evaluating their contents.
func deferredExpression(config *schema.AtmosConfiguration, value string) bool {
	left, right := "{{", "}}"
	if config != nil && len(config.Templates.Settings.Delimiters) == 2 {
		left, right = config.Templates.Settings.Delimiters[0], config.Templates.Settings.Delimiters[1]
	}
	// Recognize syntax without parsing functions or evaluating external reads.
	if left != "" && right != "" && strings.Contains(value, left) && strings.Contains(value, right) {
		return true
	}
	value = strings.TrimSpace(value)
	for _, tag := range u.AtmosYamlTags {
		if value == tag || strings.HasPrefix(value, tag+" ") {
			return true
		}
	}
	return false
}

// staticTargetValues omits deferred target expressions from dry-run validation while retaining static
// values, including malformed ones.
func staticTargetValues(config *schema.AtmosConfiguration, value any) any {
	if text, ok := value.(string); ok && deferredExpression(config, text) {
		return nil
	}
	if items, ok := value.([]any); ok {
		result := make([]any, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok && deferredExpression(config, text) {
				continue
			}
			result = append(result, item)
		}
		return result
	}
	return value
}

// staticStackSetProvision copies target maps before removing deferred account and region expressions,
// preserving the caller's configuration.
func staticStackSetProvision(atmosConfig *schema.AtmosConfiguration, section map[string]any) map[string]any {
	provision, _ := section["provision"].(map[string]any)
	rawTargets, _ := provision["targets"].(map[string]any)
	targets := maps.Clone(rawTargets)
	for name, raw := range targets {
		target, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		target = maps.Clone(target)
		for _, field := range []string{"accounts", "regions"} {
			target[field] = staticTargetValues(atmosConfig, target[field])
		}
		targets[name] = target
	}
	return map[string]any{"targets": targets}
}
