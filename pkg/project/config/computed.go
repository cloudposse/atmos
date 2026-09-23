package config

import (
	"fmt"
	"strings"

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

// valueReferencesAnswer reports whether a computed field's Value expression
// textually references answers.<name> as a token -- the same
// token-scanning approach pkg/condition's celMentionsIdentifier uses for
// CEL When expressions, adapted to the "answers.<name>" shape a Go-template
// Value expression uses instead of CEL's bare "<name>". A plain
// substring/token check is safe here specifically because a computed
// field's own name is never ambiguous the way a general answers.* dot-path
// reference can be (see validateFieldOptionsSource's doc comment on why
// options: dot-paths deliberately skip this check): the full set of
// declared computed field names is always statically known at load time,
// with no --set-only or spec.values-only namesake to confuse it with.
func valueReferencesAnswer(expr, name string) bool {
	prefix := "answers." + name
	for _, token := range strings.FieldsFunc(expr, isNotIdentifierRune) {
		if token == prefix || strings.HasPrefix(token, prefix+".") {
			return true
		}
	}
	return false
}

// isNotIdentifierRune reports whether r can't be part of a dotted
// identifier token (the same delimiter rule pkg/condition's
// celMentionsIdentifier uses), extracted to its own function so the
// FieldsFunc closure doesn't inflate valueReferencesAnswer's own
// cyclomatic complexity.
func isNotIdentifierRune(r rune) bool {
	return r != '_' &&
		r != '.' &&
		(r < '0' || r > '9') &&
		(r < 'A' || r > 'Z') &&
		(r < 'a' || r > 'z')
}

// validateComputedFieldOrdering statically rejects a computed field whose
// Value expression references itself or a computed field declared after it
// in spec.fields[] -- ComputeFields evaluates computed fields once, in
// declaration order, so such a reference would otherwise silently resolve
// to a missing map key (nil) at render time instead of erroring, and a nil
// interpolated directly into file content renders as the literal string
// "<no value>" rather than failing loudly. Referencing an earlier-declared
// computed field, or any regular field regardless of order, is unaffected
// -- see ComputeFields' own doc comment for why those are always safe.
func validateComputedFieldOrdering(fields []FieldDefinition) error {
	for i := range fields {
		field := &fields[i]
		if field.Type != fieldTypeComputed {
			continue
		}
		for j := i; j < len(fields); j++ {
			later := &fields[j]
			if later.Type != fieldTypeComputed {
				continue
			}
			if !valueReferencesAnswer(field.Value, later.Name) {
				continue
			}
			if j == i {
				return errUtils.Build(errUtils.ErrScaffoldComputedFieldInvalid).
					WithExplanationf("Field %q references itself in its own `value:` expression", field.Name).
					WithHint("A computed field can't reference its own not-yet-computed value; remove the self-reference").
					WithContext("field_name", field.Name).
					WithExitCode(2).
					Err()
			}
			return errUtils.Build(errUtils.ErrScaffoldComputedFieldInvalid).
				WithExplanationf("Field %q references computed field %q, which is declared after it", field.Name, later.Name).
				WithHintf("Declare %q before %q -- a computed field can only reference an earlier-declared computed field", later.Name, field.Name).
				WithContext("field_name", field.Name).
				WithContext("referenced_field", later.Name).
				WithExitCode(2).
				Err()
		}
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
