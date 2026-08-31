package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PromptForScaffoldConfig prompts the user for scaffold configuration values using a dynamic form built from the provided ScaffoldConfig; userValues supplies initial values and is populated with results; returns an error on failure.
// Accepts the same FieldFormOption as ValidateFieldValues (e.g.
// WithFieldOptionsRenderer); most callers need none of them.
func PromptForScaffoldConfig(scaffoldConfig *ScaffoldConfig, userValues map[string]interface{}, opts ...FieldFormOption) error {
	defer perf.Track(nil, "config.PromptForScaffoldConfig")()

	// Initialize form values with user values and defaults
	formValues := initializeFormValues(scaffoldConfig, userValues)

	// Build the form with grouped fields
	huhForm, ctx, err := buildConfigForm(scaffoldConfig, formValues, opts...)
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

	// Now that the interactive form has finished and the terminal is back to
	// normal, it's safe to report any dynamic Options resolution failures
	// collected during the session (see reportOptionsErrors).
	reportOptionsErrors(ctx.optionsErrors)

	// Extract form values back to userValues
	extractFormValues(userValues, ctx.valueGetters)

	return ValidateFieldValues(scaffoldConfig, userValues, opts...)
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
// Returns the form and the fieldFormContext built for it -- callers use
// ctx.valueGetters to extract values after submission and ctx.optionsErrors
// to report dynamic Options resolution failures collected during the form
// session (see reportOptionsErrors). Returns a nil form when the template
// declares no fields.
func buildConfigForm(scaffoldConfig *ScaffoldConfig, formValues map[string]interface{}, opts ...FieldFormOption) (*huh.Form, *fieldFormContext, error) {
	if len(scaffoldConfig.Spec.Fields) == 0 {
		return nil, nil, nil
	}

	// Should we run in accessible mode?
	// Note: ACCESSIBLE is a standard environment variable used by the huh form library
	// to enable accessible mode for screen readers. Using viper for consistency.
	v := viper.New()
	_ = v.BindEnv("ACCESSIBLE")
	accessible := v.GetBool("ACCESSIBLE")

	// Store value getters for after form completion. Also consulted (by a
	// hidden field's WithHideFunc closure below) to answer "what has the
	// user entered so far" during the form session itself.
	valueGetters := make(map[string]func() interface{})

	ctx := &fieldFormContext{
		valueGetters:  valueGetters,
		fieldPointers: make(map[string]any),
		fieldsByName:  fieldDefinitionsByName(scaffoldConfig.Spec.Fields),
		render:        resolveFieldFormOptions(opts).render,
		delimiters:    defaultDelimiters(scaffoldConfig.Spec.Delimiters),
		optionsErrors: make(map[string]error),
	}

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
		huhField, getter := createFieldInContext(field.Name, field, formValues, ctx)
		valueGetters[field.Name] = getter

		group := huh.NewGroup(huhField)
		if !field.When.IsZero() {
			group = group.WithHideFunc(fieldHideFunc(field.When, valueGetters))
		}
		groups = append(groups, group)
	}

	huhForm := huh.NewForm(groups...).WithAccessible(accessible)

	return huhForm, ctx, nil
}

// snapshotAnswers reads the current value of every field's getter into a
// plain map, for evaluating a later field's When condition against answers
// collected so far. Getters for fields not yet reached by the user simply
// return their zero/default value, which a well-formed When referencing
// only earlier-declared fields never observes.
func snapshotAnswers(valueGetters map[string]func() interface{}) map[string]any {
	answers := make(map[string]any, len(valueGetters))
	for name, getter := range valueGetters {
		answers[name] = getter()
	}
	return answers
}

// fieldHideFunc builds the huh.Group.WithHideFunc closure for a field
// declaring When: the group is hidden whenever When evaluates false against
// a snapshot of every other field's current answer.
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

// fieldFormContext bundles the form state createField needs to build a
// field whose Options resolve dynamically against other fields' answers:
// valueGetters for reading any field's current value (the same mechanism
// fieldHideFunc uses for When), fieldPointers so a later field's
// OptionsFunc can bind its change-detection to a specific earlier field's
// own bound value regardless of type (a select's *string, a multiselect's
// *[]string, a confirm's *bool), the renderer/delimiters resolveFieldOptions
// needs for the Go-template-expression Options form, and optionsErrors for
// collecting dynamic Options resolution failures encountered during the form
// session (see dynamicOptionsFunc and reportOptionsErrors) so they can be
// reported once the interactive session ends instead of mid-render.
type fieldFormContext struct {
	valueGetters  map[string]func() interface{}
	fieldPointers map[string]any
	fieldsByName  map[string]*FieldDefinition
	render        FieldOptionsRenderer
	delimiters    []string
	optionsErrors map[string]error
}

// toHuhOptions converts resolved options into huh.Option values. Labels
// never reach answers/templates: huh's own Value(&value) binding only ever
// writes back an option's Value, never its label, so this is the only place
// that needs to get label/value pairing right.
func toHuhOptions(options []ResolvedOption) []huh.Option[string] {
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt.Label, opt.Value)
	}
	return huhOptions
}

// dynamicOptionsFunc returns the closure passed to huh's OptionsFunc for a
// field whose Options resolve dynamically (a string: an answers.* dot-path
// or a Go-template expression). It's re-invoked by huh whenever its bound
// optionsBindings change, resolving fresh each time against a snapshot of
// every other field's current answer -- the same snapshotAnswers mechanism
// fieldHideFunc uses for When -- so it resolves against whatever the user
// ultimately answered for an earlier field, since each field is its own
// sequential prompt (huh.Group) and this one is only ever displayed or
// validated after the fields before it are already answered.
//
// A resolution failure is recorded in ctx.optionsErrors rather than logged
// here: this closure runs inside huh's Update loop while the interactive
// form is still on screen, and a log line printed at this point would
// corrupt the current frame. See reportOptionsErrors, which logs the
// collected errors once runFormInteraction returns and the terminal is back
// to normal.
func dynamicOptionsFunc(field *FieldDefinition, ctx *fieldFormContext) func() []huh.Option[string] {
	return func() []huh.Option[string] {
		options, err := resolveFieldOptions(field, snapshotAnswers(ctx.valueGetters), ctx.fieldsByName, ctx.render, ctx.delimiters)
		if err != nil {
			if ctx.optionsErrors == nil {
				ctx.optionsErrors = make(map[string]error)
			}
			ctx.optionsErrors[field.Name] = err
			return nil
		}
		return toHuhOptions(options)
	}
}

// reportOptionsErrors logs any dynamic Options resolution failures collected
// in optionsErrors during the form session (see dynamicOptionsFunc). Callers
// must invoke this only after runFormInteraction returns, once the
// interactive form is no longer on screen, so the log line can't garble the
// terminal frame. Iterates field names in sorted order for deterministic
// output when more than one field failed to resolve.
func reportOptionsErrors(optionsErrors map[string]error) {
	names := make([]string, 0, len(optionsErrors))
	for name := range optionsErrors {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		log.Warn("Failed to resolve dynamic field options", "field", name, "error", optionsErrors[name])
	}
}

// answersReferenceRe matches an "answers.<name>" reference inside a
// Go-template Options expression, e.g. the "envs" in
// `{{ splitList "," answers.envs }}` -- see referencedAnswerNames.
var answersReferenceRe = regexp.MustCompile(`answers\.(\w+)`)

// referencedAnswerNames returns the top-level answer name(s) a dynamic
// Options source string reads, so its huh OptionsFunc bindings can be
// scoped to only the field(s) actually referenced -- not every field in the
// form. This matters for two reasons: it avoids re-resolving (and, for the
// template-expression form, re-rendering) on a totally unrelated field's
// keystroke, and it avoids a field's own bindings ever including its own
// pointer (which would make huh treat the field's own selection as a
// bindings change and clear its own filter text on every selection).
//
// The dot-path form has exactly one referenced name, extracted the same way
// resolveFieldOptionsFromAnswers/validateFieldOptionsSource do. A template
// expression may reference several, found via a plain regex scan rather
// than a text/template parse: pkg/generator/engine's FuncMap registers
// "answers" as a zero-arg function specifically so "answers.X" parses as a
// call+field chain, and real Options expressions almost always also call a
// Sprig/Gomplate function from that same FuncMap -- which pkg/project/config
// cannot import (that package already imports this one; see
// resolveFieldOptions's doc comment for the identical cycle), so parsing
// the expression for real would require function definitions this package
// structurally can't have.
func referencedAnswerNames(source string, delimiters []string) []string {
	if !strings.Contains(source, delimiters[0]) {
		path, ok := strings.CutPrefix(source, answersPrefix)
		if !ok {
			return nil
		}
		root, _, _ := strings.Cut(path, ".")
		return []string{root}
	}

	matches := answersReferenceRe.FindAllStringSubmatch(source, -1)
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		names = append(names, m[1])
	}
	return names
}

// optionsBindings returns the huh OptionsFunc bindings value for a dynamic
// Options field: the pointer(s) for just the field(s) referencedAnswerNames
// finds, looked up in ctx.fieldPointers at the point this field is built --
// i.e. only already-declared fields. A forward reference (a source naming a
// field declared later) simply finds nothing yet and is silently excluded,
// the same no-constraint degradation validateFieldOptionsSource's own doc
// comment already accepts for a forward/preset-sourced dot-path.
//
// The ownField parameter names the field these bindings are being built for,
// and is always excluded even if referencedAnswerNames finds it (a
// self-sourced dot-path, e.g. Options: "answers.envs" on the "envs" field
// itself -- TestSelfReferenceIsTautologicallyValid documents that this is a
// deliberate, valid shape, not a load-time error). Field registration in
// createFieldInContext stores a field's own bound pointer into
// ctx.fieldPointers before resolving its own Options, so without this
// exclusion a self-reference would find its own just-registered pointer and
// huh would treat the field's own selection/keystroke as a bindings change,
// clearing its own filter text -- exactly what the "not every field in the
// form" reasoning above already avoids for other fields.
func optionsBindings(source, ownField string, ctx *fieldFormContext) map[string]any {
	bindings := make(map[string]any)
	for _, name := range referencedAnswerNames(source, ctx.delimiters) {
		if name == ownField {
			continue
		}
		if ptr, ok := ctx.fieldPointers[name]; ok {
			bindings[name] = ptr
		}
	}
	return bindings
}

// staticOptions resolves a field's Options when it isn't a dynamic string
// (i.e. a literal list, or unset) -- never errors, since neither of those
// shapes depends on answers or a renderer.
func staticOptions(field *FieldDefinition, delimiters []string) []huh.Option[string] {
	options, _ := resolveFieldOptions(field, nil, nil, nil, delimiters)
	return toHuhOptions(options)
}

// createField creates a huh field based on the field definition, without
// support for dynamic (answers-referencing) Options -- the entry point for
// isolated single-field unit tests that don't exercise cross-field state,
// since buildConfigForm's real form-building path calls createFieldInContext
// directly instead, threading one fieldFormContext shared across every field
// so a later field's dynamic Options can resolve against an earlier field's
// answer.
func createField(key string, field *FieldDefinition, values map[string]interface{}) (huh.Field, func() interface{}) {
	return createFieldInContext(key, field, values, &fieldFormContext{
		fieldPointers: make(map[string]any),
		delimiters:    defaultDelimiters(nil),
	})
}

// createFieldInContext creates a huh field based on the field definition.
// It returns the field and a function to get the updated value.
//
//nolint:gocognit,revive,cyclop,funlen // complex TUI field factory handling multiple field types
func createFieldInContext(key string, field *FieldDefinition, values map[string]interface{}, ctx *fieldFormContext) (huh.Field, func() interface{}) {
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
		ctx.fieldPointers[key] = &value

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
		ctx.fieldPointers[key] = &value

		selectField := huh.NewSelect[string]().
			Title(fieldTitle(field)).
			Description(field.Description).
			Value(&value)

		if source, dynamic := field.Options.(string); dynamic {
			selectField = selectField.OptionsFunc(dynamicOptionsFunc(field, ctx), optionsBindings(source, key, ctx))
		} else {
			selectField = selectField.Options(staticOptions(field, ctx.delimiters)...)
		}

		if field.Required {
			selectField = selectField.Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("%w: %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
				}
				return validateFieldValue(field, s, optionsResolutionContext{
					answers:      snapshotAnswers(ctx.valueGetters),
					fieldsByName: ctx.fieldsByName,
					render:       ctx.render,
					delimiters:   ctx.delimiters,
				})
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

		// Registered even for a multiselect with static Options: a later
		// field's dynamic Options may reference this one's answer.
		ctx.fieldPointers[key] = &value

		multiSelect := huh.NewMultiSelect[string]().
			Title(fieldTitle(field)).
			Description(field.Description).
			Value(&value).
			Filterable(true)

		if source, dynamic := field.Options.(string); dynamic {
			multiSelect = multiSelect.OptionsFunc(dynamicOptionsFunc(field, ctx), optionsBindings(source, key, ctx))
		} else {
			multiSelect = multiSelect.Options(staticOptions(field, ctx.delimiters)...)
		}

		if field.Required {
			multiSelect = multiSelect.Validate(func(s []string) error {
				if len(s) == 0 {
					return fmt.Errorf("%w: at least one %s", errUtils.ErrGeneratorFieldRequired, fieldTitle(field))
				}
				return validateFieldValue(field, s, optionsResolutionContext{
					answers:      snapshotAnswers(ctx.valueGetters),
					fieldsByName: ctx.fieldsByName,
					render:       ctx.render,
					delimiters:   ctx.delimiters,
				})
			})
		}

		return multiSelect, func() interface{} { return value }

	case "confirm", "bool", "boolean":
		var value bool
		if b, ok := currentValue.(bool); ok {
			value = b
		}
		ctx.fieldPointers[key] = &value

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
		ctx.fieldPointers[key] = &value

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
