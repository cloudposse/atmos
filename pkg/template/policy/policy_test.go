package policy

import (
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/template/render"
)

func TestPolicyTemplateFuncsWrapsEveryFunctionWhenRulesAreEnabled(t *testing.T) {
	for _, kind := range []string{"function", "source"} {
		t.Run(kind, func(t *testing.T) {
			templatePolicy(t, kind, "false", nil)
			original := template.FuncMap{"upper": strings.ToUpper, "answer": func() int { return 42 }}

			result, err := TemplateFuncs(original, "stack.yaml", &render.Data{})
			require.NoError(t, err)
			require.Len(t, result, len(original))
			assert.NotEqual(t, reflect.ValueOf(original).Pointer(), reflect.ValueOf(result).Pointer(), "a private map is returned, never the cached one")
			assert.Equal(t, reflect.ValueOf(strings.ToUpper).Pointer(), reflect.ValueOf(original["upper"]).Pointer(), "the cached map is not mutated")

			upper, ok := result["upper"].(func(string) (string, error))
			require.True(t, ok, "single-result functions gain an error result: %T", result["upper"])
			value, err := upper("abc")
			require.NoError(t, err)
			assert.Equal(t, "ABC", value)

			answer, ok := result["answer"].(func() (int, error))
			require.True(t, ok, "%T", result["answer"])
			number, err := answer()
			require.NoError(t, err)
			assert.Equal(t, 42, number)
		})
	}
}

func TestPolicyTemplateFuncsCapabilityLookupErrors(t *testing.T) {
	original := template.FuncMap{"upper": strings.ToUpper}
	tests := []struct {
		name       string
		shouldFail func(kind string, call int) bool
	}{
		{"function lookup", func(kind string, _ int) bool { return kind == "function" }},
		{"source lookup", func(kind string, _ int) bool { return kind == "source" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failPolicyEnabled(t, tc.shouldFail)
			result, err := TemplateFuncs(original, "stack.yaml", &render.Data{})
			require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
			assert.Nil(t, result, "a failed lookup must not hand back unguarded functions")
		})
	}
}

func TestAdmitTemplateCallGetHostByNameSource(t *testing.T) {
	templatePolicy(t, "source", "input.scheme == 'dns' && input.host == 'blocked.example.invalid'", nil)

	t.Run("denied host", func(t *testing.T) {
		err := admitTemplateCall("getHostByName", "stack.yaml", []reflect.Value{reflect.ValueOf("blocked.example.invalid")})
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	})

	t.Run("host the rule does not match is admitted", func(t *testing.T) {
		err := admitTemplateCall("getHostByName", "stack.yaml", []reflect.Value{reflect.ValueOf("allowed.example.invalid")})
		require.NoError(t, err)
	})

	t.Run("other functions never consult the DNS source gate", func(t *testing.T) {
		err := admitTemplateCall("upper", "stack.yaml", []reflect.Value{reflect.ValueOf("blocked.example.invalid")})
		require.NoError(t, err)
	})

	t.Run("a call without arguments has no host to check", func(t *testing.T) {
		require.NoError(t, admitTemplateCall("getHostByName", "stack.yaml", nil))
	})
}
