package ui

import (
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// CreationPlan is a fully rendered additive generation, with no hooks executed
// and no files written. Each caller validates its domain-specific outputs before
// applying Files. Existing project answers never leak into a new generation.
type CreationPlan struct {
	Files  []engine.File
	Config *config.ScaffoldConfig
	Values map[string]any
}

// PlanCreation uses the same questionnaire, conditions, matrices, delimiters and
// renderer as init/scaffold, allowing callers to validate all files before writing.
func (ui *InitUI) PlanCreation(template *templates.Configuration, target string, defaults bool, values map[string]any) (*CreationPlan, error) {
	defer perf.Track(nil, "ui.InitUI.PlanCreation")()

	plan := &CreationPlan{}
	oldDry, oldPreview, oldPlan := ui.processor.DryRun, ui.processor.PreviewFile, ui.creationPlan
	ui.processor.DryRun, ui.creationPlan = true, plan
	ui.processor.PreviewFile = func(file engine.File) { plan.Files = append(plan.Files, file) }
	defer func() { ui.processor.DryRun, ui.processor.PreviewFile, ui.creationPlan = oldDry, oldPreview, oldPlan }()
	if err := ui.executeWithSetup(template, target, false, false, defaults, "", values, nil); err != nil {
		return nil, err
	}
	return plan, nil
}
