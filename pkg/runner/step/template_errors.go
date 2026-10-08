package step

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// literalHintFields are the string fields whose body commonly contains `{{` that is not meant
// as a template, so a render failure there gets the `!literal` hint.
var literalHintFields = map[string]bool{"script": true, "command": true}

// TemplateFieldError reports that rendering a step field as a template failed. The message
// names the step and the field and, for a script read from a file with !include, the source file,
// so the failure points at the text to fix instead of at an anonymous template pass. A script or
// command body that contains `{{` also gets a hint to mark the field `!literal`.
func TemplateFieldError(step *schema.WorkflowStep, field string, cause error) error {
	defer perf.Track(nil, "step.TemplateFieldError")()

	where := fmt.Sprintf("step %q field %s", step.Name, field)
	if field == "script" && step.ScriptSource != "" {
		where += fmt.Sprintf(" (%s)", step.ScriptSource)
	}
	builder := errUtils.Build(errUtils.ErrTemplateEvaluation).
		WithCause(fmt.Errorf("%s: %w", where, cause)).
		WithContext("step", step.Name).
		WithContext("field", field)
	if field == "script" && step.ScriptSource != "" {
		builder = builder.WithContext("source", step.ScriptSource)
	}
	switch {
	case field == "script" && step.ScriptSource != "" && !step.IsLiteral(field) && fieldHasTemplateOpen(step, field):
		// The YAML tag cannot be written inside the included file, so name what works there.
		builder = builder.WithHint("If the script contains `{{` that is not a template expression, move that text into a `load()`ed module, or include the file with `!include.raw` so it is used exactly as written.")
	case literalHintFields[field] && !step.IsLiteral(field) && fieldHasTemplateOpen(step, field):
		builder = builder.WithHintf("If the %s contains `{{` that is not a template expression, write the field with the `!literal` YAML tag so it is used exactly as written.", field)
	}
	return builder.Err()
}

func fieldHasTemplateOpen(step *schema.WorkflowStep, field string) bool {
	switch field {
	case "script":
		return strings.Contains(step.Script, templateOpenDelim)
	case "command":
		return strings.Contains(step.Command, templateOpenDelim)
	default:
		return false
	}
}
