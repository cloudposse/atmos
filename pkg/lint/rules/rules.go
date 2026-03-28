// Package rules provides all built-in lint rule implementations for atmos lint stacks.
// Each rule is in a separate file named lNN_<description>.go where NN is the rule number.
package rules

import "github.com/cloudposse/atmos/pkg/lint"

// All returns all built-in lint rules.
func All() []lint.LintRule {
	return []lint.LintRule{
		newL09CycleRule(),
		newL04AbstractLeakRule(),
		newL02RedundantOverrideRule(),
		newL01DeadVarRule(),
		newL03ImportDepthRule(),
		newL07OrphanedFileRule(),
		newL08SensitiveVarRule(),
		newL10EnvShadowingRule(),
		newL05CohesionRule(),
		newL06DRYRule(),
	}
}
