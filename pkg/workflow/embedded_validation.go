package workflow

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	_ "github.com/cloudposse/atmos/pkg/script/starlark" // Register the embedded interpreter.
)

const embeddedValidationTitle = "Embedded interpreter cannot run in a container"

// ValidateEmbeddedInterpreters rejects, before any step runs, a workflow whose steps would
// run an embedded interpreter (such as Starlark) under an enabled container. Embedded
// interpreters run inside the Atmos process, so they cannot execute in a container; without
// this up-front check the failure only surfaces after earlier steps have already run.
func ValidateEmbeddedInterpreters(workflowName string, def *schema.WorkflowDefinition) error {
	defer perf.Track(nil, "workflow.ValidateEmbeddedInterpreters")()

	if def == nil {
		return nil
	}
	return ValidateEmbeddedInterpreterSteps(workflowName, def.Container, def.Steps)
}

// ValidateEmbeddedInterpreterSteps is the step-list form of ValidateEmbeddedInterpreters. The
// ambient container is the workflow-level container (nil for custom commands, which only have
// step-level containers). It recurses into parallel, matrix, and test children.
func ValidateEmbeddedInterpreterSteps(owner string, ambient *schema.WorkflowContainer, steps []schema.WorkflowStep) error {
	defer perf.Track(nil, "workflow.ValidateEmbeddedInterpreterSteps")()

	for i := range steps {
		step := &steps[i]
		if err := validateEmbeddedStep(owner, ambient, step, i); err != nil {
			return err
		}
		if err := ValidateEmbeddedInterpreterSteps(owner, ambient, step.Steps); err != nil {
			return err
		}
	}
	return nil
}

// validateEmbeddedStep checks a single step (children are handled by the caller).
func validateEmbeddedStep(owner string, ambient *schema.WorkflowContainer, step *schema.WorkflowStep, index int) error {
	if step.Type != schema.TaskTypeScript {
		return nil
	}
	interpreter := strings.TrimSpace(step.Interpreter)
	// Templated interpreters are only known at runtime, where the runtime checks remain.
	if interpreter == "" || strings.Contains(interpreter, "{{") {
		return nil
	}
	if _, embedded := script.Get(interpreter); !embedded {
		return nil
	}
	containerized := step.Container.IsEnabled() || (ambient.IsEnabled() && !StepContainerDisabled(step))
	if !containerized {
		return nil
	}

	name := strings.TrimSpace(step.Name)
	if name == "" {
		name = fmt.Sprintf("#%d", index+1)
	}
	cause := fmt.Errorf("%w: step %q uses embedded interpreter %q under a container", errUtils.ErrScript, name, interpreter)
	return errUtils.Build(cause).
		WithTitle(embeddedValidationTitle).
		WithExplanationf("%s step `%s` uses the embedded `%s` interpreter, which runs inside Atmos and cannot run in a container.", owner, name, interpreter).
		WithContext("owner", owner).
		WithContext("step", name).
		WithContext("interpreter", interpreter).
		WithHintf("Set `container: false` on step %s — embedded interpreters run inside Atmos.", name).
		WithExitCode(1).
		Err()
}

// rejectEmbeddedScriptInContainer fails a script step whose interpreter is a registered
// embedded interpreter, which cannot run inside a container. The step's Interpreter must
// already be rendered (see RenderScriptInterpreter) so a templated name cannot bypass the check.
func rejectEmbeddedScriptInContainer(step *schema.WorkflowStep) error {
	if step == nil || step.Type != schema.TaskTypeScript {
		return nil
	}
	if _, embedded := script.Get(step.Interpreter); !embedded {
		return nil
	}
	return fmt.Errorf("%w: embedded %s requires container: false", errUtils.ErrScript, strings.TrimSpace(step.Interpreter))
}

// RenderScriptInterpreter renders a templated interpreter on a script step in place, so the
// container checks and the container command see the effective interpreter instead of the raw
// template. The step must be a copy the caller owns. Non-script steps and plain interpreters
// are left untouched.
func RenderScriptInterpreter(step *schema.WorkflowStep, render func(string) (string, error)) error {
	defer perf.Track(nil, "workflow.RenderScriptInterpreter")()

	if step == nil || render == nil || step.Type != schema.TaskTypeScript || !strings.Contains(step.Interpreter, "{{") {
		return nil
	}
	rendered, err := render(step.Interpreter)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "interpreter").
			Err()
	}
	step.Interpreter = rendered
	return nil
}
