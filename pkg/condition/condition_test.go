package condition

import (
	"encoding/json"
	"errors"
	"runtime"
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
		ctx    Context
		want   bool
		assert func(t *testing.T, condition Condition)
	}{
		{
			name: "scalar ci",
			yaml: "when: ci\n",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: true,
			assert: func(t *testing.T, condition Condition) {
				assert.True(t, condition.MentionsAny(PredicateCI))
				assert.False(t, condition.MentionsAny(PredicateSuccess))
			},
		},
		{
			name: "scalar local",
			yaml: "when: local\n",
			ctx:  Context{CI: false, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "scalar never",
			yaml: "when: never\n",
			ctx:  Context{CI: false, Status: PredicateSuccess},
			want: false,
		},
		{
			name: "list is all shorthand",
			yaml: "when: [ci, success]\n",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "list all fails when one child fails",
			yaml: "when: [ci, failure]\n",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: false,
		},
		{
			name: "object all",
			yaml: "when:\n  all: [ci, success]\n",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "object any",
			yaml: "when:\n  any: [ci, local]\n",
			ctx:  Context{CI: false, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "object not",
			yaml: "when:\n  not: ci\n",
			ctx:  Context{CI: false, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "nested compound",
			yaml: "when:\n  all:\n    - any: [ci, local]\n    - not: failure\n",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "success predicate",
			yaml: "when: success\n",
			ctx:  Context{CI: false, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "failure predicate",
			yaml: "when: failure\n",
			ctx:  Context{CI: false, Status: PredicateFailure},
			want: true,
		},
		{
			name: "bare cel expression",
			yaml: "when: ci && stack == 'prod'\n",
			ctx:  Context{CI: true, Status: PredicateSuccess, Stack: "prod"},
			want: true,
		},
		{
			name: "explicit cel expression",
			yaml: "when: !cel 'ci && status == \"success\"'\n",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: true,
			assert: func(t *testing.T, condition Condition) {
				assert.True(t, condition.MentionsLifecycleStatus())
				assert.False(t, condition.MentionsAny(PredicateSuccess))
			},
		},
		{
			name: "nested cel expression",
			yaml: "when:\n  all:\n    - ci\n    - !cel 'component == \"vpc\"'\n",
			ctx:  Context{CI: true, Status: PredicateSuccess, Component: "vpc"},
			want: true,
		},
		{
			name: "cel false",
			yaml: "when: component == 'vpc'\n",
			ctx:  Context{CI: false, Status: PredicateSuccess, Component: "app"},
			want: false,
		},
		{
			name: "cel env map",
			yaml: "when: env['DEPLOY_ENV'] == 'prod'\n",
			ctx:  Context{Status: PredicateSuccess, Env: map[string]string{"DEPLOY_ENV": "prod"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg conditionConfig
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &cfg))
			got, err := cfg.When.EvaluateE(tt.ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
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
	assert.True(t, condition.Evaluate(Context{}))
	assert.True(t, condition.EvaluateWithImplicitSuccess(Context{Status: PredicateSuccess}))
	assert.False(t, condition.EvaluateWithImplicitSuccess(Context{Status: PredicateFailure}))
}

func TestConditionEvaluateWithImplicitSuccess(t *testing.T) {
	tests := []struct {
		name string
		when any
		ctx  Context
		want bool
	}{
		{
			name: "ci implies success",
			when: "ci",
			ctx:  Context{CI: true, Status: PredicateFailure},
			want: false,
		},
		{
			name: "ci runs on success",
			when: "ci",
			ctx:  Context{CI: true, Status: PredicateSuccess},
			want: true,
		},
		{
			name: "explicit always removes implicit success",
			when: []any{"ci", PredicateAlways},
			ctx:  Context{CI: true, Status: PredicateFailure},
			want: true,
		},
		{
			name: "explicit failure removes implicit success",
			when: []any{"ci", PredicateFailure},
			ctx:  Context{CI: true, Status: PredicateFailure},
			want: true,
		},
		{
			name: "cel without status implies success",
			when: "!cel ci",
			ctx:  Context{CI: true, Status: PredicateFailure},
			want: false,
		},
		{
			name: "cel status removes implicit success",
			when: "!cel status == 'failure'",
			ctx:  Context{CI: true, Status: PredicateFailure},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := New(tt.when)
			require.NoError(t, err)
			got, evalErr := condition.EvaluateWithImplicitSuccessE(tt.ctx)
			require.NoError(t, evalErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConditionEvaluateContinueE(t *testing.T) {
	tests := []struct {
		name string
		cont any
		want bool
	}{
		{
			name: "unset never forgives",
			cont: nil,
			want: false,
		},
		{
			name: "always forgives a failure",
			cont: PredicateAlways,
			want: true,
		},
		{
			name: "explicit failure forgives a failure",
			cont: PredicateFailure,
			want: true,
		},
		{
			name: "success does not forgive a failure",
			cont: PredicateSuccess,
			want: false,
		},
		{
			name: "never does not forgive a failure",
			cont: PredicateNever,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cond Condition
			if tt.cont != nil {
				var err error
				cond, err = New(tt.cont)
				require.NoError(t, err)
			}
			got, err := cond.EvaluateContinueE(Context{Status: PredicateFailure})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConditionEvaluateContinueEPropagatesMalformedCEL(t *testing.T) {
	// Bypasses New's compile-time validation (which would normally catch this at config-load
	// time) to exercise EvaluateContinueE's own error propagation for a runtime CEL failure —
	// same technique as TestConditionEvaluateEReturnsRuntimeError.
	cond := Condition{node: &Node{Kind: kindCEL, Expr: "unknown"}}
	_, evalErr := cond.EvaluateContinueE(Context{Status: PredicateFailure})
	require.Error(t, evalErr, "malformed continue: CEL must hard-fail, never be silently forgiven")
	assert.True(t, errors.Is(evalErr, ErrInvalidWhenCondition))
}

func TestConditionEvaluate_PlatformFacts(t *testing.T) {
	ctx := Context{
		Status:   PredicateSuccess,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}

	matchOS, err := New("!cel os == '" + runtime.GOOS + "'")
	require.NoError(t, err)
	assert.True(t, matchOS.Evaluate(ctx))

	mismatchOS, err := New("!cel os == 'not-a-real-os'")
	require.NoError(t, err)
	assert.False(t, mismatchOS.Evaluate(ctx))

	combined, err := New("!cel os == '" + runtime.GOOS + "' && arch == '" + runtime.GOARCH + "'")
	require.NoError(t, err)
	assert.True(t, combined.Evaluate(ctx))

	platform, err := New("!cel platform == '" + runtime.GOOS + "/" + runtime.GOARCH + "'")
	require.NoError(t, err)
	assert.True(t, platform.Evaluate(ctx))
}

func TestConditionEvaluate_PreconditionFact(t *testing.T) {
	success, err := New("!cel precondition.success")
	require.NoError(t, err)
	assert.True(t, success.Evaluate(Context{PreconditionSuccess: true}))
	assert.False(t, success.Evaluate(Context{PreconditionSuccess: false}))

	negated, err := New("!cel !precondition.success")
	require.NoError(t, err)
	assert.False(t, negated.Evaluate(Context{PreconditionSuccess: true}))
	assert.True(t, negated.Evaluate(Context{PreconditionSuccess: false}))
}

func TestConditionEvaluate_StructuredSourcesArtifactsFacts(t *testing.T) {
	ctx := Context{
		Sources:   []FileFact{{Path: "a.go", Mtime: 200, Checksum: "hash-a"}},
		Artifacts: []FileFact{{Path: "bin/app", Mtime: 100, Checksum: "hash-b"}},
	}

	stale, err := New("!cel sources.exists(s, artifacts.all(a, s.mtime > a.mtime))")
	require.NoError(t, err)
	assert.True(t, stale.Evaluate(ctx), "a source newer than every artifact must report stale")

	notStale, err := New("!cel sources.exists(s, artifacts.all(a, s.mtime < a.mtime))")
	require.NoError(t, err)
	assert.False(t, notStale.Evaluate(ctx))

	pathAccess, err := New("!cel sources[0].path == 'a.go' && artifacts[0].checksum == 'hash-b'")
	require.NoError(t, err)
	assert.True(t, pathAccess.Evaluate(ctx))

	sizeCheck, err := New("!cel size(sources) > 0")
	require.NoError(t, err)
	assert.True(t, sizeCheck.Evaluate(ctx))
}

func TestConditionMentionsCELIdentifier(t *testing.T) {
	cond, err := New("!cel checksum.changed && precondition.success")
	require.NoError(t, err)

	assert.True(t, cond.MentionsCELIdentifier("checksum"))
	assert.True(t, cond.MentionsCELIdentifier("precondition"))
	assert.False(t, cond.MentionsCELIdentifier("timestamp"))
	assert.False(t, Condition{}.MentionsCELIdentifier("checksum"), "a zero-value condition mentions nothing")
}

func TestConditionEvaluate_EmptySourcesArtifactsActivateCleanly(t *testing.T) {
	// A step with no inputs.sources/artifacts.paths declared must still evaluate a `when:`
	// expression that references sources/artifacts (as an empty list), not panic or error --
	// CEL's list adapter rejects a nil slice, so the zero Context must produce non-nil empties.
	cond, err := New("!cel sources == [] && artifacts == []")
	require.NoError(t, err)
	assert.True(t, cond.Evaluate(Context{}))
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
		{name: "invalid cel", yaml: "when: stack ==\n"},
		{name: "unknown cel variable", yaml: "when: unknown\n"},
		{name: "cel returns string", yaml: "when: stack\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg conditionConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidWhenCondition)
		})
	}
}

func TestConditionDecodeHook(t *testing.T) {
	var cfg conditionConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &cfg,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			DecodeHook(),
		),
	})
	require.NoError(t, err)

	require.NoError(t, decoder.Decode(map[string]any{
		"when": map[string]any{"all": []any{"ci", "success"}},
	}))
	assert.True(t, cfg.When.Evaluate(Context{CI: true, Status: PredicateSuccess}))
	assert.False(t, cfg.When.Evaluate(Context{CI: false, Status: PredicateSuccess}))
}

func TestConditionNewDecodedShapes(t *testing.T) {
	t.Run("nil is empty", func(t *testing.T) {
		condition, err := New(nil)
		require.NoError(t, err)
		assert.True(t, condition.IsZero())
	})

	t.Run("condition passes through", func(t *testing.T) {
		original := Must("ci")
		condition, err := New(original)
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(Context{CI: true}))
	})

	t.Run("empty scalar is empty", func(t *testing.T) {
		condition, err := New(" ")
		require.NoError(t, err)
		assert.True(t, condition.IsZero())
	})

	t.Run("string slice is all", func(t *testing.T) {
		condition, err := New([]string{"ci", "success"})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(Context{CI: true, Status: PredicateSuccess}))
		assert.False(t, condition.Evaluate(Context{CI: true, Status: PredicateFailure}))
	})

	t.Run("any keyed map normalizes", func(t *testing.T) {
		condition, err := New(map[any]any{"not": "ci"})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(Context{CI: false}))
	})

	t.Run("all accepts scalar child", func(t *testing.T) {
		condition, err := New(map[string]any{"all": "ci"})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(Context{CI: true}))
	})

	t.Run("all accepts empty scalar child as empty", func(t *testing.T) {
		condition, err := New(map[string]any{"all": ""})
		require.NoError(t, err)
		assert.True(t, condition.Evaluate(Context{}))
	})
}

func TestConditionNewRejectsMalformedDecodedShapes(t *testing.T) {
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
		{name: "malformed cel", value: "stack =="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.value)
			require.Error(t, err)
		})
	}
}

func TestRegisterPredicate(t *testing.T) {
	name := "custom-test-condition"
	predicatesMu.Lock()
	previous, hadPrevious := predicates[name]
	predicatesMu.Unlock()
	t.Cleanup(func() {
		predicatesMu.Lock()
		defer predicatesMu.Unlock()
		if hadPrevious {
			predicates[name] = previous
			return
		}
		delete(predicates, name)
	})

	RegisterPredicate("custom-test-condition", func(ctx Context) bool {
		return ctx.Status == "custom"
	})

	condition := Must("custom-test-condition")
	assert.True(t, condition.Evaluate(Context{Status: "custom"}))
	assert.False(t, condition.Evaluate(Context{Status: "other"}))
}

func TestValidateStep(t *testing.T) {
	require.NoError(t, ValidateStep(Must([]any{"ci", "success"})))
	require.NoError(t, ValidateStep(Must("failure")))
	require.NoError(t, ValidateStep(Must(map[string]any{"not": "failure"})))
	require.NoError(t, ValidateStep(Must("status == 'failure'")))
}

func TestConditionJSONRoundTrip(t *testing.T) {
	original := Must(map[string]any{"any": []any{"ci", map[string]any{"not": "failure"}}})
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Condition
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.True(t, decoded.Evaluate(Context{CI: true, Status: PredicateFailure}))
	assert.False(t, decoded.Evaluate(Context{CI: false, Status: PredicateFailure}))
	assert.True(t, decoded.Evaluate(Context{CI: false, Status: PredicateSuccess}))
}

func TestConditionJSONRoundTripCEL(t *testing.T) {
	original := Must("stack == 'prod' && ci")
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Condition
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Evaluate(Context{CI: true, Stack: "prod"}))
	assert.False(t, decoded.Evaluate(Context{CI: false, Stack: "prod"}))
}

func TestConditionJSONRoundTripAll(t *testing.T) {
	original := Must([]any{"ci", "success"})
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Condition
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Evaluate(Context{CI: true, Status: PredicateSuccess}))
	assert.False(t, decoded.Evaluate(Context{CI: true, Status: PredicateFailure}))
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
	err := json.Unmarshal([]byte("123"), &decoded)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidWhenCondition)
}

func TestConditionMustPanicsOnInvalidInput(t *testing.T) {
	assert.Panics(t, func() {
		Must(123)
	})
}

func TestConditionDefensiveNodeBranches(t *testing.T) {
	t.Run("unknown kind evaluates false", func(t *testing.T) {
		node := Node{Kind: "unknown"}
		assert.False(t, node.Evaluate(Context{}))
	})

	t.Run("not without one child evaluates false", func(t *testing.T) {
		node := Node{Kind: kindNot}
		assert.False(t, node.Evaluate(Context{}))
	})

	t.Run("unknown kind serializes null", func(t *testing.T) {
		condition := Condition{node: &Node{Kind: "unknown"}}
		data, err := json.Marshal(condition)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("not without one child serializes null", func(t *testing.T) {
		condition := Condition{node: &Node{Kind: kindNot}}
		data, err := json.Marshal(condition)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("mentions ignores unknown kind", func(t *testing.T) {
		condition := Condition{node: &Node{Kind: "unknown"}}
		assert.False(t, condition.MentionsAny(PredicateCI))
	})

	t.Run("decode hook passes through other target types", func(t *testing.T) {
		var cfg struct {
			Value string `mapstructure:"value"`
		}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:     &cfg,
			DecodeHook: DecodeHook(),
		})
		require.NoError(t, err)
		require.NoError(t, decoder.Decode(map[string]any{"value": "ci"}))
		assert.Equal(t, "ci", cfg.Value)
	})
}

func TestConditionEvaluateEReturnsRuntimeError(t *testing.T) {
	condition := Condition{node: &Node{Kind: kindCEL, Expr: "ci"}}
	ok, err := condition.EvaluateE(Context{CI: true})
	require.NoError(t, err)
	assert.True(t, ok)

	condition = Condition{node: &Node{Kind: kindCEL, Expr: "unknown"}}
	ok, err = condition.EvaluateE(Context{})
	require.Error(t, err)
	assert.False(t, ok)
	assert.True(t, errors.Is(err, ErrInvalidWhenCondition))
}
