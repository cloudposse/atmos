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
