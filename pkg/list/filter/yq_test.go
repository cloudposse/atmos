package filter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewYQPredicateFilter_EmptyExpressionFails(t *testing.T) {
	_, err := NewYQPredicateFilter("", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestYQPredicateFilter_Apply(t *testing.T) {
	rows := []map[string]any{
		{"component": "vpc", "stack": "ue2-dev", "vars": map[string]any{"region": "us-east-2"}},
		{"component": "eks", "stack": "ue2-dev", "vars": map[string]any{"region": "us-east-2"}},
		{"component": "vpc", "stack": "uw2-prod", "vars": map[string]any{"region": "us-west-2"}},
	}

	tests := []struct {
		name     string
		expr     string
		wantLen  int
		wantComp map[string]bool // expected component names in result
	}{
		{
			name:     "equality predicate keeps matching rows",
			expr:     `.component == "vpc"`,
			wantLen:  2,
			wantComp: map[string]bool{"vpc": true},
		},
		{
			name:     "nested path predicate",
			expr:     `.vars.region == "us-west-2"`,
			wantLen:  1,
			wantComp: map[string]bool{"vpc": true},
		},
		{
			name:    "predicate matching no rows yields empty result",
			expr:    `.component == "nope"`,
			wantLen: 0,
		},
		{
			name:    "bare path is truthy when set",
			expr:    `.vars.region`,
			wantLen: 3,
		},
		{
			name:    "missing path is falsy",
			expr:    `.does.not.exist`,
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := NewYQPredicateFilter(tc.expr, nil)
			require.NoError(t, err)

			out, err := f.Apply(rows)
			require.NoError(t, err)

			got, ok := out.([]map[string]any)
			require.True(t, ok)
			require.Len(t, got, tc.wantLen)

			if tc.wantComp != nil {
				for _, r := range got {
					assert.True(t, tc.wantComp[r["component"].(string)], "unexpected component %q in result", r["component"])
				}
			}
		})
	}
}

func TestYQPredicateFilter_Apply_InvalidExpression(t *testing.T) {
	rows := []map[string]any{{"a": 1}}
	f := &YQPredicateFilter{Expr: "@@@invalid@@@"}

	_, err := f.Apply(rows)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestYQPredicateFilter_Apply_InvalidData(t *testing.T) {
	f := &YQPredicateFilter{Expr: ".a"}

	_, err := f.Apply("not a slice")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestNewYQProjector_EmptyExpressionFails(t *testing.T) {
	_, err := NewYQProjector("", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestYQProjector_Apply_ScalarResult(t *testing.T) {
	rows := []map[string]any{
		{"component": "vpc", "stack": "ue2-dev", "vars": map[string]any{"region": "us-east-2"}},
		{"component": "eks", "stack": "uw2-prod", "vars": map[string]any{"region": "us-west-2"}},
	}

	p, err := NewYQProjector(".vars.region", nil)
	require.NoError(t, err)

	out, err := p.Apply(rows)
	require.NoError(t, err)

	got, ok := out.([]map[string]any)
	require.True(t, ok)
	require.Len(t, got, 2)
	// stack + component are preserved so default column templates still resolve.
	assert.Equal(t, "vpc", got[0]["component"])
	assert.Equal(t, "ue2-dev", got[0]["stack"])
	assert.Equal(t, "us-east-2", got[0]["value"])
	assert.Equal(t, "us-west-2", got[1]["value"])
}

func TestYQProjector_Apply_MapResult(t *testing.T) {
	rows := []map[string]any{
		{"component": "vpc", "stack": "ue2-dev", "vars": map[string]any{"region": "us-east-2", "namespace": "cp"}},
	}

	p, err := NewYQProjector(`{"region": .vars.region, "ns": .vars.namespace}`, nil)
	require.NoError(t, err)

	out, err := p.Apply(rows)
	require.NoError(t, err)

	got, ok := out.([]map[string]any)
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "vpc", got[0]["component"])
	assert.Equal(t, "ue2-dev", got[0]["stack"])
	assert.Equal(t, "us-east-2", got[0]["region"])
	assert.Equal(t, "cp", got[0]["ns"])
}

func TestYQProjector_Apply_NilResult_DropsRow(t *testing.T) {
	rows := []map[string]any{
		{"component": "vpc", "stack": "ue2-dev", "vars": map[string]any{"region": "us-east-2"}},
		{"component": "eks", "stack": "uw2-prod"}, // no .vars.region — nil result
	}

	p, err := NewYQProjector(".vars.region", nil)
	require.NoError(t, err)

	out, err := p.Apply(rows)
	require.NoError(t, err)

	got, ok := out.([]map[string]any)
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "vpc", got[0]["component"])
}

func TestYQProjector_Apply_InvalidExpression(t *testing.T) {
	rows := []map[string]any{{"a": 1}}
	p := &YQProjector{Expr: "@@@invalid@@@"}

	_, err := p.Apply(rows)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want bool
	}{
		{"nil is falsy", nil, false},
		{"bool true is truthy", true, true},
		{"bool false is falsy", false, false},
		{"empty string is falsy", "", false},
		{`"false" string is falsy`, "false", false},
		{`"null" string is falsy`, "null", false},
		{"non-empty string is truthy", "abc", true},
		{"zero int is falsy", 0, false},
		{"non-zero int is truthy", 1, true},
		{"empty slice is falsy", []any{}, false},
		{"non-empty slice is truthy", []any{"x"}, true},
		{"empty map is falsy", map[string]any{}, false},
		{"non-empty map is truthy", map[string]any{"k": 1}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isTruthy(tc.v))
		})
	}
}

func TestYQPredicateFilter_Matches(t *testing.T) {
	item := map[string]any{
		"component": "vpc",
		"empty":     "",
		"settings": map[string]any{
			"github": map[string]any{"actions_enabled": true},
			"off":    map[string]any{"actions_enabled": false},
		},
		"list": []any{},
	}

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{name: "true comparison", expr: `.component == "vpc"`, want: true},
		{name: "false comparison", expr: `.component == "eks"`, want: false},
		{name: "boolean true value", expr: `.settings.github.actions_enabled`, want: true},
		{name: "boolean false value", expr: `.settings.off.actions_enabled`, want: false},
		{name: "missing path with default false", expr: `.settings.missing.actions_enabled // false`, want: false},
		{name: "missing path is null", expr: `.nope`, want: false},
		{name: "empty string is falsy", expr: `.empty`, want: false},
		{name: "empty slice is falsy", expr: `.list`, want: false},
		{name: "non-empty string is truthy", expr: `.component`, want: true},
		{name: "non-empty map is truthy", expr: `.settings`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewYQPredicateFilter(tt.expr, nil)
			require.NoError(t, err)

			got, err := f.Matches(item)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYQPredicateFilter_Matches_InvalidExpression(t *testing.T) {
	f, err := NewYQPredicateFilter(`.[ not valid`, nil)
	require.NoError(t, err)

	_, err = f.Matches(map[string]any{"component": "vpc"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestYQPredicateFilter_ApplyEqualsMatches(t *testing.T) {
	rows := []map[string]any{
		{"component": "vpc"},
		{"component": "eks"},
		{"component": "vpc"},
	}
	f, err := NewYQPredicateFilter(`.component == "vpc"`, nil)
	require.NoError(t, err)

	var want []map[string]any
	for _, row := range rows {
		ok, matchErr := f.Matches(row)
		require.NoError(t, matchErr)
		if ok {
			want = append(want, row)
		}
	}

	got, err := f.Apply(rows)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
