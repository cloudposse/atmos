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

// ComputeFields evaluates every type: computed field's Value, in the order
// fields are declared in spec.fields, and writes each result into values
// under the field's own name. Value is either a template-expression string
// containing a template action under delimiters (rendered via render,
// resolved against answers -- see containsTemplateAction) or a literal:
// either a non-string (number, bool, list, map) or a plain string with no
// template action in it at all, stored as-is with no rendering. A computed
// field may reference any regular field's answer (all of those are already
// collected by the time ComputeFields runs, whether prompted, --set, or
// defaulted) and any earlier-declared computed field's own result -- a
// later computed field sees it in values because each result is written
// back before the next field is evaluated. A field whose When evaluates
// false against the values collected so far is skipped entirely (left
// unset, deleting any stale value a persisted record already carried for
// it), matching how a hidden regular field is never prompted for either.
func ComputeFields(scaffoldConfig *ScaffoldConfig, values map[string]interface{}, render ComputedFieldRenderer) error {
	defer perf.Track(nil, "config.ComputeFields")()

	delimiters := defaultDelimiters(scaffoldConfig.Spec.Delimiters)

	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Type != fieldTypeComputed {
			continue
		}
		if !field.When.Evaluate(condition.Context{Answers: values}) {
			// A skipped computed field is unset, full stop -- delete any
			// stale value a persisted record (loaded via LoadUserValues,
			// merged in before ComputeFields ever runs) already carried
			// for this field from a prior generation where When was still
			// true. Leaving it in place would let a since-invalidated
			// computed value keep reaching template rendering.
			delete(values, field.Name)
			continue
		}

		expr, isExpression := field.Value.(string)
		if !isExpression || !containsTemplateAction(expr, delimiters) {
			// Not a template-expression string -- an already-resolved
			// literal (hand-authored, or produced by a YAML function like
			// !include before this field was ever unmarshaled), or a plain
			// string with no template action in it at all (see
			// containsTemplateAction). Use it as-is; no renderer needed at
			// all for this field.
			values[field.Name] = field.Value
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

		value, err := render(expr, values, delimiters)
		if err != nil {
			return fmt.Errorf("computed field %q: %w", field.Name, err)
		}
		values[field.Name] = value
	}
	return nil
}

// containsTemplateAction reports whether expr contains a template action
// under delimiters -- the only signal available to distinguish a plain
// string literal ("hello") from a Go-template expression ("{{ \"hello\" }}")
// for a computed field's string-typed Value, since both decode to the same
// Go string type from YAML with nothing else to tell them apart. A string
// containing both the left and right delimiter is sent to render, which
// still enforces its own single-template-action shape (see
// engine.singleValuePipe) -- this only decides whether render is attempted
// at all, not what render accepts once it is.
func containsTemplateAction(expr string, delimiters []string) bool {
	return strings.Contains(expr, delimiters[0]) && strings.Contains(expr, delimiters[1])
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
	tokens := strings.FieldsFunc(expr, isNotIdentifierRune)
	for i, token := range tokens {
		if token == prefix || strings.HasPrefix(token, prefix+".") {
			return true
		}
		// Go templates invoke the zero-argument "answers" function when it's
		// passed to index or Sprig's get (e.g. {{ index answers "name" }},
		// {{ get answers "name" }}), reaching the same answers map as the
		// dotted answers.name form above -- but the token scan above never
		// sees them joined into one token, since the quotes around the
		// string argument split each into separate function/"answers"/name
		// tokens.
		if callsFuncOnAnswers(tokens, i, name, "index") || callsFuncOnAnswers(tokens, i, name, "get") {
			return true
		}
	}
	return false
}

// callsFuncOnAnswers reports whether tokens[i:i+3] is the token sequence
// funcName, "answers", name -- i.e. a Go-template function call of the
// shape {{ funcName answers "name" }}, once quotes have been stripped by
// the same token scan valueReferencesAnswer uses.
func callsFuncOnAnswers(tokens []string, i int, name, funcName string) bool {
	return i+2 < len(tokens) && tokens[i] == funcName && tokens[i+1] == "answers" && tokens[i+2] == name
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
		if err := rejectComputedFieldWhenOrdering(fields, i); err != nil {
			return err
		}
		expr, isExpression := field.Value.(string)
		if !isExpression {
			// A literal value has no expression to scan for a self/
			// forward-reference -- nothing to validate.
			continue
		}
		for j := i; j < len(fields); j++ {
			later := &fields[j]
			if later.Type != fieldTypeComputed {
				continue
			}
			if !valueReferencesAnswer(expr, later.Name) {
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

// rejectComputedFieldWhenOrdering rejects a computed field's own When
// condition referencing itself or a computed field declared after it --
// the same self/forward-reference rule validateComputedFieldOrdering
// already enforces for Value, applied to When instead. ComputeFields
// evaluates a computed field's own When before its Value, so a self/
// forward reference here hits the same missing-value problem, but worse:
// Condition.Evaluate collapses any evaluation error (e.g. a CEL map access
// to a not-yet-populated key) to false, so the field is silently omitted
// rather than erroring at all.
func rejectComputedFieldWhenOrdering(fields []FieldDefinition, i int) error {
	field := &fields[i]
	for j := i; j < len(fields); j++ {
		later := &fields[j]
		if later.Type != fieldTypeComputed || !field.When.MentionsCELIdentifier("answers."+later.Name) {
			continue
		}
		if j == i {
			return errUtils.Build(errUtils.ErrScaffoldComputedFieldInvalid).
				WithExplanationf("Field %q references itself in its own `when:` condition", field.Name).
				WithHint("A computed field can't reference its own not-yet-computed value; remove the self-reference").
				WithContext("field_name", field.Name).
				WithExitCode(2).
				Err()
		}
		return errUtils.Build(errUtils.ErrScaffoldComputedFieldInvalid).
			WithExplanationf("Field %q references computed field %q in its `when:` condition, which is declared after it", field.Name, later.Name).
			WithHintf("Declare %q before %q -- a computed field can only reference an earlier-declared computed field", later.Name, field.Name).
			WithContext("field_name", field.Name).
			WithContext("referenced_field", later.Name).
			WithExitCode(2).
			Err()
	}
	return nil
}

// validateOptionsNotComputed statically rejects a select/multiselect
// field's string-valued options: (the answers.<path> dot-path form, or a
// Go-template expression -- valueReferencesAnswer matches both shapes
// identically) when it references a type: computed field's name. Options
// resolution always runs before ComputeFields, for both the interactive
// dynamicOptionsFunc path and the non-interactive
// validateSelectValue/validateMultiSelectValue path (see
// resolveFieldOptionsFromAnswers), so a computed field's value can never
// exist yet at that point -- there is no timing under which this
// combination could work. Left unrejected, this permanently and silently
// disables option validation for the field (an empty resolved options list
// is treated by validateSelectValue/validateMultiSelectValue as "no
// constraint", not an error) and, interactively, presents the user with
// zero selectable choices, always. A matrix: axis referencing a computed
// field is unaffected by any of this -- matrix expansion runs on the final
// merged answers, after ComputeFields.
func validateOptionsNotComputed(fields []FieldDefinition) error {
	computedNames := collectComputedFieldNames(fields)
	if len(computedNames) == 0 {
		return nil
	}

	for i := range fields {
		field := &fields[i]
		source, ok := field.Options.(string)
		if !ok {
			continue
		}
		for _, computedName := range computedNames {
			if !valueReferencesAnswer(source, computedName) {
				continue
			}
			return errUtils.Build(errUtils.ErrScaffoldFieldOptionsInvalid).
				WithExplanationf("Field %q declares `options:` referencing computed field %q", field.Name, computedName).
				WithHint("options: is resolved before computed fields are evaluated, so a computed field's value is never available here -- use a static list, or reference a regular field instead").
				WithContext("field_name", field.Name).
				WithContext("referenced_field", computedName).
				Err()
		}
	}
	return nil
}

// validateWhenNotComputed statically rejects a regular (non-computed)
// field's when: condition when it references a type: computed field's
// name -- the same timing bug validateOptionsNotComputed rejects for
// options:, applied to When instead. Regular-field When conditions are
// evaluated (to decide prompt/validation visibility) before ComputeFields
// ever populates a computed field's value, so answers.<computed-name> is
// always absent at that point; the documented contract only ever allows
// the reverse dependency (a computed field's own When, or its Value/
// Template.Args, may reference a regular field). Left unrejected, the
// gated field is silently hidden and skipped during validation, forever,
// with no error surfaced at all.
func validateWhenNotComputed(fields []FieldDefinition) error {
	computedNames := collectComputedFieldNames(fields)
	if len(computedNames) == 0 {
		return nil
	}

	for i := range fields {
		field := &fields[i]
		if field.Type == fieldTypeComputed {
			continue
		}
		for _, computedName := range computedNames {
			if !field.When.MentionsCELIdentifier("answers." + computedName) {
				continue
			}
			return errUtils.Build(errUtils.ErrScaffoldComputedFieldInvalid).
				WithExplanationf("Field %q declares `when:` referencing computed field %q", field.Name, computedName).
				WithHint("when: is evaluated before computed fields are evaluated, so a computed field's value is never available here -- reference a regular field instead").
				WithContext("field_name", field.Name).
				WithContext("referenced_field", computedName).
				Err()
		}
	}
	return nil
}

// collectComputedFieldNames returns every type: computed field's name --
// shared by validateOptionsNotComputed and validateWhenNotComputed, which
// both reject a different field property referencing one of these names.
func collectComputedFieldNames(fields []FieldDefinition) []string {
	var names []string
	for i := range fields {
		if fields[i].Type == fieldTypeComputed {
			names = append(names, fields[i].Name)
		}
	}
	return names
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

// StripComputedFieldValues deletes every type: computed field's key from
// values in place. Used when a scaffold setup form's own result -- which
// already contains ComputeFields' output for this run, keyed just like any
// other field -- is about to be re-threaded through as a future call's
// cmdTemplateValues (see resolvePreCollectedValues). Left in place, that
// future call's own RejectComputedFieldOverrides would mistake this run's
// computed output for a user-supplied --set override on the same field and
// reject it, even though the user never touched it.
func StripComputedFieldValues(scaffoldConfig *ScaffoldConfig, values map[string]interface{}) {
	defer perf.Track(nil, "config.StripComputedFieldValues")()

	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Type == fieldTypeComputed {
			delete(values, field.Name)
		}
	}
}
