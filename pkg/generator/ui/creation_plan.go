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

	return ui.planCreation(template, target, defaults, values, false)
}

// PlanCreationQuiet is PlanCreation without the dry-run report: it prints no per-file
// "(would create)" lines, no "Would generate N files." summary, and no README, so a caller that
// reports its own result (such as `atmos stack create`) does not show the preview as if it were
// the outcome. The configuration summary shown after the questionnaire is unaffected.
func (ui *InitUI) PlanCreationQuiet(template *templates.Configuration, target string, defaults bool, values map[string]any) (*CreationPlan, error) {
	defer perf.Track(nil, "ui.InitUI.PlanCreationQuiet")()

	return ui.planCreation(template, target, defaults, values, true)
}

// planCreation renders the plan; quiet suppresses the dry-run report (see PlanCreationQuiet).
func (ui *InitUI) planCreation(template *templates.Configuration, target string, defaults bool, values map[string]any, quiet bool) (*CreationPlan, error) {
	plan := &CreationPlan{}
	oldDry, oldPreview, oldPlan, oldQuiet := ui.processor.DryRun, ui.processor.PreviewFile, ui.creationPlan, ui.quietPlan
	ui.processor.DryRun, ui.creationPlan, ui.quietPlan = true, plan, quiet
	ui.processor.PreviewFile = func(file engine.File) { plan.Files = append(plan.Files, file) }
	defer func() {
		ui.processor.DryRun, ui.processor.PreviewFile, ui.creationPlan, ui.quietPlan = oldDry, oldPreview, oldPlan, oldQuiet
	}()
	if err := ui.executeWithSetup(template, target, false, false, defaults, "", values, nil); err != nil {
		return nil, err
	}
	return plan, nil
}
