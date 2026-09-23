package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
)

// fakeComputedRenderer builds a ComputedFieldRenderer test double that
// looks up expr in a fixed table, so tests can exercise ComputeFields'
// own orchestration (ordering, When gating, error propagation) without
// depending on engine.Processor.RenderAnswersExpression's real Go-template
// evaluation.
func fakeComputedRenderer(t *testing.T, table map[string]any) ComputedFieldRenderer {
	t.Helper()
	return func(expr string, _ map[string]interface{}, _ []string) (any, error) {
		value, ok := table[expr]
		if !ok {
			return nil, errors.New("no fake render entry for expression: " + expr)
		}
		return value, nil
	}
}

func TestComputeFields_WritesValueIntoValues(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "regions", Type: "multiselect"},
		{Name: "primary_region", Type: fieldTypeComputed, Value: "{{ answers.regions }}"},
	}}}
	values := map[string]interface{}{"regions": []string{"us-east-1"}}
	render := fakeComputedRenderer(t, map[string]any{"{{ answers.regions }}": "us-east-1"})

	err := ComputeFields(cfg, values, render)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", values["primary_region"])
}

// TestComputeFields_LaterComputedFieldSeesEarlierResult proves a computed
// field's own result is visible to a later computed field declared after
// it, matching the declared-order dependency rule computed fields document.
func TestComputeFields_LaterComputedFieldSeesEarlierResult(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "first_computed", Type: fieldTypeComputed, Value: "expr-first"},
		{Name: "second_computed", Type: fieldTypeComputed, Value: "expr-second"},
	}}}
	values := map[string]interface{}{}

	var secondSawFirst any
	render := ComputedFieldRenderer(func(expr string, answers map[string]interface{}, _ []string) (any, error) {
		if expr == "expr-first" {
			return "first-value", nil
		}
		secondSawFirst = answers["first_computed"]
		return "second-value", nil
	})

	err := ComputeFields(cfg, values, render)
	require.NoError(t, err)
	assert.Equal(t, "first-value", secondSawFirst)
	assert.Equal(t, "second-value", values["second_computed"])
}

func TestComputeFields_SkipsNonComputedFields(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "regular", Type: "input"},
	}}}
	values := map[string]interface{}{"regular": "unchanged"}

	err := ComputeFields(cfg, values, func(string, map[string]interface{}, []string) (any, error) {
		t.Fatal("render must not be called for a non-computed field")
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, "unchanged", values["regular"])
}

// TestComputeFields_SkipsWhenFalse proves a computed field whose When
// evaluates false is left unset entirely, mirroring how a hidden regular
// field is never prompted for.
func TestComputeFields_SkipsWhenFalse(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "hidden_computed", Type: fieldTypeComputed, Value: "expr", When: condition.Must("answers.enabled == true")},
	}}}
	values := map[string]interface{}{"enabled": false}

	err := ComputeFields(cfg, values, func(string, map[string]interface{}, []string) (any, error) {
		t.Fatal("render must not be called when When evaluates false")
		return nil, nil
	})
	require.NoError(t, err)
	_, exists := values["hidden_computed"]
	assert.False(t, exists)
}

func TestComputeFields_RenderErrorPropagates(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "broken_computed", Type: fieldTypeComputed, Value: "expr"},
	}}}
	values := map[string]interface{}{}
	renderErr := errors.New("boom")

	err := ComputeFields(cfg, values, func(string, map[string]interface{}, []string) (any, error) {
		return nil, renderErr
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, renderErr)
}

func TestComputeFields_NilRendererErrors(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "broken_computed", Type: fieldTypeComputed, Value: "expr"},
	}}}

	err := ComputeFields(cfg, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldComputedFieldInvalid)
}

func TestRejectComputedFieldOverrides(t *testing.T) {
	cfg := &ScaffoldConfig{Spec: ScaffoldSpec{Fields: []FieldDefinition{
		{Name: "regular", Type: "input"},
		{Name: "computed_field", Type: fieldTypeComputed, Value: "expr"},
	}}}

	t.Run("no overrides", func(t *testing.T) {
		err := RejectComputedFieldOverrides(cfg, map[string]interface{}{})
		require.NoError(t, err)
	})

	t.Run("override for a regular field is fine", func(t *testing.T) {
		err := RejectComputedFieldOverrides(cfg, map[string]interface{}{"regular": "value"})
		require.NoError(t, err)
	})

	t.Run("override for a computed field is rejected", func(t *testing.T) {
		err := RejectComputedFieldOverrides(cfg, map[string]interface{}{"computed_field": "value"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrScaffoldComputedFieldNotSettable)
	})
}

func TestValidateComputedFieldDefinition(t *testing.T) {
	tests := []struct {
		name    string
		field   FieldDefinition
		wantErr bool
	}{
		{name: "valid computed field", field: FieldDefinition{Name: "f", Type: fieldTypeComputed, Value: "expr"}},
		{name: "valid regular field", field: FieldDefinition{Name: "f", Type: "input"}},
		{name: "computed without value", field: FieldDefinition{Name: "f", Type: fieldTypeComputed}, wantErr: true},
		{name: "computed with required", field: FieldDefinition{Name: "f", Type: fieldTypeComputed, Value: "expr", Required: true}, wantErr: true},
		{name: "computed with default", field: FieldDefinition{Name: "f", Type: fieldTypeComputed, Value: "expr", Default: "x"}, wantErr: true},
		{name: "non-computed with value", field: FieldDefinition{Name: "f", Type: "input", Value: "expr"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComputedFieldDefinition(&tt.field)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrScaffoldComputedFieldInvalid)
				return
			}
			require.NoError(t, err)
		})
	}
}
