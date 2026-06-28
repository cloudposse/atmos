package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/junit"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	junitStepType       = "junit"
	junitActionSummary  = "summary"
	junitActionAnnotate = "annotate"
	junitActionAll      = "all"
)

// CI seams — replaced in tests so the step can be exercised without a CI provider.
var (
	writeStepSummaryFn = ci.WriteStepSummary
	annotateFn         = ci.Annotate
)

// JUnitHandler ingests JUnit XML files and surfaces them in CI: it renders a
// markdown step summary and emits inline annotations for failing/errored tests.
// It works with any test runner that produces JUnit (terraform/opentofu test,
// pytest, go-test→junit, …), usable from a workflow, custom command, or
// `kind: step` lifecycle hook.
type JUnitHandler struct {
	BaseHandler
}

func init() {
	Register(&JUnitHandler{
		BaseHandler: NewBaseHandler(junitStepType, CategoryOutput, false),
	})
}

// junitStepAction returns the step action, defaulting to "all".
func junitStepAction(step *schema.WorkflowStep) string {
	action := strings.TrimSpace(step.Action)
	if action == "" {
		return junitActionAll
	}
	return action
}

// Validate checks junit step configuration.
func (h *JUnitHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.JUnitHandler.Validate")()

	if len(step.Files) == 0 {
		return errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("field", "files").
			WithExplanation("A junit step must set `files` to one or more globs of JUnit XML files").
			Err()
	}

	switch junitStepAction(step) {
	case junitActionSummary, junitActionAnnotate, junitActionAll:
		return nil
	default:
		return errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("field", "action").
			WithContext("value", step.Action).
			WithExplanation("Action must be `summary`, `annotate`, or `all`").
			Err()
	}
}

// Execute parses the JUnit files, writes a CI step summary, and emits
// annotations for failures.
func (h *JUnitHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.JUnitHandler.Execute")()

	if err := h.Validate(step); err != nil {
		return nil, err
	}

	report, fileCount, err := h.loadReport(step, vars)
	if err != nil {
		return nil, err
	}
	report.Aggregate()

	action := junitStepAction(step)
	if action == junitActionSummary || action == junitActionAll {
		h.writeSummary(step, &report)
	}
	if action == junitActionAnnotate || action == junitActionAll {
		emitAnnotations(&report)
	}

	// Always print a concise result line so local (non-CI) runs see something.
	passed := report.Tests - report.Failures - report.Errors - report.Skipped
	ui.Writef("JUnit: %d tests, %d passed, %d failed, %d errored, %d skipped (%d file(s))\n",
		report.Tests, passed, report.Failures, report.Errors, report.Skipped, fileCount)

	return NewStepResult(fmt.Sprintf("%d passed, %d failed", passed, report.Failures+report.Errors)), nil
}

// loadReport resolves the file globs, parses each JUnit file, and merges them
// into a single report.
func (h *JUnitHandler) loadReport(step *schema.WorkflowStep, vars *Variables) (junit.Report, int, error) {
	var merged junit.Report
	fileCount := 0

	for _, pattern := range step.Files {
		resolved, err := vars.Resolve(pattern)
		if err != nil {
			return junit.Report{}, 0, fmt.Errorf("step '%s': failed to resolve files pattern %q: %w", step.Name, pattern, err)
		}
		matches, err := filepath.Glob(resolved)
		if err != nil {
			return junit.Report{}, 0, fmt.Errorf("step '%s': invalid files pattern %q: %w", step.Name, resolved, err)
		}
		for _, path := range matches {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return junit.Report{}, 0, fmt.Errorf("step '%s': failed to read JUnit file %q: %w", step.Name, path, readErr)
			}
			report, parseErr := junit.Parse(data)
			if parseErr != nil {
				return junit.Report{}, 0, fmt.Errorf("step '%s': %q: %w", step.Name, path, parseErr)
			}
			merged.Suites = append(merged.Suites, report.Suites...)
			fileCount++
		}
	}

	if fileCount == 0 {
		return junit.Report{}, 0, errUtils.Build(errUtils.ErrStepNoFilesFound).
			WithContext("step", step.Name).
			WithContext("files", strings.Join(step.Files, ", ")).
			WithExplanation("No JUnit XML files matched the `files` globs").
			Err()
	}
	return merged, fileCount, nil
}

// writeSummary renders the report as markdown and appends it to the CI step
// summary (no-op outside CI).
func (h *JUnitHandler) writeSummary(step *schema.WorkflowStep, report *junit.Report) {
	title := step.Title
	if title == "" {
		title = "Test results"
	}
	md := junit.Markdown(report, junit.Options{Title: title})
	if err := writeStepSummaryFn("\n" + md); err != nil {
		log.Debug("junit step: failed to write CI step summary", "error", err)
	}
}

// emitAnnotations renders one CI annotation per failing/errored test (no-op
// outside CI or when the provider lacks annotation support).
func emitAnnotations(report *junit.Report) {
	failed := report.FailedCases()
	if len(failed) == 0 {
		return
	}
	annotations := make([]ci.Annotation, 0, len(failed))
	for _, f := range failed {
		annotations = append(annotations, ci.Annotation{
			Path:      f.File,
			StartLine: f.Line,
			Level:     ci.AnnotationError,
			Title:     fmt.Sprintf("test: %s", f.Name),
			Message:   f.Message,
		})
	}
	if err := annotateFn(annotations); err != nil {
		log.Debug("junit step: failed to emit CI annotations", "error", err)
	}
}
