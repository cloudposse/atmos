package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFailureError preserves a failure already displayed by a completed test report.
// Callers can avoid an additional error box while retaining failure policies.
//
// Err holds every leaf failure joined together so errors.Is and errors.As keep matching them. The
// report already shows each leaf error, so Error returns a one-line summary built from Failed and
// Total instead of repeating the joined text.
type TestFailureError struct {
	Err    error
	Failed int
	Total  int
}

// Error returns a summary such as "2 of 7 tests failed". Failures that happen before any leaf ran
// (for example while preparing the test environment) have no failed leaves, so the underlying
// message is kept.
func (e *TestFailureError) Error() string {
	defer perf.Track(nil, "step.TestFailureError.Error")()

	if e.Failed <= 0 {
		if e.Err == nil {
			return errUtils.ErrTestsFailed.Error()
		}
		return e.Err.Error()
	}
	noun := "tests"
	if e.Total == 1 {
		noun = "test"
	}
	return fmt.Sprintf("%d of %d %s failed", e.Failed, e.Total, noun)
}

// Unwrap preserves error matching through the displayed-failure marker.
func (e *TestFailureError) Unwrap() error {
	defer perf.Track(nil, "step.TestFailureError.Unwrap")()
	return e.Err
}

// Is reports whether target is ErrTestsFailed.
func (e *TestFailureError) Is(target error) bool {
	defer perf.Track(nil, "step.TestFailureError.Is")()
	return target == errUtils.ErrTestsFailed
}

// TestRunner connects the shared registry to the workflow scheduler.
type TestRunner interface {
	RunTest(context.Context, *schema.WorkflowStep, *Variables, *schema.WorkflowDefinition) (*StepResult, error)
}

var testRunner TestRunner

// RegisterTestRunner installs the test execution bridge at startup.
func RegisterTestRunner(r TestRunner) {
	defer perf.Track(nil, "step.RegisterTestRunner")()
	testRunner = r
}

// TestHandler runs a group of checks with failure-only output.
type TestHandler struct{ BaseHandler }

func init() {
	Register(&TestHandler{BaseHandler: NewBaseHandler(schema.TaskTypeTest, CategoryCommand, false)})
}

// Validate checks the group and rejects children that cannot be captured safely.
func (h *TestHandler) Validate(s *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.TestHandler.Validate")()

	if err := schema.ValidateTestStep(s); err != nil {
		return err
	}
	return validateTestHandlers(s.Steps)
}

// validateTestHandlers rejects unregistered and interactive handlers before running any test.
func validateTestHandlers(steps []schema.WorkflowStep) error {
	for i := range steps {
		s := &steps[i]
		if s.Type == schema.TaskTypeParallel || s.Type == schema.TaskTypeMatrix {
			if err := validateTestHandlers(s.Steps); err != nil {
				return err
			}
			continue
		}
		kind := s.Type
		if kind == "" {
			kind = schema.TaskTypeShell
		}
		handler, ok := Get(kind)
		if !ok {
			return fmt.Errorf("%w: unknown test step type %q", schema.ErrWorkflowControlStepInvalid, kind)
		}
		if handler.RequiresTTY() {
			return fmt.Errorf("%w: interactive step %q cannot run inside test", schema.ErrWorkflowControlStepInvalid, s.Name)
		}
	}
	return nil
}

// Execute runs a test group through the registered bridge.
func (h *TestHandler) Execute(ctx context.Context, s *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.TestHandler.Execute")()

	return h.ExecuteWithWorkflow(ctx, s, vars, nil)
}

// ExecuteWithWorkflow preserves the caller's environment and working directory defaults.
func (h *TestHandler) ExecuteWithWorkflow(ctx context.Context, s *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.TestHandler.ExecuteWithWorkflow")()

	if err := h.Validate(s); err != nil {
		return nil, err
	}
	if testRunner == nil {
		return nil, fmt.Errorf("%w: test runner is not registered", schema.ErrWorkflowControlStepInvalid)
	}
	return testRunner.RunTest(ctx, s, vars, workflow)
}
