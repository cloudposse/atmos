package step

import (
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// starlarkTypeHint corrects the common mistake of using `starlark` as a step type.
const starlarkTypeHint = "Use `type: script` with `interpreter: starlark`."

// RegisteredStepTypes returns every step type a workflow or custom command may use, sorted: the
// registered handler names plus the types the executors run directly (shell, atmos, exec, and the
// control steps).
func RegisteredStepTypes() []string {
	defer perf.Track(nil, "step.RegisteredStepTypes")()

	seen := map[string]bool{
		schema.TaskTypeShell: true, schema.TaskTypeAtmos: true, schema.TaskTypeExec: true,
		schema.TaskTypeScript: true, schema.TaskTypeParallel: true, schema.TaskTypeMatrix: true,
	}
	for name := range List() {
		seen[name] = true
	}
	types := make([]string, 0, len(seen))
	for name := range seen {
		types = append(types, name)
	}
	sort.Strings(types)
	return types
}

// UnsupportedStepTypeError builds the error for a step whose type is not known. The owner is the
// place the step is in ("workflow \"deploy\"" or "custom command \"build\""); the step is named
// rather than numbered, and the explanation lists the registered types. A `starlark` type, the
// usual typo for a script step, gets a hint with the correct spelling.
func UnsupportedStepTypeError(owner, stepName, stepType string) error {
	defer perf.Track(nil, "step.UnsupportedStepTypeError")()

	return UnsupportedStepTypeBuilder(owner, stepName, stepType).Err()
}

// UnsupportedStepTypeBuilder is UnsupportedStepTypeError before Err is called, so a caller can add
// its own title, context, or exit code.
func UnsupportedStepTypeBuilder(owner, stepName, stepType string) *errUtils.ErrorBuilder {
	defer perf.Track(nil, "step.UnsupportedStepTypeBuilder")()

	name := strings.TrimSpace(stepName)
	if name == "" {
		name = "(unnamed)"
	}
	builder := errUtils.Build(errUtils.ErrInvalidWorkflowStepType).
		WithExplanationf("Step `%s` of %s uses unsupported type `%s`. Registered step types: %s.",
			name, owner, stepType, strings.Join(RegisteredStepTypes(), ", ")).
		WithContext("step", name).
		WithContext("type", stepType)
	if strings.EqualFold(strings.TrimSpace(stepType), "starlark") {
		builder = builder.WithHint(starlarkTypeHint)
	}
	return builder
}
