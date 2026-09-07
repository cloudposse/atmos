package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/perf"
)

// isMissingValue reports whether value is considered absent for required-field
// validation: nil, empty string (after trimming), empty []string, or empty
// []interface{} all count as missing.
func isMissingValue(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []interface{}:
		return len(v) == 0
	default:
		return false
	}
}

// MissingRequiredValues returns the names of required fields that have no
// usable (non-nil, non-empty-string) value in the provided values map. Used
// to fail fast in non-interactive mode instead of generating a broken
// project. A field whose When condition evaluates false against the
// already-known values is not prompted for interactively either, so it is
// never treated as missing here.
func MissingRequiredValues(scaffoldConfig *ScaffoldConfig, values map[string]interface{}) []string {
	defer perf.Track(nil, "config.MissingRequiredValues")()

	var missing []string
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if !field.Required {
			continue
		}
		if !field.When.Evaluate(condition.Context{Answers: values}) {
			continue
		}
		value, exists := values[field.Name]
		if !exists || value == nil || isMissingValue(value) {
			missing = append(missing, field.Name)
		}
	}
	return missing
}

// FieldFormOption configures optional behavior shared by
// PromptForScaffoldConfig and ValidateFieldValues. Callers that don't need
// any of it (the common case) simply omit every option.
type FieldFormOption func(*fieldFormOptions)

type fieldFormOptions struct {
	render FieldOptionsRenderer
}

// WithFieldOptionsRenderer supplies the renderer used to resolve a field's
// Options when declared as a Go-template expression (see
// FieldOptionsRenderer). Omitting this option disables the expression form,
// restricting Options resolution to literal lists and answers.* dot-paths.
//
//nolint:lintroller // trivial option constructor, same as pkg/ui/markdown's With* option functions.
func WithFieldOptionsRenderer(render FieldOptionsRenderer) FieldFormOption {
	return func(o *fieldFormOptions) { o.render = render }
}

func resolveFieldFormOptions(opts []FieldFormOption) fieldFormOptions {
	var o fieldFormOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// ValidateFieldValues validates every active declared field against its
// required, type, pattern, and option constraints. It deliberately ignores
// undeclared values so templates can continue accepting extensibility values
// supplied through --set.
func ValidateFieldValues(scaffoldConfig *ScaffoldConfig, values map[string]interface{}, opts ...FieldFormOption) error {
	defer perf.Track(nil, "config.ValidateFieldValues")()

	optCtx := optionsResolutionContext{
		answers:      values,
		fieldsByName: fieldDefinitionsByName(scaffoldConfig.Spec.Fields),
		render:       resolveFieldFormOptions(opts).render,
		delimiters:   defaultDelimiters(scaffoldConfig.Spec.Delimiters),
	}

	var missing []string
	var invalid []string
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if !field.When.Evaluate(condition.Context{Answers: values}) {
			continue
		}

		value, exists := values[field.Name]
		if !exists || value == nil || isMissingValue(value) {
			if field.Required {
				missing = append(missing, field.Name)
			}
			continue
		}

		if err := validateFieldValue(field, value, optCtx); err != nil {
			invalid = append(invalid, err.Error())
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, strings.Join(missing, ", "))
	}
	if len(invalid) > 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrGeneratorValidation, strings.Join(invalid, "; "))
	}
	return nil
}

// fieldDefinitionsByName indexes scaffold fields by name for Options
// dot-path label recovery (see sourceFieldLabels) -- built once per
// validation/form pass rather than once per field.
func fieldDefinitionsByName(fields []FieldDefinition) map[string]*FieldDefinition {
	byName := make(map[string]*FieldDefinition, len(fields))
	for i := range fields {
		byName[fields[i].Name] = &fields[i]
	}
	return byName
}

func validateFieldDefinitions(scaffoldConfig *ScaffoldConfig) error {
	delimiters := defaultDelimiters(scaffoldConfig.Spec.Delimiters)
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Validation != nil && field.Validation.Pattern != "" {
			if !isTextFieldType(field.Type) {
				return fmt.Errorf("%w: field %q uses validation.pattern but type %q is not text", errUtils.ErrGeneratorValidation, field.Name, field.Type)
			}
			if _, err := regexp.Compile(field.Validation.Pattern); err != nil {
				return fmt.Errorf("%w: field %q has invalid validation.pattern: %w", errUtils.ErrGeneratorValidation, field.Name, err)
			}
		}
		if err := validateFieldOptionsSource(field, delimiters); err != nil {
			return err
		}
		if err := validateFieldOptionsList(field); err != nil {
			return err
		}
	}
	return nil
}

// validateFieldOptionsList statically validates a static (non-string)
// Options list's shape -- mirrors validateMatrixAxisAnyList's precedent for
// matrix's own []any literal-list validation -- so a malformed option (a
// missing `value`, or a non-string/non-object element) surfaces at
// scaffold-load time rather than only when someone later tries to select
// from the broken field.
func validateFieldOptionsList(field *FieldDefinition) error {
	raw, ok := field.Options.([]any)
	if !ok {
		return nil
	}
	if _, err := resolvedOptionList(field.Name, raw); err != nil {
		return errUtils.Build(errUtils.ErrScaffoldFieldOptionsInvalid).
			WithExplanationf("Field %q declares an invalid `options:` entry: %s", field.Name, err).
			WithHint("Each option must be a string, or an object with a required `value` and optional `label`").
			WithContext("field_name", field.Name).
			Err()
	}
	return nil
}

// validateFieldOptionsSource statically validates a dynamic (string)
// Options value's shape: a Go-template expression (containing
// delimiters[0]) is accepted as-is, mirroring validateMatrixAxisStringValue
// -- its actual rendered result can only be checked once real answers are
// known, at prompt/generation time. A dot-path must start with the required
// "answers." prefix. Unlike a matrix axis (whose own load-time check stops
// here too), this deliberately does NOT check that the referenced field was
// declared earlier: a dot-path may equally reference a spec.values preset or
// a --set-supplied value that was never declared as a field at all (the
// same "structured value supplied through --set or a template preset"
// resolveFieldOptionsFromAnswers already documents as valid) -- something
// load time can't distinguish from a genuine typo/forward reference. That
// distinction is left entirely to resolveFieldOptions's runtime checks
// (errFieldOptionsSourceNotFound/errFieldOptionsSourceNotList), the same way
// a matrix axis's own dynamic source is never order-validated either.
func validateFieldOptionsSource(field *FieldDefinition, delimiters []string) error {
	source, ok := field.Options.(string)
	if !ok {
		return nil
	}
	if strings.Contains(source, delimiters[0]) {
		return nil
	}
	if strings.HasPrefix(source, answersPrefix) {
		return nil
	}
	return errUtils.Build(errUtils.ErrScaffoldFieldOptionsInvalid).
		WithExplanationf("Field %q declares `options: %q`, which is neither a template expression nor an `answers.*` reference", field.Name, source).
		WithHintf("Reference a prior answer with the %q prefix, e.g. `options: \"answers.environments\"`, or use a Go-template expression", answersPrefix).
		WithContext("field_name", field.Name).
		Err()
}

// FieldOptionsRenderer renders a Go-template Options expression (e.g.
// `{{ splitList "," answers.csv_field }}`) against answers into its
// resolved list of string values, honoring the scaffold's own
// spec.delimiters override. Mirrors pkg/generator/engine's AxisRenderer
// exactly (same signature) -- duplicated rather than imported for the same
// reason as defaultDelimiters above; both are satisfied by the same
// *engine.Processor.RenderAnswersListExpression method at the call site, so
// `options:` and `matrix:` axis expressions render identically. A nil
// renderer disables the expression form, restricting resolution to literal
// lists and answers.* dot-paths.
type FieldOptionsRenderer func(expr string, answers map[string]interface{}, delimiters []string) ([]string, error)

// ResolvedOption is one resolved choice for a select/multiselect field:
// Label is what's displayed, Value is what's stored in answers and passed
// to templates. They're identical unless the field's Options (or the field
// referenced by a dot-path Options value) declared explicit {label, value}
// pairs -- see FieldOption.
type ResolvedOption struct {
	Label string
	Value string
}

// resolveFieldOptions resolves a field's declared Options into its concrete
// list of choices: nil for an unset field, the list as-is for a static
// list, or -- for a string -- either a Go-template expression (rendered via
// render) or an answers.<path> dot-path (the referenced answer must already
// be list-shaped). Mirrors pkg/generator/engine/matrix.go's
// resolveMatrixAxis; duplicated for the same import-cycle reason as
// defaultDelimiters above.
func resolveFieldOptions(field *FieldDefinition, answers map[string]interface{}, fieldsByName map[string]*FieldDefinition, render FieldOptionsRenderer, delimiters []string) ([]ResolvedOption, error) {
	switch v := field.Options.(type) {
	case nil:
		return nil, nil
	case []string:
		return plainOptionList(v), nil
	case []any:
		return resolvedOptionList(field.Name, v)
	case string:
		if strings.Contains(v, delimiters[0]) {
			return resolveFieldOptionsExpression(field.Name, v, answers, render, delimiters)
		}
		return resolveFieldOptionsFromAnswers(field.Name, v, answers, fieldsByName)
	default:
		return nil, fmt.Errorf("%w: field %q options resolved to %T", errFieldOptionsSourceInvalid, field.Name, v)
	}
}

// plainOptionList wraps a literal []string Options value into
// label-equals-value ResolvedOptions.
func plainOptionList(values []string) []ResolvedOption {
	result := make([]ResolvedOption, len(values))
	for i, v := range values {
		result[i] = ResolvedOption{Label: v, Value: v}
	}
	return result
}

// resolveFieldOptionsExpression renders a template-expression Options value
// via render. A nil render (e.g. a unit test that doesn't wire one) is a
// clear error rather than a silently empty option list. The renderer only
// ever produces plain strings -- Sprig/Gomplate functions like splitList
// can't produce structured {label,value} pairs -- so label always equals
// value here.
func resolveFieldOptionsExpression(fieldName, expr string, answers map[string]interface{}, render FieldOptionsRenderer, delimiters []string) ([]ResolvedOption, error) {
	if render == nil {
		return nil, fmt.Errorf("%w: field %q options is a template expression, but no renderer is available", errFieldOptionsSourceInvalid, fieldName)
	}
	values, err := render(expr, answers, delimiters)
	if err != nil {
		return nil, err
	}
	return plainOptionList(values), nil
}

// resolveFieldOptionsFromAnswers resolves a dynamic Options source (a
// dot-path string with the "answers" prefix) against the answers map,
// requiring the resolved value to already be list-shaped -- a multiselect
// answer, or a structured value supplied through --set or a template
// preset. When the dot-path is a single segment (answers.<name>, not a
// deeper path) and <name> names a declared field whose own Options is a
// static list, labels are recovered from that field's definition for
// whichever values are present here -- the same (but filtered) {label,
// value} pairs the source field itself would show. A deeper path, an
// undeclared name, or a dynamically-sourced upstream field all simply fall
// back to label == value; this is a best-effort presentation lookup, never
// an error.
func resolveFieldOptionsFromAnswers(fieldName, source string, answers map[string]interface{}, fieldsByName map[string]*FieldDefinition) ([]ResolvedOption, error) {
	path, ok := strings.CutPrefix(source, answersPrefix)
	if !ok {
		return nil, fmt.Errorf("%w: field %q: %q does not start with %q", errFieldOptionsSourceInvalid, fieldName, source, answersPrefix)
	}

	segments := strings.Split(path, ".")
	var current interface{} = answers
	for _, segment := range segments {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: field %q source %q: %q is not a map", errFieldOptionsSourceNotFound, fieldName, source, segment)
		}
		value, exists := m[segment]
		if !exists {
			return nil, fmt.Errorf("%w: field %q source %q", errFieldOptionsSourceNotFound, fieldName, source)
		}
		current = value
	}

	values, err := answersStringList(fieldName, source, current)
	if err != nil {
		return nil, err
	}

	labels := sourceFieldLabels(segments, fieldsByName)
	result := make([]ResolvedOption, len(values))
	for i, v := range values {
		label := v
		if l, ok := labels[v]; ok {
			label = l
		}
		result[i] = ResolvedOption{Label: label, Value: v}
	}
	return result, nil
}

// answersStringList coerces a resolved answers value into []string,
// matching the shapes resolveMatrixAxisFromAnswers accepts (nil, []string,
// []any of strings).
func answersStringList(fieldName, source string, current any) ([]string, error) {
	switch v := current.(type) {
	case nil:
		return nil, nil
	case []string:
		return v, nil
	case []any:
		values := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: field %q source %q: element at index %d is a %T, not a string", errFieldOptionsSourceInvalid, fieldName, source, i, item)
			}
			values[i] = s
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%w: field %q source %q resolved to %T", errFieldOptionsSourceNotList, fieldName, source, v)
	}
}

// sourceFieldLabels returns a value->label lookup recovered from the
// referenced field's own static Options list, or nil if segments names a
// nested path, an undeclared field, or a field whose Options isn't a static
// list -- see resolveFieldOptionsFromAnswers's doc comment for why each of
// those degrades silently instead of erroring.
func sourceFieldLabels(segments []string, fieldsByName map[string]*FieldDefinition) map[string]string {
	if len(segments) != 1 {
		return nil
	}
	source, ok := fieldsByName[segments[0]]
	if !ok {
		return nil
	}

	raw, ok := source.Options.([]any)
	if !ok {
		return nil // plain []string is already its own label; anything else has no static labels to recover.
	}

	options, err := resolvedOptionList(source.Name, raw)
	if err != nil {
		return nil
	}
	labels := make(map[string]string, len(options))
	for _, opt := range options {
		labels[opt.Value] = opt.Label
	}
	return labels
}

// resolvedOptionList coerces a []any Options/answers value into
// []ResolvedOption: a plain string becomes label == value; a map (with a
// required non-empty "value" string and optional "label" string, defaulting
// to value when absent or empty) becomes a FieldOption-shaped entry.
func resolvedOptionList(fieldName string, values []any) ([]ResolvedOption, error) {
	result := make([]ResolvedOption, len(values))
	for i, item := range values {
		opt, err := resolvedOptionFromItem(fieldName, i, item)
		if err != nil {
			return nil, err
		}
		result[i] = opt
	}
	return result, nil
}

// resolvedOptionFromItem parses one Options list element, where
// map[string]any is what yaml.v3 actually produces for a mapping decoded
// into an `any` field (confirmed against yaml.v3's own decode.go:
// string-keyed mappings use stringMapType, not the legacy yaml.v2
// map[any]any); the map[any]any branch is a defensive fallback for any
// other decode path, mirroring pkg/condition's own normalizeAnyMap.
func resolvedOptionFromItem(fieldName string, index int, item any) (ResolvedOption, error) {
	switch v := item.(type) {
	case string:
		return ResolvedOption{Label: v, Value: v}, nil
	case map[string]interface{}:
		return fieldOptionFromMap(fieldName, index, v)
	case map[interface{}]interface{}:
		converted := make(map[string]interface{}, len(v))
		for k, val := range v {
			converted[fmt.Sprint(k)] = val
		}
		return fieldOptionFromMap(fieldName, index, converted)
	default:
		return ResolvedOption{}, fmt.Errorf("%w: field %q option at index %d is a %T, not a string or a {label, value} object", errFieldOptionsSourceInvalid, fieldName, index, item)
	}
}

// fieldOptionFromMap parses one {label, value} object: value is required
// and must be a non-empty string; label, if present, must be a string (an
// empty string is treated the same as omitting it) -- a non-string label
// (e.g. a bare YAML number where quotes were meant) is rejected the same
// way a non-string value already is, rather than silently discarded.
func fieldOptionFromMap(fieldName string, index int, m map[string]interface{}) (ResolvedOption, error) {
	valueRaw, ok := m["value"]
	if !ok {
		return ResolvedOption{}, fmt.Errorf("%w: field %q option at index %d is missing required `value`", errFieldOptionsSourceInvalid, fieldName, index)
	}
	value, ok := valueRaw.(string)
	if !ok || value == "" {
		return ResolvedOption{}, fmt.Errorf("%w: field %q option at index %d has a non-string or empty `value`", errFieldOptionsSourceInvalid, fieldName, index)
	}
	label := value
	if labelRaw, ok := m["label"]; ok {
		l, ok := labelRaw.(string)
		if !ok {
			return ResolvedOption{}, fmt.Errorf("%w: field %q option at index %d has a non-string `label`", errFieldOptionsSourceInvalid, fieldName, index)
		}
		if l != "" {
			label = l
		}
	}
	return ResolvedOption{Label: label, Value: value}, nil
}

// answersPrefix is the required prefix for a dynamic matrix axis or field
// Options source, a root reference into the answers map -- see
// validateFileMatrix and validateFieldOptionsSource.
const answersPrefix = "answers."

// defaultLeftDelimiter and defaultRightDelimiter are the Go template
// delimiters assumed when a scaffold declares no override -- mirrors
// pkg/generator/engine's own defaults (duplicated rather than imported: that
// package already imports this one, so importing it back would cycle).
const (
	defaultLeftDelimiter  = "{{"
	defaultRightDelimiter = "}}"
)

// defaultDelimiters returns delimiters unchanged when it's a valid
// two-element pair with both sides non-empty, otherwise the default
// "{{"/"}}" -- mirrors pkg/generator/engine's defaultDelimiters so a
// matrix axis or field Options template expression is recognized here using
// the same delimiters ExpandMatrix/RenderAnswersListExpression will actually
// use at generation/prompt time. Named generically (not "axis") because it
// now backs both matrix axis and field Options delimiter detection.
func defaultDelimiters(delimiters []string) []string {
	if len(delimiters) != 2 || delimiters[0] == "" || delimiters[1] == "" {
		return []string{defaultLeftDelimiter, defaultRightDelimiter}
	}
	return delimiters
}

// validateFileMatrix statically validates each spec.files[] entry's matrix
// configuration: target is required when matrix is set, and every axis is
// either a non-empty literal list or an `answers.`-prefixed dot-path
// string. LoadScaffoldConfigFromContent's JSON Schema validation (see
// MatrixAxes/FileSpec.JSONSchemaExtend in config.go) already mirrors most
// of this and runs first, so this function is mainly a defensive backstop
// plus the one constraint schema can't express: the `answers.` prefix
// requirement.
func validateFileMatrix(scaffoldConfig *ScaffoldConfig) error {
	delimiters := defaultDelimiters(scaffoldConfig.Spec.Delimiters)
	for i := range scaffoldConfig.Spec.Files {
		file := &scaffoldConfig.Spec.Files[i]
		if len(file.Matrix) == 0 {
			continue
		}
		if file.Target == "" {
			return fmt.Errorf("%w: file %q", errUtils.ErrScaffoldMatrixTargetRequired, file.Path)
		}
		for axis, value := range file.Matrix {
			if err := validateMatrixAxisValue(file.Path, axis, value, delimiters); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateMatrixAxisValue validates one matrix axis's declared value: a
// non-empty literal list, a string that references a top-level answer via
// the same `answers.<path>` root reference When's CEL expressions read, or a
// Go-template expression (e.g. `{{ collectKeys answers.environments }}`) computing
// the list from nested/structured answer data. A template expression's
// actual rendered result can only be checked once real answers are known, at
// generation time -- this only confirms the string looks like one.
func validateMatrixAxisValue(filePath, axis string, value any, delimiters []string) error {
	switch v := value.(type) {
	case string:
		return validateMatrixAxisStringValue(filePath, axis, v, delimiters)
	case []string:
		return validateMatrixAxisNonEmpty(filePath, axis, len(v))
	case []any:
		return validateMatrixAxisAnyList(filePath, axis, v)
	default:
		return fmt.Errorf("%w: file %q axis %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis)
	}
}

// validateMatrixAxisStringValue validates a string axis value: either an
// `answers.<path>` dot-path or a Go-template expression (see
// validateMatrixAxisValue's doc comment), where delimiters is the
// scaffold's own configured left/right pair (see defaultDelimiters), so a
// custom delimiter like `[[ ... ]]` is recognized as an expression instead
// of being rejected for not starting with `answers.`.
func validateMatrixAxisStringValue(filePath, axis, v string, delimiters []string) error {
	if strings.HasPrefix(v, answersPrefix) || strings.Contains(v, delimiters[0]) {
		return nil
	}
	return fmt.Errorf("%w: file %q axis %q: %q is neither a template expression nor does it start with %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis, v, answersPrefix)
}

// validateMatrixAxisNonEmpty rejects an empty literal-list axis value.
func validateMatrixAxisNonEmpty(filePath, axis string, length int) error {
	if length == 0 {
		return fmt.Errorf("%w: file %q axis %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis)
	}
	return nil
}

// validateMatrixAxisAnyList validates a []any literal-list axis value: it
// must be non-empty and every element must be a string.
func validateMatrixAxisAnyList(filePath, axis string, v []any) error {
	if err := validateMatrixAxisNonEmpty(filePath, axis, len(v)); err != nil {
		return err
	}
	for _, item := range v {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%w: file %q axis %q: element %v is not a string", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis, item)
		}
	}
	return nil
}

func isTextFieldType(fieldType string) bool {
	switch fieldType {
	case "input", "text", "string":
		return true
	default:
		return false
	}
}

// optionsResolutionContext bundles what resolveFieldOptions needs beyond the
// field and value themselves, keeping validateFieldValue/
// validateSelectValue/validateMultiSelectValue within revive's
// argument-limit -- these four always travel together from a single
// ValidateFieldValues/dynamicOptionsFunc call site down to resolveFieldOptions.
type optionsResolutionContext struct {
	answers      map[string]interface{}
	fieldsByName map[string]*FieldDefinition
	render       FieldOptionsRenderer
	delimiters   []string
}

func validateFieldValue(field *FieldDefinition, value interface{}, ctx optionsResolutionContext) error {
	switch field.Type {
	case "input", "text", "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeText, field.Name)
		}
		return validateTextValue(field, text)
	case "select":
		return validateSelectValue(field, value, ctx)
	case "multiselect":
		return validateMultiSelectValue(field, value, ctx)
	case "confirm", "bool", "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeBoolean, field.Name)
		}
	}
	return nil
}

func validateSelectValue(field *FieldDefinition, value interface{}, ctx optionsResolutionContext) error {
	selected, ok := value.(string)
	if !ok {
		return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeStringOption, field.Name)
	}
	options, err := resolveFieldOptions(field, ctx.answers, ctx.fieldsByName, ctx.render, ctx.delimiters)
	if err != nil {
		return err
	}
	if len(options) > 0 && !containsOption(options, selected) {
		return unsupportedOptionError(field.Name, selected, options)
	}
	return nil
}

func validateMultiSelectValue(field *FieldDefinition, value interface{}, ctx optionsResolutionContext) error {
	selected, ok := stringSliceValue(value)
	if !ok {
		return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeStringOptions, field.Name)
	}
	options, err := resolveFieldOptions(field, ctx.answers, ctx.fieldsByName, ctx.render, ctx.delimiters)
	if err != nil {
		return err
	}
	for _, option := range selected {
		if len(options) > 0 && !containsOption(options, option) {
			return unsupportedOptionError(field.Name, option, options)
		}
	}
	return nil
}

// unsupportedOptionError reports a value that isn't among a field's resolved
// options, naming the valid values so a value/label mix-up (e.g. passing a
// display label like "Development" via --set instead of its underlying
// value "dev") is immediately obvious rather than requiring a trip back to
// the scaffold.yaml to see what's actually accepted.
func unsupportedOptionError(fieldName, option string, options []ResolvedOption) error {
	values := make([]string, len(options))
	for i, opt := range options {
		values[i] = opt.Value
	}
	return fmt.Errorf("%w: field %q option %q (valid values: %s)", errFieldUnsupportedOption, fieldName, option, strings.Join(values, ", "))
}

func validateTextValue(field *FieldDefinition, value string) error {
	if field.Validation == nil || field.Validation.Pattern == "" {
		return nil
	}
	pattern, err := regexp.Compile(field.Validation.Pattern)
	if err != nil {
		return fmt.Errorf("%w for field %q: %w", errInvalidFieldPattern, field.Name, err)
	}
	if pattern.MatchString(value) {
		return nil
	}
	if field.Validation.Message != "" {
		return fmt.Errorf("%w for field %q: %s", errFieldValidationFailed, field.Name, field.Validation.Message)
	}
	return fmt.Errorf("%w for field %q: does not match validation.pattern", errFieldValidationFailed, field.Name)
}

func containsOption(options []ResolvedOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func stringSliceValue(value interface{}) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		return v, true
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[i] = text
		}
		return result, true
	default:
		return nil, false
	}
}

// isBooleanFieldType reports whether a field's declared type represents a
// boolean value (the confirm prompt type, or an explicit bool/boolean type).
func isBooleanFieldType(fieldType string) bool {
	switch fieldType {
	case "confirm", "bool", "boolean":
		return true
	default:
		return false
	}
}

// multiSelectSetDelimiter splits a --set value for a multiselect field into
// its selected options -- see CoerceFieldValueTypes.
const multiSelectSetDelimiter = ","

// CoerceFieldValueTypes converts string values for boolean-typed fields
// (confirm/bool/boolean) to native Go bools, and for multiselect fields to
// a []string of selected options, in place. --set (and other external
// string sources) always supplies raw strings; without this, a boolean
// value like "false" stays the truthy non-empty string "false" for both Go
// template interpolation and When condition evaluation (e.g. `answers.x ==
// true` never matches a string), and a multiselect value stays an
// unsplit string a matrix axis, a dynamic field Options source, or `for`
// range can't iterate. Values that
// aren't strings (YAML defaults, or values already typed by an interactive
// prompt) are left untouched. Invalid external boolean values return an
// error rather than silently changing the result of a conditional
// expression.
func CoerceFieldValueTypes(scaffoldConfig *ScaffoldConfig, values map[string]interface{}) error {
	defer perf.Track(nil, "config.CoerceFieldValueTypes")()

	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		raw, ok := values[field.Name].(string)
		if !ok {
			continue
		}
		switch {
		case isBooleanFieldType(field.Type):
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return fmt.Errorf("%w: %w for field %q: %q", errUtils.ErrGeneratorValidation, errInvalidBooleanFieldValue, field.Name, raw)
			}
			values[field.Name] = parsed
		case field.Type == "multiselect":
			values[field.Name] = splitMultiSelectValue(raw)
		}
	}
	return nil
}

// splitMultiSelectValue splits a --set multiselect value ("dev,staging") into
// its trimmed options. An empty (or whitespace-only) value yields an empty,
// non-nil slice -- distinct from the field never being set at all -- so a
// matrix axis, a dynamic field Options source, or when: sourced from it sees
// zero selections, not one blank one.
func splitMultiSelectValue(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, multiSelectSetDelimiter)
	values := make([]string, len(parts))
	for i, part := range parts {
		values[i] = strings.TrimSpace(part)
	}
	return values
}

// validateInteractiveTextValue validates a text field's value as entered
// interactively. Matches isMissingValue/ValidateFieldValues, which both trim
// before checking for emptiness -- otherwise a whitespace-only answer passes
// the form but fails the canonical post-form validation.
func validateInteractiveTextValue(field *FieldDefinition, value string) error {
	if field.Required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
	}
	if value == "" && !field.Required {
		return nil
	}
	return validateTextValue(field, value)
}

// fieldTitle returns the display title for a field: its label when set,
// otherwise its name.
func fieldTitle(field *FieldDefinition) string {
	if field.Label != "" {
		return field.Label
	}
	return field.Name
}
