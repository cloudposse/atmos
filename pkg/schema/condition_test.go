package schema

import (
	"encoding/json"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type conditionConfig struct {
	When Condition `yaml:"when" mapstructure:"when"`
}

func TestConditionUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		ctx    ConditionContext
		want   bool
		assert func(t *testing.T, condition Condition)
	}{
		{
			name: "scalar ci",
			yaml: "when: ci\n",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateSuccess},
			want: true,
			assert: func(t *testing.T, condition Condition) {
				assert.True(t, condition.MentionsAny(ConditionPredicateCI))
				assert.False(t, condition.MentionsAny(ConditionPredicateSuccess))
			},
		},
		{
			name: "scalar local",
			yaml: "when: local\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "scalar never",
			yaml: "when: never\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateSuccess},
			want: false,
		},
		{
			name: "list is all shorthand",
			yaml: "when: [ci, success]\n",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "list all fails when one child fails",
			yaml: "when: [ci, failure]\n",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateSuccess},
			want: false,
		},
		{
			name: "object all",
			yaml: "when:\n  all: [ci, success]\n",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "object any",
			yaml: "when:\n  any: [ci, local]\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "object not",
			yaml: "when:\n  not: ci\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "nested compound",
			yaml: "when:\n  all:\n    - any: [ci, local]\n    - not: failure\n",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "success predicate",
			yaml: "when: success\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "failure predicate",
			yaml: "when: failure\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateFailure},
			want: true,
		},
		{
			name: "unknown predicate is false",
			yaml: "when: unknown\n",
			ctx:  ConditionContext{CI: false, Status: ConditionPredicateSuccess},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg conditionConfig
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &cfg))
			assert.Equal(t, tt.want, cfg.When.Evaluate(tt.ctx))
			if tt.assert != nil {
				tt.assert(t, cfg.When)
			}
		})
	}
}

func TestConditionEmptyMatchesByDefault(t *testing.T) {
	var condition Condition
	assert.True(t, condition.IsZero())
	assert.True(t, condition.Evaluate(ConditionContext{}))
	assert.True(t, condition.EvaluateWithImplicitSuccess(ConditionContext{Status: ConditionPredicateSuccess}))
	assert.False(t, condition.EvaluateWithImplicitSuccess(ConditionContext{Status: ConditionPredicateFailure}))
}

func TestConditionEvaluateWithImplicitSuccess(t *testing.T) {
	tests := []struct {
		name        string
		when        any
		ctx         ConditionContext
		want        bool
		mentions    []string
		notMentions []string
	}{
		{
			name: "ci implies success",
			when: "ci",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateFailure},
			want: false,
		},
		{
			name: "ci runs on success",
			when: "ci",
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateSuccess},
			want: true,
		},
		{
			name: "explicit always removes implicit success",
			when: []any{"ci", "always"},
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateFailure},
			want: true,
		},
		{
			name: "explicit failure removes implicit success",
			when: []any{"ci", "failure"},
			ctx:  ConditionContext{CI: true, Status: ConditionPredicateFailure},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := NewCondition(tt.when)
			require.NoError(t, err)
			assert.Equal(t, tt.want, condition.EvaluateWithImplicitSuccess(tt.ctx))
		})
	}
}

func TestConditionInvalidYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "unknown operator", yaml: "when:\n  expr: true\n"},
		{name: "multiple operators", yaml: "when:\n  all: [ci]\n  any: [local]\n"},
		{name: "invalid scalar type", yaml: "when: 123\n"},
		{name: "not empty", yaml: "when:\n  not:\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg conditionConfig
			require.Error(t, yaml.Unmarshal([]byte(tt.yaml), &cfg))
		})
	}
}

func TestConditionDecodeHook(t *testing.T) {
	var cfg conditionConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &cfg,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			ConditionDecodeHook(),
		),
	})
	require.NoError(t, err)

	require.NoError(t, decoder.Decode(map[string]any{
		"when": map[string]any{"all": []any{"ci", "success"}},
	}))
	assert.True(t, cfg.When.Evaluate(ConditionContext{CI: true, Status: ConditionPredicateSuccess}))
	assert.False(t, cfg.When.Evaluate(ConditionContext{CI: false, Status: ConditionPredicateSuccess}))
}

func TestConditionNewConditionDecodedShapes(t *testing.T) {
	t.Run("nil is empty", func(t *testing.T) {
		condition, err := NewCondition(nil)
		require.NoError(t, err)
		assert.True(t, condition.IsZero())
	})

	t.Run("condition passes through", func(t *testing.T) {
		original := MustCondition("ci")
		condition, err := NewCondition(original)
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(ConditionContext{CI: true}))
	})

	t.Run("empty scalar is empty", func(t *testing.T) {
		condition, err := NewCondition(" ")
		require.NoError(t, err)
		assert.True(t, condition.IsZero())
	})

	t.Run("string slice is all", func(t *testing.T) {
		condition, err := NewCondition([]string{"ci", "success"})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(ConditionContext{CI: true, Status: ConditionPredicateSuccess}))
		assert.False(t, condition.Evaluate(ConditionContext{CI: true, Status: ConditionPredicateFailure}))
	})

	t.Run("any keyed map normalizes", func(t *testing.T) {
		condition, err := NewCondition(map[any]any{"not": "ci"})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(ConditionContext{CI: false}))
	})

	t.Run("all accepts scalar child", func(t *testing.T) {
		condition, err := NewCondition(map[string]any{"all": "ci"})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(ConditionContext{CI: true}))
	})

	t.Run("all accepts empty scalar child as empty", func(t *testing.T) {
		condition, err := NewCondition(map[string]any{"all": ""})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(ConditionContext{}))
	})
}

func TestConditionNewConditionRejectsMalformedDecodedShapes(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "invalid child in any slice", value: []any{"ci", 123}},
		{name: "invalid child in string slice path", value: map[string]any{"all": []any{123}}},
		{name: "invalid child in any operator", value: map[string]any{"any": []any{123}}},
		{name: "invalid child in not operator", value: map[string]any{"not": 123}},
		{name: "unknown operator", value: map[string]any{"expr": "ci"}},
		{name: "multiple operators", value: map[string]any{"all": []any{"ci"}, "any": []any{"local"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCondition(tt.value)
			require.Error(t, err)
		})
	}
}

func TestRegisterConditionPredicate(t *testing.T) {
	RegisterConditionPredicate("custom-test-condition", func(ctx ConditionContext) bool {
		return ctx.Status == "custom"
	})

	condition := MustCondition("custom-test-condition")
	assert.True(t, condition.Evaluate(ConditionContext{Status: "custom"}))
	assert.False(t, condition.Evaluate(ConditionContext{Status: "other"}))
}

func TestConditionJSONRoundTrip(t *testing.T) {
	original := MustCondition(map[string]any{"any": []any{"ci", map[string]any{"not": "failure"}}})
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Condition
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.True(t, decoded.Evaluate(ConditionContext{CI: true, Status: ConditionPredicateFailure}))
	assert.False(t, decoded.Evaluate(ConditionContext{CI: false, Status: ConditionPredicateFailure}))
	assert.True(t, decoded.Evaluate(ConditionContext{CI: false, Status: ConditionPredicateSuccess}))
}

func TestConditionJSONRoundTripAll(t *testing.T) {
	original := MustCondition([]any{"ci", "success"})
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Condition
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Evaluate(ConditionContext{CI: true, Status: ConditionPredicateSuccess}))
	assert.False(t, decoded.Evaluate(ConditionContext{CI: true, Status: ConditionPredicateFailure}))
}

func TestConditionJSONEmptyRoundTrip(t *testing.T) {
	data, err := json.Marshal(Condition{})
	require.NoError(t, err)
	assert.Equal(t, "null", string(data))

	var decoded Condition
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.IsZero())
}

func TestConditionJSONRejectsMalformedInput(t *testing.T) {
	var decoded Condition
	require.Error(t, json.Unmarshal([]byte("{"), &decoded))
}

func TestConditionJSONRejectsInvalidConditionValue(t *testing.T) {
	var decoded Condition
	require.Error(t, json.Unmarshal([]byte("123"), &decoded))
}

func TestConditionMustConditionPanicsOnInvalidInput(t *testing.T) {
	assert.Panics(t, func() {
		MustCondition(123)
	})
}

func TestConditionDefensiveNodeBranches(t *testing.T) {
	t.Run("unknown kind evaluates false", func(t *testing.T) {
		node := ConditionNode{Kind: "unknown"}
		assert.False(t, node.Evaluate(ConditionContext{}))
	})

	t.Run("not without one child evaluates false", func(t *testing.T) {
		node := ConditionNode{Kind: conditionKindNot}
		assert.False(t, node.Evaluate(ConditionContext{}))
	})

	t.Run("unknown kind serializes null", func(t *testing.T) {
		condition := Condition{node: &ConditionNode{Kind: "unknown"}}
		data, err := json.Marshal(condition)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("not without one child serializes null", func(t *testing.T) {
		condition := Condition{node: &ConditionNode{Kind: conditionKindNot}}
		data, err := json.Marshal(condition)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("mentions ignores unknown kind", func(t *testing.T) {
		condition := Condition{node: &ConditionNode{Kind: "unknown"}}
		assert.False(t, condition.MentionsAny(ConditionPredicateCI))
	})

	t.Run("decode hook passes through other target types", func(t *testing.T) {
		var cfg struct {
			Value string `mapstructure:"value"`
		}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:     &cfg,
			DecodeHook: ConditionDecodeHook(),
		})
		require.NoError(t, err)
		require.NoError(t, decoder.Decode(map[string]any{"value": "ci"}))
		assert.Equal(t, "ci", cfg.Value)
	})
}
