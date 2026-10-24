package step

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

const scriptTemplateField = "script"

// literalHintFields are the string fields whose body commonly contains a template opening (`{{` by
// default) that is not meant as a template, so a render failure there gets the `!literal` hint.
var literalHintFields = map[string]bool{scriptTemplateField: true, "command": true}

// TemplateFieldError reports that rendering a step field as a template failed. The message
// names the step and the field and, for a script read from a file with !include, the source file,
// so the failure points at the text to fix instead of at an anonymous template pass. A script or
// command body that contains `{{` also gets a hint to mark the field `!literal`. This form assumes
// the default delimiters; use Variables.TemplateFieldError when the step templates may use others.
func TemplateFieldError(step *schema.WorkflowStep, field string, cause error) error {
	defer perf.Track(nil, "step.TemplateFieldError")()

	return TemplateFieldErrorWithDelimiters(delimiters.Default(), step, field, cause)
}

// TemplateFieldError reports a failed field render like the package-level TemplateFieldError, but
// for the delimiter pair these variables render with, so the `!literal` hint names and detects the
// configured template opening.
func (v *Variables) TemplateFieldError(step *schema.WorkflowStep, field string, cause error) error {
	defer perf.Track(nil, "step.Variables.TemplateFieldError")()

	return TemplateFieldErrorWithDelimiters(v.TemplateDelimiters(), step, field, cause)
}

// TemplateFieldErrorWithDelimiters is TemplateFieldError for an explicit delimiter pair, for hosts
// that render step fields without a Variables value.
func TemplateFieldErrorWithDelimiters(delims delimiters.Delimiters, step *schema.WorkflowStep, field string, cause error) error {
	defer perf.Track(nil, "step.TemplateFieldErrorWithDelimiters")()

	includedScript := field == scriptTemplateField && step.ScriptSource != ""
	where := fmt.Sprintf("step %q field %s", step.Name, field)
	if includedScript {
		where += fmt.Sprintf(" (%s)", step.ScriptSource)
	}
	builder := errUtils.Build(errUtils.ErrTemplateEvaluation).
		WithCause(fmt.Errorf("%s: %w", where, cause)).
		WithContext("step", step.Name).
		WithContext("field", field)
	if includedScript {
		builder = builder.WithContext("source", step.ScriptSource)
	}
	if literalHintFields[field] && !step.IsLiteral(field) && fieldHasTemplateOpen(delims, step, field) {
		if includedScript {
			// The YAML tag cannot be written inside the included file, so name what works there.
			builder = builder.WithHintf("If the script contains `%s` that is not a template expression, move that text into a `load()`ed module, or include the file with `!include.raw` so it is used exactly as written.", delims.Normalize().Left)
		} else {
			builder = builder.WithHintf("If the %s contains `%s` that is not a template expression, write the field with the `!literal` YAML tag so it is used exactly as written.", field, delims.Normalize().Left)
		}
	}
	return builder.Err()
}

func fieldHasTemplateOpen(delims delimiters.Delimiters, step *schema.WorkflowStep, field string) bool {
	switch field {
	case scriptTemplateField:
		return delims.Contains(step.Script)
	case "command":
		return delims.Contains(step.Command)
	default:
		return false
	}
}
