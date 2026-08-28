package config

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PromptForScaffoldConfig prompts the user for scaffold configuration values using a dynamic form built from the provided ScaffoldConfig; userValues supplies initial values and is populated with results; returns an error on failure.
func PromptForScaffoldConfig(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) error {
	defer perf.Track(nil, "config.PromptForScaffoldConfig")()

	// Initialize form values with user values and defaults
	formValues := initializeFormValues(scaffoldConfig, userValues)

	// Build the form with grouped fields
	huhForm, valueGetters, err := buildConfigForm(scaffoldConfig, formValues)
	if err != nil {
		return err
	}
	if huhForm == nil {
		return nil // No fields to prompt for.
	}

	// Run the form interaction
	if err := runFormInteraction(huhForm); err != nil {
		return err
	}

	// Extract form values back to userValues
	extractFormValues(userValues, valueGetters)

	return ValidateFieldValues(scaffoldConfig, userValues)
}

// initializeFormValues merges default values with user-provided values, in
// the same defaults -> Spec.Values -> userValues precedence order as
// DeepMerge, so the interactive form and the non-interactive path never
// disagree about a preset template value declared in Spec.Values.
func initializeFormValues(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}) map[string]interface{} {
	formValues := make(map[string]interface{})

	// Set defaults from scaffold config
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if field.Default != nil {
			formValues[field.Name] = field.Default
		}
	}

	// Preset values declared in the template override field defaults.
	for key, value := range scaffoldConfig.Spec.Values {
		formValues[key] = value
	}

	// Override with user values
	for key, value := range userValues {
		formValues[key] = value
	}

	return formValues
}

// buildConfigForm builds the configuration form preserving the field order
// declared in the template. Each field gets its own huh.Group so a field
// declaring When can hide its group based on answers collected from fields
// declared earlier (huh runs groups sequentially, one page at a time).
// Returns the form and value getters for extracting values after submission.
// Returns a nil form when the template declares no fields.
func buildConfigForm(scaffoldConfig *ScaffoldConfig, formValues map[string]interface{}) (*huh.Form, map[string]func() interface{}, error) {
	if len(scaffoldConfig.Spec.Fields) == 0 {
		return nil, nil, nil
	}

	// Should we run in accessible mode?
	// Note: ACCESSIBLE is a standard environment variable used by the huh form library
	// to enable accessible mode for screen readers. Using viper for consistency.
	v := viper.New()
	_ = v.BindEnv("ACCESSIBLE")
	accessible := v.GetBool("ACCESSIBLE")

	// Store value getters for after form completion. Also read live (by a
	// hidden field's WithHideFunc closure below) to answer "what has the
	// user entered so far" during the form session itself.
	valueGetters := make(map[string]func() interface{})

	var groups []*huh.Group
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if _, exists := valueGetters[field.Name]; exists {
			// A silent map overwrite here would still render both prompts but
			// drop one of their answers when extractFormValues runs, since only
			// the last getter for this name survives.
			return nil, nil, errUtils.Build(errUtils.ErrDuplicateScaffoldFieldName).
				WithExplanationf("Field name `%s` is declared more than once in scaffold.yaml", field.Name).
				WithHint("Each field's `name` must be unique so its answer isn't silently dropped").
				WithContext("field_name", field.Name).
				WithExitCode(2).
				Err()
		}
		huhField, getter := createField(field.Name, field, formValues)
		valueGetters[field.Name] = getter

		group := huh.NewGroup(huhField)
		if !field.When.IsZero() {
			group = group.WithHideFunc(fieldHideFunc(field.When, valueGetters))
		}
		groups = append(groups, group)
	}

	huhForm := huh.NewForm(groups...).WithAccessible(accessible)

	return huhForm, valueGetters, nil
}

// snapshotAnswers reads the current live value of every field's getter into
// a plain map, for evaluating a later field's When condition against
// answers collected so far. Getters for fields not yet reached by the user
// simply return their zero/default value, which a well-formed When
// referencing only earlier-declared fields never observes.
func snapshotAnswers(valueGetters map[string]func() interface{}) map[string]any {
	answers := make(map[string]any, len(valueGetters))
	for name, getter := range valueGetters {
		answers[name] = getter()
	}
	return answers
}

// fieldHideFunc builds the huh.Group.WithHideFunc closure for a field
// declaring When: the group is hidden whenever When evaluates false against
// a live snapshot of every other field's current answer.
func fieldHideFunc(when condition.Condition, valueGetters map[string]func() interface{}) func() bool {
	return func() bool {
		return !when.Evaluate(condition.Context{Answers: snapshotAnswers(valueGetters)})
	}
}

// runFormInteraction is the seam used to execute a huh form.
// Tests replace this variable with a no-op to avoid launching a real TUI.
var runFormInteraction = func(huhForm *huh.Form) error {
	err := huhForm.Run()
	if err != nil {
		return fmt.Errorf("user aborted the configuration: %w", err)
	}
	return nil
}

// extractFormValues copies form values back to userValues map using value getters.
func extractFormValues(userValues map[string]interface{}, valueGetters map[string]func() interface{}) {
	for key, getter := range valueGetters {
		userValues[key] = getter()
	}
}

// createField creates a huh field based on the field definition.
// It returns the field and a function to get the updated value.
//
//nolint:gocognit,revive,cyclop,funlen // complex TUI field factory handling multiple field types
func createField(key string, field *FieldDefinition, values map[string]interface{}) (huh.Field, func() interface{}) {
	// Get current value or default
	currentValue := values[key]
	if currentValue == nil {
		currentValue = field.Default
	}

	switch field.Type {
	case "input", "text", "string":
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}
		input := huh.NewInput().
			Title(fieldTitle(field)).
			Description(field.Description).
			Placeholder(field.Placeholder).
			Value(&value)

		if field.Required {
			input = input.Validate(func(s string) error {
				return validateInteractiveTextValue(field, s)
			})
		} else if field.Validation != nil && field.Validation.Pattern != "" {
			input = input.Validate(func(s string) error {
				return validateInteractiveTextValue(field, s)
			})
		}

		return input, func() interface{} { return value }

	case "select":
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}

		var options []huh.Option[string]
		for _, option := range field.Options {
			options = append(options, huh.NewOption(option, option))
		}

		selectField := huh.NewSelect[string]().
			Title(fieldTitle(field)).
			Description(field.Description).
			Options(options...).
			Value(&value)

		if field.Required {
			selectField = selectField.Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
				}
				return validateFieldValue(field, s)
			})
		}

		return selectField, func() interface{} { return value }

	case "multiselect":
		var value []string
		if slice, ok := currentValue.([]string); ok {
			value = slice
		} else if interfaceSlice, ok := currentValue.([]interface{}); ok {
			// Convert []interface{} to []string (common when loading from YAML)
			for _, item := range interfaceSlice {
				if str, ok := item.(string); ok {
					value = append(value, str)
				}
			}
		}

		var options []huh.Option[string]
		for _, option := range field.Options {
			options = append(options, huh.NewOption(option, option))
		}

		multiSelect := huh.NewMultiSelect[string]().
			Title(fieldTitle(field)).
			Description(field.Description).
			Options(options...).
			Value(&value).
			Filterable(true)

		if field.Required {
			multiSelect = multiSelect.Validate(func(s []string) error {
				if len(s) == 0 {
					return fmt.Errorf("%w: at least one %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
				}
				return validateFieldValue(field, s)
			})
		}

		return multiSelect, func() interface{} { return value }

	case "confirm", "bool", "boolean":
		var value bool
		if b, ok := currentValue.(bool); ok {
			value = b
		}

		confirm := huh.NewConfirm().
			Title(fieldTitle(field)).
			Description(field.Description).
			Value(&value).
			Affirmative("Yes").
			Negative("No")

		return confirm, func() interface{} { return value }

	default:
		// Unknown types are rejected by schema validation before the form is
		// built; fall back to a plain input as defense in depth rather than
		// panicking.
		log.Warn("Unknown scaffold field type, falling back to input", "field", key, "type", field.Type)
		var value string
		if str, ok := currentValue.(string); ok {
			value = str
		}
		input := huh.NewInput().
			Title(fieldTitle(field)).
			Description(field.Description).
			Value(&value)
		return input, func() interface{} { return value }
	}
}

// GetConfigurationSummary returns table rows and header representing scaffold
// configuration, merged values, and their sources. Rows follow the declared
// field order.
func GetConfigurationSummary(scaffoldConfig *ScaffoldConfig, mergedValues map[string]interface{}, valueSources map[string]string) ([][]string, []string) {
	defer perf.Track(nil, "config.GetConfigurationSummary")()

	// Prepare table rows
	var rows [][]string
	for i := range scaffoldConfig.Spec.Fields {
		key := scaffoldConfig.Spec.Fields[i].Name
		value, exists := mergedValues[key]
		if !exists {
			continue
		}

		var valueStr string
		switch v := value.(type) {
		case []string:
			valueStr = strings.Join(v, ", ")
		case bool:
			valueStr = fmt.Sprintf("%t", v)
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		source := valueSources[key]
		if source == "" {
			source = "default"
		}

		rows = append(rows, []string{
			key,
			valueStr,
			source,
		})
	}

	header := []string{"Setting", "Value", "Source"}
	return rows, header
}
