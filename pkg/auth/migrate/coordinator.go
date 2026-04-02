package migrate

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Coordinator orchestrates migration steps through detect, plan, confirm, and apply phases.
type Coordinator struct {
	steps    []MigrationStep
	prompter Prompter
}

// NewCoordinator creates a new migration coordinator.
func NewCoordinator(steps []MigrationStep, prompter Prompter) *Coordinator {
	return &Coordinator{steps: steps, prompter: prompter}
}

// Run executes the full migration flow.
func (c *Coordinator) Run(ctx context.Context, dryRun, force bool) error {
	defer perf.Track(nil, "migrate.Coordinator.Run")()

	// Phase 1: Detect.
	results, err := c.detect(ctx)
	if err != nil {
		return err
	}

	// Phase 2: Plan.
	needed := c.filterNeeded(results)
	if len(needed) == 0 {
		c.printStatusTable(results)
		ui.Success("Already migrated to AWS SSO auth.")
		return nil
	}

	if err := c.plan(ctx, needed); err != nil {
		return err
	}
	c.printPlan(results, needed)

	// Dry-run stops here.
	if dryRun {
		ui.Info("Dry run complete. No changes were made.")
		return nil
	}

	// Phase 3: Confirm and apply.
	if force {
		return c.applyAll(ctx, needed)
	}
	return c.confirmAndApply(ctx, needed)
}

// detect runs Detect on each step and returns the results.
func (c *Coordinator) detect(ctx context.Context) ([]StepResult, error) {
	results := make([]StepResult, 0, len(c.steps))
	for _, step := range c.steps {
		status, err := step.Detect(ctx)
		if err != nil {
			return nil, fmt.Errorf("detecting step %q: %w", step.Name(), err)
		}
		results = append(results, StepResult{
			Step:   step,
			Status: status,
		})
	}
	return results, nil
}

// filterNeeded returns results where Status is StepNeeded.
func (c *Coordinator) filterNeeded(results []StepResult) []StepResult {
	var needed []StepResult
	for _, r := range results {
		if r.Status == StepNeeded {
			needed = append(needed, r)
		}
	}
	return needed
}

// plan runs Plan on each needed step and populates Changes.
func (c *Coordinator) plan(ctx context.Context, needed []StepResult) error {
	for i := range needed {
		changes, err := needed[i].Step.Plan(ctx)
		if err != nil {
			return fmt.Errorf("planning step %q: %w", needed[i].Step.Name(), err)
		}
		needed[i].Changes = changes
	}
	return nil
}

// printStatusTable prints the status of all steps.
func (c *Coordinator) printStatusTable(results []StepResult) {
	ui.Writeln("")
	ui.Writeln("Migration Status:")
	for _, r := range results {
		ui.Writef("  %s: %s\n", r.Step.Description(), r.Status)
	}
	ui.Writeln("")
}

// printPlan prints the planned changes summary.
func (c *Coordinator) printPlan(allResults []StepResult, needed []StepResult) {
	ui.Writeln("")
	ui.Writeln("Migration Plan:")
	for _, r := range allResults {
		if r.Status == StepNeeded {
			ui.Writef("  [CHANGE] %s\n", r.Step.Description())
		} else {
			ui.Writef("  [OK]     %s (%s)\n", r.Step.Description(), r.Status)
		}
	}
	ui.Writeln("")
	ui.Writef("  %d step(s) to apply.\n", len(needed))
	ui.Writeln("")
}

// applyAll applies all steps without confirmation.
func (c *Coordinator) applyAll(ctx context.Context, needed []StepResult) error {
	for _, r := range needed {
		ui.Infof("Applying: %s", r.Step.Description())
		if err := r.Step.Apply(ctx); err != nil {
			return fmt.Errorf("applying step %q: %w", r.Step.Name(), errUtils.ErrMigrationStepFailed)
		}
	}
	c.printSummary()
	return nil
}

// confirmAndApply prompts the user for an action, then applies accordingly.
func (c *Coordinator) confirmAndApply(ctx context.Context, needed []StepResult) error {
	action, err := c.prompter.SelectAction()
	if err != nil {
		return fmt.Errorf("selecting action: %w", err)
	}

	switch action {
	case ActionApplyAll:
		return c.applyAll(ctx, needed)
	case ActionStepByStep:
		return c.applyStepByStep(ctx, needed)
	case ActionCancel:
		return errUtils.ErrMigrationAborted
	default:
		return errUtils.ErrMigrationAborted
	}
}

// applyStepByStep walks through each step with per-step yes/no confirmation.
func (c *Coordinator) applyStepByStep(ctx context.Context, needed []StepResult) error {
	for _, r := range needed {
		title := fmt.Sprintf("Apply step %q?", r.Step.Description())
		confirmed, err := c.prompter.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirming step %q: %w", r.Step.Name(), err)
		}
		if !confirmed {
			ui.Infof("Skipping: %s", r.Step.Description())
			continue
		}
		ui.Infof("Applying: %s", r.Step.Description())
		if err := r.Step.Apply(ctx); err != nil {
			return fmt.Errorf("applying step %q: %w", r.Step.Name(), errUtils.ErrMigrationStepFailed)
		}
	}
	c.printSummary()
	return nil
}

// printSummary prints next steps after migration.
func (c *Coordinator) printSummary() {
	ui.Writeln("")
	ui.Success("Migration complete.")
	ui.Writeln("")
	ui.Writeln("Next steps:")
	ui.Writeln("  1. Review the changes made to your configuration files.")
	ui.Writeln("  2. Run 'atmos auth login' to authenticate with AWS SSO.")
	ui.Writeln("  3. Run 'atmos terraform plan' to verify your infrastructure.")
	ui.Writeln("")
}
