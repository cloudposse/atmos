package rules

import (
	"fmt"
	"reflect"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l10EnvShadowingRule detects env vars that appear at multiple levels (stack-level and component-level)
// with different values.
type l10EnvShadowingRule struct{}

func newL10EnvShadowingRule() lint.LintRule {
	return &l10EnvShadowingRule{}
}

func (r *l10EnvShadowingRule) ID() string   { return "L-10" }
func (r *l10EnvShadowingRule) Name() string { return "Env Var Shadowing" }
func (r *l10EnvShadowingRule) Description() string {
	return "Detects environment variables that appear at both stack-level and component-level with different values, creating confusion about which value takes effect."
}
func (r *l10EnvShadowingRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l10EnvShadowingRule) AutoFixable() bool       { return false }

func (r *l10EnvShadowingRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	var findings []lint.LintFinding

	for stackName, stackSection := range ctx.StacksMap {
		stackMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}

		// Get stack-level env.
		stackEnv, _ := stackMap[cfg.EnvSectionName].(map[string]any)

		componentsSection, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}

		for _, compType := range []string{cfg.TerraformSectionName, cfg.HelmfileSectionName} {
			compSection, ok := componentsSection[compType].(map[string]any)
			if !ok {
				continue
			}

			for compName, compData := range compSection {
				compMap, ok := compData.(map[string]any)
				if !ok {
					continue
				}
				compEnv, ok := compMap[cfg.EnvSectionName].(map[string]any)
				if !ok || len(compEnv) == 0 {
					continue
				}

				// Check for shadowing: same key in stack env and component env with different values.
				for envKey, compVal := range compEnv {
					stackVal, inStack := stackEnv[envKey]
					if !inStack {
						continue
					}
					if !reflect.DeepEqual(stackVal, compVal) {
						findings = append(findings, lint.LintFinding{
							RuleID:    r.ID(),
							Severity:  r.Severity(),
							Message:   fmt.Sprintf("Env var '%s' in component '%s' (stack '%s') shadows the stack-level value: component='%v', stack='%v'", envKey, compName, stackName, compVal, stackVal),
							Component: compName,
							Stack:     stackName,
							FixHint:   fmt.Sprintf("Remove '%s' from the stack-level env or the component-level env to resolve the ambiguity", envKey),
						})
					}
				}
			}
		}
	}

	return findings, nil
}
