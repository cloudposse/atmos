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

// ValidateFieldValues validates every active declared field against its
// required, type, pattern, and option constraints. It deliberately ignores
// undeclared values so templates can continue accepting extensibility values
// supplied through --set.
func ValidateFieldValues(scaffoldConfig *ScaffoldConfig, values map[string]interface{}) error {
	defer perf.Track(nil, "config.ValidateFieldValues")()

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

		if err := validateFieldValue(field, value); err != nil {
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

func validateFieldDefinitions(scaffoldConfig *ScaffoldConfig) error {
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Validation == nil || field.Validation.Pattern == "" {
			continue
		}
		if !isTextFieldType(field.Type) {
			return fmt.Errorf("%w: field %q uses validation.pattern but type %q is not text", errUtils.ErrGeneratorValidation, field.Name, field.Type)
		}
		if _, err := regexp.Compile(field.Validation.Pattern); err != nil {
			return fmt.Errorf("%w: field %q has invalid validation.pattern: %w", errUtils.ErrGeneratorValidation, field.Name, err)
		}
	}
	return nil
}

// answersPrefix is the required prefix for a dynamic matrix axis source, a
// root reference into the answers map -- see validateFileMatrix.
const answersPrefix = "answers."

// validateFileMatrix statically validates each spec.files[] entry's matrix
// configuration: target is required when matrix is set, and every axis is
// either a non-empty literal list or an `answers.`-prefixed dot-path
// string. LoadScaffoldConfigFromContent's JSON Schema validation (see
// MatrixAxes/FileSpec.JSONSchemaExtend in config.go) already mirrors most
// of this and runs first, so this function is mainly a defensive backstop
// plus the one constraint schema can't express: the `answers.` prefix
// requirement.
func validateFileMatrix(scaffoldConfig *ScaffoldConfig) error {
	for i := range scaffoldConfig.Spec.Files {
		file := &scaffoldConfig.Spec.Files[i]
		if len(file.Matrix) == 0 {
			continue
		}
		if file.Target == "" {
			return fmt.Errorf("%w: file %q", errUtils.ErrScaffoldMatrixTargetRequired, file.Path)
		}
		for axis, value := range file.Matrix {
			if err := validateMatrixAxisValue(file.Path, axis, value); err != nil {
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
func validateMatrixAxisValue(filePath, axis string, value any) error {
	switch v := value.(type) {
	case string:
		if !strings.HasPrefix(v, answersPrefix) && !strings.Contains(v, "{{") {
			return fmt.Errorf("%w: file %q axis %q: %q is neither a template expression nor does it start with %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis, v, answersPrefix)
		}
	case []string:
		if len(v) == 0 {
			return fmt.Errorf("%w: file %q axis %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis)
		}
	case []any:
		if len(v) == 0 {
			return fmt.Errorf("%w: file %q axis %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis)
		}
		for _, item := range v {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("%w: file %q axis %q: element %v is not a string", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis, item)
			}
		}
	default:
		return fmt.Errorf("%w: file %q axis %q", errUtils.ErrScaffoldMatrixAxisInvalid, filePath, axis)
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

func validateFieldValue(field *FieldDefinition, value interface{}) error {
	switch field.Type {
	case "input", "text", "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeText, field.Name)
		}
		return validateTextValue(field, text)
	case "select":
		return validateSelectValue(field, value)
	case "multiselect":
		return validateMultiSelectValue(field, value)
	case "confirm", "bool", "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeBoolean, field.Name)
		}
	}
	return nil
}

func validateSelectValue(field *FieldDefinition, value interface{}) error {
	selected, ok := value.(string)
	if !ok {
		return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeStringOption, field.Name)
	}
	if len(field.Options) > 0 && !containsOption(field.Options, selected) {
		return unsupportedOptionError(field.Name, selected)
	}
	return nil
}

func validateMultiSelectValue(field *FieldDefinition, value interface{}) error {
	selected, ok := stringSliceValue(value)
	if !ok {
		return fmt.Errorf(fieldNameErrorFormat, errFieldMustBeStringOptions, field.Name)
	}
	for _, option := range selected {
		if len(field.Options) > 0 && !containsOption(field.Options, option) {
			return unsupportedOptionError(field.Name, option)
		}
	}
	return nil
}

func unsupportedOptionError(fieldName, option string) error {
	return fmt.Errorf("%w: field %q option %q", errFieldUnsupportedOption, fieldName, option)
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

func containsOption(options []string, value string) bool {
	for _, option := range options {
		if option == value {
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
// unsplit string a matrix axis or `for` range can't iterate. Values that
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
// matrix axis or when: sourced from it sees zero selections, not one blank
// one.
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
