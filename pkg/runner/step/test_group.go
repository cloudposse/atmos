package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFailureError preserves a failure already displayed by a completed test report.
// Callers can avoid an additional error box while retaining failure policies.
type TestFailureError struct{ Err error }

// Error returns the underlying failure already shown by the test reporter.
func (e *TestFailureError) Error() string {
	defer perf.Track(nil, "step.TestFailureError.Error")()
	return e.Err.Error()
}

// Unwrap preserves error matching through the displayed-failure marker.
func (e *TestFailureError) Unwrap() error {
	defer perf.Track(nil, "step.TestFailureError.Unwrap")()
	return e.Err
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
