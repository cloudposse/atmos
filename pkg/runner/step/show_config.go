package step

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetShowConfig returns the effective show config for a step.
// Performs deep merge: step-level values override workflow-level values.
// Unset values (nil) default to false.
func GetShowConfig(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition) *schema.ShowConfig {
	defer perf.Track(nil, "step.GetShowConfig")()

	result := &schema.ShowConfig{}

	// Apply workflow-level defaults first.
	if workflow != nil && workflow.Show != nil {
		result = mergeShowConfig(result, workflow.Show)
	}

	// Apply step-level overrides (deep merge).
	if step != nil && step.Show != nil {
		result = mergeShowConfig(result, step.Show)
	}

	return result
}

// mergeShowConfig merges src into dst, with src values taking precedence.
// Only non-nil values from src override dst.
func mergeShowConfig(dst, src *schema.ShowConfig) *schema.ShowConfig {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = &schema.ShowConfig{}
	}

	if src.Header != nil {
		dst.Header = src.Header
	}
	if src.Flags != nil {
		dst.Flags = src.Flags
	}
	if src.Command != nil {
		dst.Command = src.Command
	}
	if src.Count != nil {
		dst.Count = src.Count
	}
	if src.Progress != nil {
		dst.Progress = src.Progress
	}

	return dst
}

// ShowHeader returns true if the header feature is enabled.
func ShowHeader(cfg *schema.ShowConfig) bool {
	defer perf.Track(nil, "step.ShowHeader")()

	return cfg != nil && cfg.Header != nil && *cfg.Header
}

// ShowFlags returns true if the flags feature is enabled.
func ShowFlags(cfg *schema.ShowConfig) bool {
	defer perf.Track(nil, "step.ShowFlags")()

	return cfg != nil && cfg.Flags != nil && *cfg.Flags
}

// ShowCommand returns true if the command feature is enabled.
func ShowCommand(cfg *schema.ShowConfig) bool {
	defer perf.Track(nil, "step.ShowCommand")()

	return cfg != nil && cfg.Command != nil && *cfg.Command
}

// ShowCount returns true if the count feature is enabled.
func ShowCount(cfg *schema.ShowConfig) bool {
	defer perf.Track(nil, "step.ShowCount")()

	return cfg != nil && cfg.Count != nil && *cfg.Count
}

// ShowProgress returns true if the progress feature is enabled.
func ShowProgress(cfg *schema.ShowConfig) bool {
	defer perf.Track(nil, "step.ShowProgress")()

	return cfg != nil && cfg.Progress != nil && *cfg.Progress
}

// BoolPtr is a helper to create a pointer to a bool value.
// Useful for setting ShowConfig fields in tests and configuration.
func BoolPtr(b bool) *bool {
	defer perf.Track(nil, "step.BoolPtr")()

	return &b
}
