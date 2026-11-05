package policy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCELPredicates(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       bool
		wantErr    error
	}{
		{"match", "input.command == 'plan' && input.status == 'success'", true, nil},
		{"no match", "input.command == 'apply'", false, nil},
		{"parameter", "params.allowed == input.command", true, nil},
		{"no CI", "ci == null", true, nil},
		{"kind and revisions", "request.kind == 'operation' && request.revisions.governing == 'trusted' && request.revisions.base == 'base'", true, nil},
		{"bad syntax", "input.", false, errUtils.ErrPolicyEvaluation},
		{"missing field", "input.missing == 'prod'", false, errUtils.ErrPolicyEvaluation},
		{"non bool", "'true'", false, errUtils.ErrPolicyInvalid},
		{"dynamic non bool", "input.command", false, errUtils.ErrPolicyEvaluation},
		{"dynamic bool", "params.enabled", true, nil},
		{"no automation host", "shell('true')", false, errUtils.ErrPolicyEvaluation},
		{"no legacy ci bool", "ci && true", false, errUtils.ErrPolicyEvaluation},
		{"expression limit", strings.Repeat(" ", maxExpressionBytes+1), false, errUtils.ErrPolicyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := testRequest()
			request.Revisions = Revisions{Governing: "trusted", Base: "base", Head: "candidate"}
			got, err := (&CEL{}).Evaluate(t.Context(), Evaluation{Expression: tc.expression, Request: request, Parameters: json.RawMessage(`{"allowed":"plan","enabled":true}`)})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCELExecutionLimits(t *testing.T) {
	evaluation := Evaluation{Expression: "true", Request: testRequest()}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := (&CEL{}).Evaluate(ctx, evaluation)
	require.ErrorIs(t, err, context.Canceled)
	items := make([]int, 1000)
	data, err := json.Marshal(map[string]any{"items": items})
	require.NoError(t, err)
	evaluation.Request.Input = data
	evaluation.Expression = "input.items.map(x, input.items.map(y, x + y)).size() > 0"
	_, err = (&CEL{}).Evaluate(t.Context(), evaluation)
	require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
	assert.ErrorContains(t, err, "cost limit")
}

func TestCELMalformedPayload(t *testing.T) {
	evaluation := Evaluation{Expression: "true", Request: testRequest(), Parameters: json.RawMessage(`invalid`)}
	_, err := (&CEL{}).Evaluate(t.Context(), evaluation)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
}

func TestJSONNumberPreservation(t *testing.T) {
	object, err := decodeObject(json.RawMessage(`{"signed":9007199254740993,"unsigned":18446744073709551615,"decimal":0.5,"nested":[{"negative":-10},null,true]}`))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"signed": int64(9007199254740993), "unsigned": uint64(18446744073709551615), "decimal": 0.5,
		"nested": []any{map[string]any{"negative": int64(-10)}, nil, true},
	}, object)
	for _, input := range []string{"", "null", "[]", "true", `{}`, `{} {}`, `{"n":1e400}`, `{"n":[18446744073709551616]}`, `{"n":-9223372036854775809}`} {
		t.Run(input, func(t *testing.T) {
			_, err := decodeObject(json.RawMessage(input))
			if input == "{}" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
			}
		})
	}
}
