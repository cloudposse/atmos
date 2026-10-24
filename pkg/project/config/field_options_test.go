package config

import (
	"errors"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// plainOptions builds the []ResolvedOption a plain (label-equals-value)
// string list resolves to, for concise test assertions.
func plainOptions(values ...string) []ResolvedOption {
	options := make([]ResolvedOption, len(values))
	for i, v := range values {
		options[i] = ResolvedOption{Label: v, Value: v}
	}
	return options
}

func TestResolveFieldOptions_StaticList(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []string{"dev", "staging", "prod"}}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev", "staging", "prod"), options)
}

func TestResolveFieldOptions_UnsetIsEmpty(t *testing.T) {
	field := &FieldDefinition{Name: "envs"}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Empty(t, options)
}

func TestResolveFieldOptions_YAMLDecodedAnyList(t *testing.T) {
	// A literal YAML list decodes into []any (not []string) once Options is
	// typed any -- this is the shape LoadScaffoldConfigFromContent actually
	// produces for `options: [dev, staging]`, not the []string a Go literal
	// would give in a hand-built test fixture.
	field := &FieldDefinition{Name: "envs", Options: []any{"dev", "staging"}}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev", "staging"), options)
}

func TestResolveFieldOptions_AnyListRejectsNonStringElement(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{"dev", 5}}

	_, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
}

func TestResolveFieldOptions_LabelValueObjectList(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development", "value": "dev"},
		map[string]interface{}{"label": "Staging", "value": "staging"},
	}}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{
		{Label: "Development", Value: "dev"},
		{Label: "Staging", Value: "staging"},
	}, options)
}

func TestResolveFieldOptions_LabelValueObjectListMixedWithPlainStrings(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development", "value": "dev"},
		"staging",
	}}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{
		{Label: "Development", Value: "dev"},
		{Label: "staging", Value: "staging"},
	}, options)
}

func TestResolveFieldOptions_ObjectMissingValueErrors(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development"},
	}}

	_, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
	assert.ErrorContains(t, err, "missing required `value`")
}

func TestResolveFieldOptions_ObjectEmptyValueErrors(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development", "value": ""},
	}}

	_, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
}

func TestResolveFieldOptions_ObjectWithoutLabelDefaultsToValue(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"value": "dev"},
	}}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{{Label: "dev", Value: "dev"}}, options)
}

// TestResolveFieldOptions_ObjectNonStringLabelErrors verifies a non-string
// `label` (e.g. a bare YAML number where quotes were meant) is rejected the
// same way a non-string `value` already is, rather than being silently
// discarded and falling back to the value.
func TestResolveFieldOptions_ObjectNonStringLabelErrors(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": 42, "value": "dev"},
	}}

	_, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
	assert.ErrorContains(t, err, "non-string `label`")
}

// TestUnsupportedOptionError_ListsValidValues verifies the error a user sees
// after e.g. passing a display label instead of its underlying value via
// --set names the actual valid values, rather than a bare "unsupported
// option" with no indication of what would have worked.
func TestUnsupportedOptionError_ListsValidValues(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "envs", Type: "select", Options: []any{
			map[string]interface{}{"label": "Development", "value": "dev"},
			map[string]interface{}{"label": "Staging", "value": "staging"},
		}},
	}}}

	err := ValidateFieldValues(scaffoldConfig, map[string]interface{}{"envs": "Development"})
	require.Error(t, err)
	assert.ErrorContains(t, err, `option "Development" (valid values: dev, staging)`)
}

func TestResolveFieldOptions_DotPathFromPriorMultiselectAnswer(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	answers := map[string]interface{}{"envs": []string{"dev", "staging"}}

	options, err := resolveFieldOptions(field, answers, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev", "staging"), options)
}

func TestResolveFieldOptions_DotPathFromInterfaceSliceAnswer(t *testing.T) {
	// --set/template-preset-supplied answers commonly arrive as
	// []interface{} rather than []string.
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	answers := map[string]interface{}{"envs": []any{"dev", "staging"}}

	options, err := resolveFieldOptions(field, answers, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev", "staging"), options)
}

// TestResolveFieldOptions_DotPathRecoversLabelsFromSourceField verifies "if
// that other answer had labels/values as options, it should present these
// same (but filtered) objects again": envs's static {label,value} pairs are
// recovered for default_env, filtered down to only the values actually
// present in the (simulated) envs answer.
func TestResolveFieldOptions_DotPathRecoversLabelsFromSourceField(t *testing.T) {
	envs := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development", "value": "dev"},
		map[string]interface{}{"label": "Staging", "value": "staging"},
		map[string]interface{}{"label": "Production", "value": "prod"},
	}}
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	fieldsByName := map[string]*FieldDefinition{"envs": envs}
	answers := map[string]interface{}{"envs": []string{"dev", "staging"}}

	options, err := resolveFieldOptions(field, answers, fieldsByName, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{
		{Label: "Development", Value: "dev"},
		{Label: "Staging", Value: "staging"},
	}, options)
}

// TestResolveFieldOptions_DotPathLabelRecoveryFallsBackForUnknownValue
// verifies a value present in the answer but absent from the source
// field's own Options list (e.g. supplied via --set) still resolves,
// falling back to label == value rather than erroring or dropping it.
func TestResolveFieldOptions_DotPathLabelRecoveryFallsBackForUnknownValue(t *testing.T) {
	envs := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development", "value": "dev"},
	}}
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	fieldsByName := map[string]*FieldDefinition{"envs": envs}
	answers := map[string]interface{}{"envs": []string{"dev", "custom-env"}}

	options, err := resolveFieldOptions(field, answers, fieldsByName, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{
		{Label: "Development", Value: "dev"},
		{Label: "custom-env", Value: "custom-env"},
	}, options)
}

// TestResolveFieldOptions_DotPathLabelRecoverySkipsNestedPath verifies label
// recovery only applies to a single-segment answers.<name> dot-path, per
// the documented scope decision -- a deeper path degrades to label == value.
func TestResolveFieldOptions_DotPathLabelRecoverySkipsNestedPath(t *testing.T) {
	envs := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development", "value": "dev"},
	}}
	field := &FieldDefinition{Name: "default_env", Options: "answers.nested.envs"}
	fieldsByName := map[string]*FieldDefinition{"envs": envs}
	answers := map[string]interface{}{"nested": map[string]interface{}{"envs": []string{"dev"}}}

	options, err := resolveFieldOptions(field, answers, fieldsByName, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev"), options)
}

func TestResolveFieldOptions_DotPathMissingSourceErrors(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}

	_, err := resolveFieldOptions(field, map[string]interface{}{}, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceNotFound)
}

func TestResolveFieldOptions_DotPathNonListSourceErrors(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	answers := map[string]interface{}{"envs": "not-a-list"}

	_, err := resolveFieldOptions(field, answers, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceNotList)
}

func TestResolveFieldOptions_DotPathWithoutAnswersPrefixErrors(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: "envs"}

	_, err := resolveFieldOptions(field, map[string]interface{}{"envs": []string{"dev"}}, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
}

func TestResolveFieldOptions_TemplateExpressionUsesRenderer(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: `{{ splitList "," answers.csv_envs }}`}
	answers := map[string]interface{}{"csv_envs": "dev,staging"}

	var gotExpr string
	render := func(expr string, a map[string]interface{}, delimiters []string) ([]string, error) {
		gotExpr = expr
		assert.Equal(t, answers, a)
		assert.Equal(t, defaultDelimiters(nil), delimiters)
		return []string{"dev", "staging"}, nil
	}

	options, err := resolveFieldOptions(field, answers, nil, render, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev", "staging"), options)
	assert.Equal(t, field.Options, gotExpr)
}

func TestResolveFieldOptions_TemplateExpressionWithoutRendererErrors(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: `{{ splitList "," answers.csv_envs }}`}

	_, err := resolveFieldOptions(field, map[string]interface{}{}, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
}

// TestResolveFieldOptions_TemplateExpressionRendererErrorPropagates verifies
// a renderer failure (e.g. the template itself is malformed, or a Sprig/
// Gomplate function call fails) surfaces as-is rather than being swallowed
// or wrapped into a generic invalid-source error.
func TestResolveFieldOptions_TemplateExpressionRendererErrorPropagates(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: `{{ splitList "," answers.csv_envs }}`}
	wantErr := errors.New("template render boom")
	render := func(_ string, _ map[string]interface{}, _ []string) ([]string, error) {
		return nil, wantErr
	}

	_, err := resolveFieldOptions(field, map[string]interface{}{}, nil, render, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestResolveFieldOptions_UnsupportedOptionsTypeErrors verifies a field.Options
// value of a type resolveFieldOptions's type switch doesn't recognize at all
// (not nil, []string, []any, or string) hits the switch's default branch
// rather than silently returning an empty list.
func TestResolveFieldOptions_UnsupportedOptionsTypeErrors(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: 42}

	_, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
}

// TestResolveFieldOptions_ObjectListAcceptsMapInterfaceInterfaceFallback
// covers resolvedOptionFromItem's map[interface{}]interface{} branch -- a
// defensive fallback for any decode path other than yaml.v3's own
// map[string]interface{}, but still directly constructible (and therefore
// worth testing) in Go.
func TestResolveFieldOptions_ObjectListAcceptsMapInterfaceInterfaceFallback(t *testing.T) {
	field := &FieldDefinition{Name: "envs", Options: []any{
		map[interface{}]interface{}{"label": "Development", "value": "dev"},
	}}

	options, err := resolveFieldOptions(field, nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{{Label: "Development", Value: "dev"}}, options)
}

// TestResolveFieldOptions_DotPathNestedIntermediateNotMapErrors verifies a
// nested dot-path whose intermediate segment resolves to a non-map value
// (rather than simply being absent) fails with the same
// errFieldOptionsSourceNotFound as a genuinely missing key.
func TestResolveFieldOptions_DotPathNestedIntermediateNotMapErrors(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: "answers.nested.envs"}
	answers := map[string]interface{}{"nested": "not-a-map"}

	_, err := resolveFieldOptions(field, answers, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceNotFound)
}

// TestResolveFieldOptions_DotPathAnyListRejectsNonStringElement verifies
// answersStringList's []any branch rejects a non-string element in the
// resolved answer (as opposed to TestResolveFieldOptions_AnyListRejectsNonStringElement,
// which covers the same rule for a literal Options list, not an
// answers-sourced one).
func TestResolveFieldOptions_DotPathAnyListRejectsNonStringElement(t *testing.T) {
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	answers := map[string]interface{}{"envs": []any{"dev", 5}}

	_, err := resolveFieldOptions(field, answers, nil, nil, defaultDelimiters(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errFieldOptionsSourceInvalid)
}

// TestResolveFieldOptions_DotPathLabelRecoverySkipsWhenSourceOptionsMalformed
// verifies sourceFieldLabels's third degradation path: the referenced field
// IS declared and its Options IS a static []any list, but that list itself
// fails to resolve (a malformed entry) -- label recovery degrades to
// label == value rather than propagating the source field's own error.
func TestResolveFieldOptions_DotPathLabelRecoverySkipsWhenSourceOptionsMalformed(t *testing.T) {
	envs := &FieldDefinition{Name: "envs", Options: []any{
		map[string]interface{}{"label": "Development"}, // Missing required `value`.
	}}
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}
	fieldsByName := map[string]*FieldDefinition{"envs": envs}
	answers := map[string]interface{}{"envs": []string{"dev"}}

	options, err := resolveFieldOptions(field, answers, fieldsByName, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev"), options)
}

// TestLoadScaffoldConfigRejectsInvalidFieldOptionsSource covers the static,
// scaffold-load-time validation of a dynamic (string) Options value --
// mirroring TestLoadScaffoldConfigRejectsInvalidFieldValidation's style for
// the field-order/prefix rules validateFieldOptionsSource enforces.
func TestLoadScaffoldConfigRejectsInvalidFieldOptionsSource(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: envs\n      type: multiselect\n      options: [dev, staging]\n" +
		"    - name: default_env\n      type: select\n      options: 'envs'\n"

	_, err := LoadScaffoldConfigFromContent(content)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldFieldOptionsInvalid)
	// Explanation/hint text lives on the error's structured detail/hint
	// payload (cockroachdb/errors), not in Error()'s plain string -- assert
	// it through the same accessor the CLI's own error renderer uses (see
	// pkg/pro/wrap_test.go's identical pattern), so this actually proves the
	// hint is attached, not just written in code.
	assert.Contains(t, cockroachErrors.GetAllDetails(err),
		"Field \"default_env\" declares `options: \"envs\"`, which is neither a template expression nor an `answers.*` reference")
	assert.Contains(t, cockroachErrors.GetAllHints(err),
		"Reference a prior answer with the \"answers.\" prefix, e.g. `options: \"answers.environments\"`, or use a Go-template expression")
}

// TestLoadScaffoldConfigRejectsInvalidFieldOptionsList covers the static,
// scaffold-load-time validation of a static Options list's shape (mirroring
// validateMatrixAxisAnyList's precedent for matrix's own []any literal-list
// validation): a malformed {label,value} object surfaces here, not only
// later when someone tries to select from the broken field.
func TestLoadScaffoldConfigRejectsInvalidFieldOptionsList(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: envs\n      type: select\n      options:\n        - label: Development\n"

	_, err := LoadScaffoldConfigFromContent(content)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldFieldOptionsInvalid)
	assert.Contains(t, cockroachErrors.GetAllDetails(err)[0], "missing required `value`")
}

// TestLoadScaffoldConfigAcceptsLabelValueOptionsList is the positive
// counterpart: a well-formed {label,value} list loads successfully.
func TestLoadScaffoldConfigAcceptsLabelValueOptionsList(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: envs\n      type: select\n      options:\n        - label: Development\n          value: dev\n        - label: Staging\n          value: staging\n"

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
	options, err := resolveFieldOptions(&scaffoldConfig.Spec.Fields[0], nil, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, []ResolvedOption{
		{Label: "Development", Value: "dev"},
		{Label: "Staging", Value: "staging"},
	}, options)
}

// TestLoadScaffoldConfigAllowsForwardAndSelfReference documents a
// deliberate design choice: unlike the (now-removed) load-time
// "declared earlier" check, a dot-path's root name is never checked
// against field declaration order at load time, because load time can't
// distinguish a genuine forward/self-reference mistake from a legitimate
// spec.values preset or --set-supplied value that was never declared as a
// field at all (see resolveFieldOptionsFromAnswers's own doc comment, and
// validateFieldOptionsSource's). Both cases below load successfully; their
// actual runtime behavior is covered by TestForwardReferenceResolvesToNoConstraint
// and TestSelfReferenceIsTautologicallyValid.
func TestLoadScaffoldConfigAllowsForwardAndSelfReference(t *testing.T) {
	forwardRef := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: default_env\n      type: select\n      options: 'answers.envs'\n" +
		"    - name: envs\n      type: multiselect\n      options: [dev, staging]\n"
	_, err := LoadScaffoldConfigFromContent(forwardRef)
	require.NoError(t, err)

	selfRef := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: envs\n      type: multiselect\n      options: 'answers.envs'\n"
	_, err = LoadScaffoldConfigFromContent(selfRef)
	require.NoError(t, err)
}

// TestForwardReferenceResolvesToNoConstraint verifies the actual runtime
// behavior of a forward reference (options: "answers.envs" where envs is
// declared later): DeepMerge gives every declared field an explicit map
// entry defaulting to nil, so the not-yet-reached field's answer resolves
// to nil -- which resolveFieldOptionsFromAnswers treats as "no options yet"
// (matching resolveMatrixAxisFromAnswers's identical nil handling), not an
// error. The practical effect: any value is accepted for default_env, since
// an empty resolved option list disables the membership check entirely.
func TestForwardReferenceResolvesToNoConstraint(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "default_env", Type: "select", Options: "answers.envs"},
		{Name: "envs", Type: "multiselect", Options: []string{"dev", "staging"}},
	}}}

	values := DeepMerge(scaffoldConfig, map[string]interface{}{"default_env": "literally-anything"})
	require.NoError(t, ValidateFieldValues(scaffoldConfig, values))
}

// TestSelfReferenceIsTautologicallyValid verifies the actual runtime
// behavior of a field referencing its own answer: by the time
// ValidateFieldValues checks it, the field's own final value IS the
// answer resolveFieldOptionsFromAnswers reads back, so every selected
// item trivially matches its own option list. Not an error, but provides
// no real validation -- documented here so this doesn't read as untested
// behavior.
func TestSelfReferenceIsTautologicallyValid(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "envs", Type: "multiselect", Options: "answers.envs"},
	}}}

	values := DeepMerge(scaffoldConfig, map[string]interface{}{"envs": []string{"dev", "staging"}})
	require.NoError(t, ValidateFieldValues(scaffoldConfig, values))
}

// TestLoadScaffoldConfigAcceptsTemplateExpressionOptionsSource covers
// validateFieldOptionsSource's other accepted shape (a Go-template
// expression containing the delimiter), which
// TestLoadScaffoldConfigAcceptsValidFieldOptionsSource below doesn't
// exercise (it only covers the answers.* dot-path form).
func TestLoadScaffoldConfigAcceptsTemplateExpressionOptionsSource(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: csv_envs\n      type: input\n" +
		"    - name: default_env\n      type: select\n      options: '{{ splitList \",\" answers.csv_envs }}'\n"

	_, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
}

// TestLoadScaffoldConfigAcceptsValidFieldOptionsSource is the positive
// counterpart: a dot-path referencing an earlier field, and a template
// expression, both load and evaluate correctly end to end.
func TestLoadScaffoldConfigAcceptsValidFieldOptionsSource(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n" +
		"    - name: envs\n      type: multiselect\n      options: [dev, staging, prod]\n" +
		"    - name: default_env\n      type: select\n      options: 'answers.envs'\n"

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
	require.Len(t, scaffoldConfig.Spec.Fields, 2)

	options, err := resolveFieldOptions(&scaffoldConfig.Spec.Fields[1], map[string]interface{}{
		"envs": []string{"dev", "staging"},
	}, nil, nil, defaultDelimiters(nil))
	require.NoError(t, err)
	assert.Equal(t, plainOptions("dev", "staging"), options)
}

// TestValidateFieldValues_DynamicOptionsFromPriorAnswer exercises the
// headless --set/--defaults path (ValidateFieldValues, not the interactive
// huh form): a select's value must be one of the environments the user
// actually picked in an earlier multiselect, not the full static universe.
func TestValidateFieldValues_DynamicOptionsFromPriorAnswer(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "envs", Type: "multiselect", Options: []string{"dev", "staging", "prod"}},
		{Name: "default_env", Type: "select", Options: "answers.envs"},
	}}}

	valid := map[string]interface{}{"envs": []string{"dev", "staging"}, "default_env": "staging"}
	require.NoError(t, ValidateFieldValues(scaffoldConfig, valid))

	invalid := map[string]interface{}{"envs": []string{"dev", "staging"}, "default_env": "prod"}
	err := ValidateFieldValues(scaffoldConfig, invalid)
	require.Error(t, err)
	assert.ErrorContains(t, err, "default_env")
}

// TestValidateFieldValues_SelectOptionsResolutionErrorPropagates verifies
// validateSelectValue propagates a resolveFieldOptions failure itself (here,
// the dot-path source "envs" was never supplied at all) rather than
// swallowing it into a generic unsupported-option error.
func TestValidateFieldValues_SelectOptionsResolutionErrorPropagates(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "default_env", Type: "select", Options: "answers.envs"},
	}}}

	err := ValidateFieldValues(scaffoldConfig, map[string]interface{}{"default_env": "dev"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGeneratorValidation)
	assert.ErrorContains(t, err, "default_env")
}

// TestValidateFieldValues_MultiSelectOptionsResolutionErrorPropagates is the
// multiselect counterpart of TestValidateFieldValues_SelectOptionsResolutionErrorPropagates.
func TestValidateFieldValues_MultiSelectOptionsResolutionErrorPropagates(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "regions", Type: "multiselect", Options: "answers.envs"},
	}}}

	err := ValidateFieldValues(scaffoldConfig, map[string]interface{}{"regions": []string{"dev"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGeneratorValidation)
	assert.ErrorContains(t, err, "regions")
}

// TestValidateFieldValues_WithFieldOptionsRendererOption exercises
// WithFieldOptionsRenderer end to end through ValidateFieldValues -- the
// headless --set/--defaults path pkg/generator/ui.validateSetupValues
// actually calls in production -- proving the option threads through
// resolveFieldFormOptions into optionsResolutionContext.render, not just
// that resolveFieldOptions itself accepts a renderer argument directly.
func TestValidateFieldValues_WithFieldOptionsRendererOption(t *testing.T) {
	scaffoldConfig := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "csv_envs", Type: "input"},
		{Name: "default_env", Type: "select", Options: `{{ splitList "," answers.csv_envs }}`},
	}}}
	render := func(_ string, answers map[string]interface{}, _ []string) ([]string, error) {
		csv, _ := answers["csv_envs"].(string)
		return strings.Split(csv, ","), nil
	}

	valid := map[string]interface{}{"csv_envs": "dev,staging", "default_env": "staging"}
	require.NoError(t, ValidateFieldValues(scaffoldConfig, valid, WithFieldOptionsRenderer(render)))

	invalid := map[string]interface{}{"csv_envs": "dev,staging", "default_env": "prod"}
	err := ValidateFieldValues(scaffoldConfig, invalid, WithFieldOptionsRenderer(render))
	require.Error(t, err)
	assert.ErrorContains(t, err, "default_env")
}

// TestDynamicOptionsFunc_ReactsToMultiselectMutation verifies the
// interactive-form reactivity mechanism directly: dynamicOptionsFunc's
// closure re-resolves against the CURRENT contents of a bound multiselect
// value each time it's invoked, matching the re-evaluation huh's
// OptionsFunc performs once the user's selection in an earlier field has
// changed.
func TestDynamicOptionsFunc_ReactsToMultiselectMutation(t *testing.T) {
	envsValue := []string{"dev"}
	ctx := &fieldFormContext{
		valueGetters: map[string]func() interface{}{
			"envs": func() interface{} { return envsValue },
		},
		fieldPointers: map[string]any{"envs": &envsValue},
		delimiters:    defaultDelimiters(nil),
	}
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}

	fn := dynamicOptionsFunc(field, ctx)
	assert.Equal(t, toHuhOptions(plainOptions("dev")), fn())

	envsValue = []string{"dev", "staging"}
	assert.Equal(t, toHuhOptions(plainOptions("dev", "staging")), fn())
}

// TestDynamicOptionsFunc_ResolutionErrorReturnsNilOptions verifies that a
// resolution failure at prompt time (e.g. the source answer isn't
// list-shaped) degrades to an empty option list rather than panicking huh's
// render loop.
func TestDynamicOptionsFunc_ResolutionErrorReturnsNilOptions(t *testing.T) {
	ctx := &fieldFormContext{
		valueGetters: map[string]func() interface{}{
			"envs": func() interface{} { return "not-a-list" },
		},
		fieldPointers: map[string]any{},
		delimiters:    defaultDelimiters(nil),
	}
	field := &FieldDefinition{Name: "default_env", Options: "answers.envs"}

	fn := dynamicOptionsFunc(field, ctx)
	assert.Nil(t, fn())
}

func TestReferencedAnswerNames_DotPath(t *testing.T) {
	names := referencedAnswerNames("answers.envs", defaultDelimiters(nil))
	assert.Equal(t, []string{"envs"}, names)
}

func TestReferencedAnswerNames_DotPathNested(t *testing.T) {
	names := referencedAnswerNames("answers.regions_by_env.dev", defaultDelimiters(nil))
	assert.Equal(t, []string{"regions_by_env"}, names)
}

func TestReferencedAnswerNames_TemplateExpressionSingleReference(t *testing.T) {
	names := referencedAnswerNames(`{{ splitList "," answers.csv_envs }}`, defaultDelimiters(nil))
	assert.Equal(t, []string{"csv_envs"}, names)
}

func TestReferencedAnswerNames_TemplateExpressionMultipleReferencesDeduplicated(t *testing.T) {
	names := referencedAnswerNames(`{{ if answers.use_regions }}{{ answers.regions }}{{ else }}{{ answers.regions }}{{ end }}`, defaultDelimiters(nil))
	assert.ElementsMatch(t, []string{"use_regions", "regions"}, names)
}

func TestReferencedAnswerNames_TemplateExpressionIgnoresMatrixReferences(t *testing.T) {
	names := referencedAnswerNames(`{{ list matrix.environment }}`, defaultDelimiters(nil))
	assert.Empty(t, names)
}

// TestReferencedAnswerNames_UnrecognizedSourceReturnsNil verifies the
// dot-path branch's own defense in depth: a source that's neither a
// template expression (no delimiter) nor an answers.*-prefixed dot-path
// returns nil rather than a garbage root name -- in practice
// validateFieldOptionsSource already rejects this shape at load time, so
// this only guards against a caller invoking referencedAnswerNames directly
// with an unvalidated string.
func TestReferencedAnswerNames_UnrecognizedSourceReturnsNil(t *testing.T) {
	names := referencedAnswerNames("not-a-recognized-source", defaultDelimiters(nil))
	assert.Nil(t, names)
}

func TestOptionsBindings_OnlyIncludesReferencedField(t *testing.T) {
	var envs []string
	var other string
	ctx := &fieldFormContext{
		fieldPointers: map[string]any{"envs": &envs, "other": &other},
		delimiters:    defaultDelimiters(nil),
	}

	bindings := optionsBindings("answers.envs", "default_env", ctx)
	assert.Equal(t, map[string]any{"envs": &envs}, bindings)
}

// TestOptionsBindings_ForwardReferenceIsSilentlyExcluded verifies the
// documented degradation: a source naming a field not yet registered in
// ctx.fieldPointers (declared later, or a typo) simply isn't included,
// rather than panicking or erroring -- consistent with
// validateFieldOptionsSource no longer rejecting this at load time.
func TestOptionsBindings_ForwardReferenceIsSilentlyExcluded(t *testing.T) {
	ctx := &fieldFormContext{
		fieldPointers: map[string]any{},
		delimiters:    defaultDelimiters(nil),
	}

	bindings := optionsBindings("answers.not_yet_declared", "default_env", ctx)
	assert.Empty(t, bindings)
}

// TestOptionsBindings_ExcludesFieldsOwnPointer documents why filtering
// matters beyond avoiding unrelated re-renders: a field whose Options
// dynamically reference a DIFFERENT field never has its own pointer
// included, so its own selection never counts as a bindings change (which
// would otherwise make huh clear the field's own filter text on every
// selection -- see referencedAnswerNames's doc comment).
func TestOptionsBindings_ExcludesFieldsOwnPointer(t *testing.T) {
	var self []string
	var other []string
	ctx := &fieldFormContext{
		fieldPointers: map[string]any{"self": &self, "other": &other},
		delimiters:    defaultDelimiters(nil),
	}

	bindings := optionsBindings("answers.other", "self", ctx)
	assert.NotContains(t, bindings, "self")
	assert.Contains(t, bindings, "other")
}

// TestOptionsBindings_ExcludesSelfReference verifies the actual self-sourced
// case TestSelfReferenceIsTautologicallyValid documents as valid (e.g. an
// "envs" field whose Options is "answers.envs"): createFieldInContext
// registers a field's own bound pointer into ctx.fieldPointers before
// resolving its own Options, so without the ownField exclusion,
// referencedAnswerNames finding the field's own name would make
// optionsBindings include the field's own just-registered pointer --
// which would make huh treat the field's own selection as a bindings
// change and clear its own filter text on every selection.
func TestOptionsBindings_ExcludesSelfReference(t *testing.T) {
	var envs []string
	ctx := &fieldFormContext{
		fieldPointers: map[string]any{"envs": &envs},
		delimiters:    defaultDelimiters(nil),
	}

	bindings := optionsBindings("answers.envs", "envs", ctx)
	assert.Empty(t, bindings)
}
