package step

import (
	"errors"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateOutput rejects an `output` mode that does not exist, so a typo such as
// `output: capture` fails loudly instead of silently behaving like another mode. Handlers of
// command-running step types call it from Validate.
func (h BaseHandler) ValidateOutput(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.BaseHandler.ValidateOutput")()

	err := schema.ValidateStepOutput(step)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errUtils.ErrStepInvalidOutputMode) {
		return err
	}
	return errUtils.Build(errUtils.ErrStepInvalidOutputMode).
		WithExplanationf("Step '%s' sets output '%s'.", step.Name, strings.TrimSpace(step.Output)).
		WithHintf("Valid output modes: %s.", strings.Join(schema.StepOutputModes(), ", ")).
		WithContext("step", step.Name).
		WithContext("output", step.Output).
		Err()
}
