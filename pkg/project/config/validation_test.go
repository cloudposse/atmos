package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
)

func TestValidateFieldValues(t *testing.T) {
	config := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "required_text", Type: "input", Required: true},
		{Name: "pattern", Type: "text", Validation: &FieldValidation{Pattern: "^[a-z]+$", Message: "lowercase only"}},
		{Name: "choice", Type: "select", Options: []string{"one", "two"}},
		{Name: "choices", Type: "multiselect", Required: true, Options: []string{"one", "two"}},
		{Name: "enabled", Type: "boolean", Required: true},
		{Name: "conditional", Type: "input", Required: true, When: condition.Must("answers.enabled == true")},
	}}}

	tests := []struct {
		name                   string
		values                 map[string]interface{}
		wantErr                error
		contains               string
		fallbackPatternMessage bool
	}{
		{
			name: "all supported valid values including false boolean",
			values: map[string]interface{}{
				"required_text": "value", "pattern": "valid", "choice": "one", "choices": []string{"one"}, "enabled": false,
			},
		},
		{
			name: "valid interface slice and active conditional field",
			values: map[string]interface{}{
				"required_text": "value", "pattern": "valid", "choice": "two", "choices": []interface{}{"one", "two"}, "enabled": true, "conditional": "present",
			},
		},
		{
			name:    "missing required key",
			values:  map[string]interface{}{"choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorFieldRequired, contains: "required_text",
		},
		{
			name: "missing required text", values: map[string]interface{}{"required_text": " \t", "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorFieldRequired, contains: "required_text",
		},
		{
			name: "nil required text", values: map[string]interface{}{"required_text": nil, "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorFieldRequired, contains: "required_text",
		},
		{
			name: "empty required multiselect", values: map[string]interface{}{"required_text": "value", "choices": []string{}, "enabled": false},
			wantErr: errUtils.ErrGeneratorFieldRequired, contains: "choices",
		},
		{
			name: "empty interface multiselect", values: map[string]interface{}{"required_text": "value", "choices": []interface{}{}, "enabled": false},
			wantErr: errUtils.ErrGeneratorFieldRequired, contains: "choices",
		},
		{
			name: "pattern custom message", values: map[string]interface{}{"required_text": "value", "pattern": "INVALID1", "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "lowercase only",
		},
		{
			name: "pattern fallback message", values: map[string]interface{}{"required_text": "value", "pattern": "INVALID1", "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "validation.pattern", fallbackPatternMessage: true,
		},
		{
			name: "text field must be a string", values: map[string]interface{}{"required_text": 42, "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "must be text",
		},
		{
			name: "unsupported select option", values: map[string]interface{}{"required_text": "value", "choice": "three", "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "choice",
		},
		{
			name: "select must be a string", values: map[string]interface{}{"required_text": "value", "choice": true, "choices": []string{"one"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "string option",
		},
		{
			name: "unsupported multiselect option", values: map[string]interface{}{"required_text": "value", "choices": []interface{}{"one", "three"}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "choices",
		},
		{
			name: "invalid multiselect item type", values: map[string]interface{}{"required_text": "value", "choices": []interface{}{"one", 2}, "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "choices",
		},
		{
			name: "multiselect must be a list", values: map[string]interface{}{"required_text": "value", "choices": "one", "enabled": false},
			wantErr: errUtils.ErrGeneratorValidation, contains: "list of string options",
		},
		{
			name: "invalid boolean type", values: map[string]interface{}{"required_text": "value", "choices": []string{"one"}, "enabled": 1},
			wantErr: errUtils.ErrGeneratorValidation, contains: "enabled",
		},
		{
			name: "active conditional required field", values: map[string]interface{}{"required_text": "value", "choices": []string{"one"}, "enabled": true},
			wantErr: errUtils.ErrGeneratorFieldRequired, contains: "conditional",
		},
		{
			name: "hidden conditional field is not validated", values: map[string]interface{}{"required_text": "value", "choices": []string{"one"}, "enabled": false, "conditional": 12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fallbackPatternMessage {
				config.Spec.Fields[1].Validation.Message = ""
				t.Cleanup(func() { config.Spec.Fields[1].Validation.Message = "lowercase only" })
			}
			err := ValidateFieldValues(config, tt.values)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), err)
			assert.ErrorContains(t, err, tt.contains)
		})
	}
}

func TestValidateFieldValuesReportsAllInvalidFieldsInFieldOrder(t *testing.T) {
	config := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "first", Type: "select", Options: []string{"one"}},
		{Name: "second", Type: "boolean"},
	}}}

	err := ValidateFieldValues(config, map[string]interface{}{"first": "two", "second": "not-a-bool"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGeneratorValidation), err)
	assert.Equal(t, "generator validation failed: field has unsupported option: field \"first\" option \"two\"; field must be true or false: \"second\"", err.Error())
}

func TestValidateFieldValuesReportsMissingFieldsInFieldOrder(t *testing.T) {
	config := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "first", Type: "input", Required: true},
		{Name: "second", Type: "multiselect", Required: true},
		{Name: "enabled", Type: "boolean", Required: true},
	}}}

	err := ValidateFieldValues(config, map[string]interface{}{"second": []string{}, "enabled": false})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGeneratorFieldRequired), err)
	assert.Equal(t, "field is required: first, second", err.Error())
}

func TestValidateInteractiveTextValue(t *testing.T) {
	tests := []struct {
		name    string
		field   *FieldDefinition
		value   string
		wantErr bool
	}{
		{name: "required whitespace", field: &FieldDefinition{Name: "name", Required: true}, value: " \t", wantErr: true},
		{name: "optional blank", field: &FieldDefinition{Name: "name", Validation: &FieldValidation{Pattern: "^[a-z]+$"}}, value: ""},
		{name: "matching pattern", field: &FieldDefinition{Name: "name", Validation: &FieldValidation{Pattern: "^[a-z]+$"}}, value: "valid"},
		{name: "nonmatching pattern", field: &FieldDefinition{Name: "name", Validation: &FieldValidation{Pattern: "^[a-z]+$"}}, value: "INVALID", wantErr: true},
		{name: "invalid runtime pattern", field: &FieldDefinition{Name: "name", Validation: &FieldValidation{Pattern: "["}}, value: "value", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInteractiveTextValue(tt.field, tt.value)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestValidateFieldValues_DefaultsPresetsPersistedAndFlagsSatisfyRequired(t *testing.T) {
	tests := []struct {
		name       string
		config     *ScaffoldConfig
		persisted  map[string]interface{}
		flagValues map[string]interface{}
	}{
		{
			name:   "default",
			config: &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{{Name: "name", Type: "input", Required: true, Default: "default"}}}},
		},
		{
			name:   "preset",
			config: &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{{Name: "name", Type: "input", Required: true}}, Values: map[string]any{"name": "preset"}}},
		},
		{
			name:      "persisted",
			config:    &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{{Name: "name", Type: "input", Required: true}}}},
			persisted: map[string]interface{}{"name": "persisted"},
		},
		{
			name:       "flag",
			config:     &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{{Name: "name", Type: "input", Required: true}}}},
			flagValues: map[string]interface{}{"name": "flag"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := DeepMerge(tt.config, tt.persisted)
			for key, value := range tt.flagValues {
				values[key] = value
			}
			require.NoError(t, ValidateFieldValues(tt.config, values))
		})
	}
}

func TestValidateFieldValuesRejectsInvalidValuesFromEveryMergedSource(t *testing.T) {
	base := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{{Name: "name", Type: "input", Required: true, Validation: &FieldValidation{Pattern: "^[a-z]+$"}}}}}
	tests := []struct {
		name      string
		config    *ScaffoldConfig
		persisted map[string]interface{}
		flags     map[string]interface{}
	}{
		{name: "default", config: &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{{Name: "name", Type: "input", Required: true, Default: "INVALID", Validation: base.Spec.Fields[0].Validation}}}}},
		{name: "preset", config: &ScaffoldConfig{Spec: ScaffoldSpec{Fields: base.Spec.Fields, Values: map[string]any{"name": "INVALID"}}}},
		{name: "persisted", config: base, persisted: map[string]interface{}{"name": "INVALID"}},
		{name: "flag", config: base, flags: map[string]interface{}{"name": "INVALID"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := DeepMerge(tt.config, tt.persisted)
			for key, value := range tt.flags {
				values[key] = value
			}
			err := ValidateFieldValues(tt.config, values)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrGeneratorValidation), err)
		})
	}
}

func TestPromptForScaffoldConfigRunsCanonicalValidation(t *testing.T) {
	withNoOpFormRunner(t)
	config := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "name", Type: "input", Required: true, Validation: &FieldValidation{Pattern: "^[a-z]+$"}},
		{Name: "choice", Type: "select", Options: []string{"one"}},
	}}}

	err := PromptForScaffoldConfig(config, map[string]interface{}{"name": "INVALID", "choice": "two"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGeneratorValidation), err)
}

func TestLoadScaffoldConfigRejectsInvalidFieldValidation(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		contains string
	}{
		{
			name:     "invalid regex",
			content:  "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n    - name: value\n      type: input\n      validation:\n        pattern: '['\n",
			contains: "invalid validation.pattern",
		},
		{
			name:     "pattern on select",
			content:  "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  fields:\n    - name: value\n      type: select\n      options: [one]\n      validation:\n        pattern: 'one'\n",
			contains: "not text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadScaffoldConfigFromContent(tt.content)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrGeneratorValidation), err)
			assert.ErrorContains(t, err, tt.contains)
		})
	}
}

// TestLoadScaffoldConfigRejectsInvalidFileMatrix covers matrix configurations
// LoadScaffoldConfigFromContent rejects. Most of these are now caught by the
// generated JSON Schema itself (pkg/datafetcher/schema/scaffold/scaffold-config/1.0.json,
// regenerated from FileSpec/MatrixAxes's jsonschema tags in config.go) inside
// manifest.Load, before validateFileMatrix's own Go-level checks ever run --
// so their expected error is the schema sentinel ErrManifestValidation, not
// the more specific matrix sentinel that same bad input would have hit if
// validateFileMatrix were the first (or only) line of defense. A dynamic
// axis's "answers." prefix requirement is the one constraint the schema
// can't express (any generic string satisfies the schema), so it's still
// validateFileMatrix -- via ErrScaffoldMatrixAxisInvalid -- that rejects it.
func TestLoadScaffoldConfigRejectsInvalidFileMatrix(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantErr  error
		contains string
	}{
		{
			name: "matrix without target",
			content: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
				"    - path: deploy.yaml\n      matrix:\n        region: [us-east-1]\n",
			wantErr:  errUtils.ErrManifestValidation,
			contains: "target",
		},
		{
			// The schema's if/then rule only checked that target is present
			// (required), not that it's non-empty -- FileSpec.JSONSchemaExtend
			// in config.go now also constrains the then branch's target with
			// minLength: 1, so an empty target is rejected here too, matching
			// validateFileMatrix's own file.Target == "" check.
			name: "matrix with empty target",
			content: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
				"    - path: deploy.yaml\n      target: \"\"\n      matrix:\n        region: [us-east-1]\n",
			wantErr:  errUtils.ErrManifestValidation,
			contains: "target",
		},
		{
			name: "empty literal axis",
			content: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
				"    - path: deploy.yaml\n      target: out.yaml\n      matrix:\n        region: []\n",
			wantErr:  errUtils.ErrManifestValidation,
			contains: "region",
		},
		{
			name: "dynamic axis missing answers prefix",
			content: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
				"    - path: deploy.yaml\n      target: out.yaml\n      matrix:\n        region: regions\n",
			wantErr:  errUtils.ErrScaffoldMatrixAxisInvalid,
			contains: "region",
		},
		{
			name: "axis of unsupported type",
			content: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
				"    - path: deploy.yaml\n      target: out.yaml\n      matrix:\n        region: 5\n",
			wantErr:  errUtils.ErrManifestValidation,
			contains: "region",
		},
		{
			// A literal axis list with a non-string element (as opposed to
			// "axis of unsupported type" above, where the axis itself isn't
			// a list at all) exercises the schema's array/items branch
			// (MatrixAxes.JSONSchemaExtend in config.go) rather than its
			// oneOf/type branch.
			name: "literal axis list with non-string element",
			content: "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
				"    - path: deploy.yaml\n      target: out.yaml\n      matrix:\n        region: [5]\n",
			wantErr:  errUtils.ErrManifestValidation,
			contains: "region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadScaffoldConfigFromContent(tt.content)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), err)
			assert.ErrorContains(t, err, tt.contains)
		})
	}
}

// TestValidateMatrixAxisValueRejectsNonStringElement is a direct unit test of
// validateMatrixAxisValue's []any branch, called in-package rather than
// through LoadScaffoldConfigFromContent: the same region: [5] input is
// already rejected earlier by JSON Schema validation on that path (see
// "literal axis list with non-string element" above), which would never
// reach this function at all. This test exercises validateMatrixAxisValue
// itself, as a defense-in-depth backstop should it ever be reached from a
// caller that skips schema validation.
func TestValidateMatrixAxisValueRejectsNonStringElement(t *testing.T) {
	err := validateMatrixAxisValue("deploy.yaml", "region", []any{5})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixAxisInvalid)
	assert.ErrorContains(t, err, "region")
}

func TestLoadScaffoldConfigAcceptsValidFileMatrix(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
		"    - path: deploy.yaml\n      target: \"deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml\"\n" +
		"      matrix:\n        environment: [dev, staging, production]\n        region: answers.regions\n"

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
	require.Len(t, scaffoldConfig.Spec.Files, 1)
	assert.Equal(t, "deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml", scaffoldConfig.Spec.Files[0].Target)
}

func TestLoadScaffoldConfigAcceptsTemplateExpressionAxis(t *testing.T) {
	content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n" +
		"    - path: deploy.yaml\n      target: \"deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml\"\n" +
		"      matrix:\n        environment: '{{ keys answers.environments }}'\n" +
		"        region: '{{ keys answers.environments \"regions\" }}'\n"

	scaffoldConfig, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
	require.Len(t, scaffoldConfig.Spec.Files, 1)
	assert.Equal(t, "{{ keys answers.environments }}", scaffoldConfig.Spec.Files[0].Matrix["environment"])
}
