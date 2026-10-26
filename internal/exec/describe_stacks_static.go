package exec

import (
	"bytes"
	"fmt"
	"slices"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// findDescribeStacksMap keeps static inspection isolated from manifest maps cached by evaluated callers.
//
//nolint:revive // Return shape intentionally mirrors FindStacksMap.
func findDescribeStacksMap(config *schema.AtmosConfiguration, ignoreMissing, staticOnly bool) (map[string]any, map[string]map[string]any, AllStacksDeferredContexts, error) {
	if !staticOnly {
		return FindStacksMap(config, ignoreMissing)
	}
	_, stacks, raw, deferred, err := processYAMLConfigFilesWithOptions(config, config.StacksBaseAbsolutePath,
		config.TerraformDirAbsolutePath, config.HelmfileDirAbsolutePath, config.PackerDirAbsolutePath,
		config.AnsibleDirAbsolutePath, config.StackConfigFilesAbsolutePaths, false, true, ignoreMissing, &StackLoadOptions{SkipTemplates: true, StaticOnly: true})
	return stacks, raw, deferred, err
}

func (p *describeStacksProcessor) resolveStackName(file, manifest string, info schema.ConfigAndStacksInfo, vars map[string]any) (string, schema.Context, error) { //nolint:gocritic // Mirrors the existing read-only name resolver.
	if !p.staticOnly || manifest != "" || p.atmosConfig.Stacks.NameTemplate == "" {
		return ResolveStackName(p.atmosConfig, file, manifest, info, vars)
	}
	// A static name template may read already merged fields and use pure Go built-ins.
	// Atmos/gomplate functions are intentionally unavailable because they can perform I/O.
	delims, err := delimiters.Parse(p.atmosConfig.Templates.Settings.Delimiters)
	if err != nil {
		return "", schema.Context{}, err
	}
	tmpl, err := template.New("static-stack-name").Delims(delims.Left, delims.Right).
		Option("missingkey=error").Parse(p.atmosConfig.Stacks.NameTemplate)
	if err != nil {
		return "", schema.Context{}, staticStackNameError(file, err)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, info.ComponentSection); err != nil {
		return "", schema.Context{}, staticStackNameError(file, err)
	}
	return output.String(), schema.Context{}, nil
}

func staticStackNameError(file string, err error) error {
	return fmt.Errorf("%w: cannot determine stack name for %q without evaluating dynamic expressions: %w", errUtils.ErrTemplateEvaluation, file, err)
}

// hasRequestedComponents avoids inventing a global name when component overrides determine real stack names.
func (p *describeStacksProcessor) hasRequestedComponents(stack map[string]any) bool {
	components, _ := stack["components"].(map[string]any)
	for kind, section := range components {
		if len(p.componentTypes) > 0 && !slices.Contains(p.componentTypes, kind) {
			continue
		}
		if values, ok := section.(map[string]any); ok && len(values) > 0 {
			return true
		}
	}
	return false
}

func (p *describeStacksProcessor) includeStaticEmptyStack(file string, stack map[string]any) error {
	vars, _ := stack["vars"].(map[string]any)
	name, _, err := p.resolveStackName(file, "", schema.ConfigAndStacksInfo{ComponentSection: stack}, vars)
	if err != nil {
		return err
	}
	if !shouldFilterByStack(p.filterByStack, file, name) {
		if _, exists := p.finalStacksMap[name]; !exists {
			p.finalStacksMap[name] = map[string]any{"components": map[string]any{}}
		}
	}
	return nil
}
