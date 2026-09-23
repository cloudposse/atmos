package config

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ComputedFieldRenderer resolves a computed field's Value expression against
// answers, returning its decoded value of arbitrary shape (string, bool,
// number, list, or map). Supplied by
// engine.Processor.RenderAnswersExpression; duplicated here rather than
// importing pkg/generator/engine, for the same import-cycle reason
// FieldOptionsRenderer is (see its own doc comment).
type ComputedFieldRenderer func(expr string, answers map[string]interface{}, delimiters []string) (any, error)

// ComputeFields evaluates every type: computed field's Value expression, in
// the order fields are declared in spec.fields, and writes each result into
// values under the field's own name. A computed field may reference any
// regular field's answer (all of those are already collected by the time
// ComputeFields runs, whether prompted, --set, or defaulted) and any
// earlier-declared computed field's own result -- a later computed field
// sees it in values because each result is written back before the next
// field is evaluated. A field whose When evaluates false against the
// values collected so far is skipped entirely (left unset), matching how a
// hidden regular field is never prompted for either.
func ComputeFields(scaffoldConfig *ScaffoldConfig, values map[string]interface{}, render ComputedFieldRenderer) error {
	defer perf.Track(nil, "config.ComputeFields")()

	delimiters := defaultDelimiters(scaffoldConfig.Spec.Delimiters)

	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Type != fieldTypeComputed {
			continue
		}
		if !field.When.Evaluate(condition.Context{Answers: values}) {
			continue
		}
		if render == nil {
			return errUtils.Build(errUtils.ErrScaffoldComputedFieldInvalid).
				WithExplanationf("Field %q is `type: computed` but no expression renderer is available", field.Name).
				WithHint("This is an Atmos bug: ComputeFields was called without a ComputedFieldRenderer").
				WithContext("field_name", field.Name).
				WithExitCode(2).
				Err()
		}

		value, err := render(field.Value, values, delimiters)
		if err != nil {
			return fmt.Errorf("computed field %q: %w", field.Name, err)
		}
		values[field.Name] = value
	}
	return nil
}

// RejectComputedFieldOverrides returns an error if overrides (the --set
// flags supplied on the command line) supplies a value for any type:
// computed field. Computed fields are always derived by ComputeFields;
// surfacing a clear error here -- rather than letting ComputeFields silently
// overwrite the --set value later -- avoids a confusing "I set it but it
// didn't take" experience.
func RejectComputedFieldOverrides(scaffoldConfig *ScaffoldConfig, overrides map[string]interface{}) error {
	defer perf.Track(nil, "config.RejectComputedFieldOverrides")()

	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Type != fieldTypeComputed {
			continue
		}
		if _, exists := overrides[field.Name]; !exists {
			continue
		}
		return errUtils.Build(errUtils.ErrScaffoldComputedFieldNotSettable).
			WithExplanationf("Field %q is `type: computed` and cannot be set with --set", field.Name).
			WithHintf("Remove `--set %s=...`; its value is always derived from other answers", field.Name).
			WithContext("field_name", field.Name).
			WithExitCode(2).
			Err()
	}
	return nil
}
