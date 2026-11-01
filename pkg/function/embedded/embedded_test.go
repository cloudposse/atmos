package embedded

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
)

func envSource(name string) string {
	return yamlsource.Source{Tag: "!env", Argument: name}.Encode()
}

// evaluateByArgument returns values keyed by the source's argument.
func evaluateByArgument(values map[string]any) Evaluator {
	return func(input string) (any, error) {
		source, ok := yamlsource.Decode(input)
		if !ok {
			return input, nil
		}
		return values[source.Argument], nil
	}
}

func TestResolveSubstitutesScalarValues(t *testing.T) {
	evaluate := evaluateByArgument(map[string]any{"S": "text", "B": true, "I": 7, "F": 1.5, "N": nil})

	for _, tt := range []struct{ name, input, want string }{
		{"prefix", "x-" + envSource("S"), "x-text"},
		{"suffix", envSource("S") + "-y", "text-y"},
		{"two values", envSource("S") + "/" + envSource("I"), "text/7"},
		{"bool", "b=" + envSource("B"), "b=true"},
		{"float", "f=" + envSource("F"), "f=1.5"},
		{"null becomes empty", "n=" + envSource("N"), "n="},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.input, evaluate)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveRejectsStructuredValues(t *testing.T) {
	for _, value := range []any{map[string]any{"k": "v"}, []any{"a"}} {
		_, err := Resolve("x-"+envSource("S"), evaluateByArgument(map[string]any{"S": value}))
		require.ErrorIs(t, err, errUtils.ErrYamlFuncTemplateEmbedding)
	}
}

func TestResolveRejectsTransformedSourceWithoutEvaluating(t *testing.T) {
	_, err := Resolve(strings.ToUpper(envSource("S")), func(string) (any, error) {
		t.Fatal("a rewritten source must not be evaluated")
		return nil, nil
	})
	require.ErrorIs(t, err, errUtils.ErrYamlFuncTemplateEmbedding)
}

func TestResolvePropagatesEvaluationErrors(t *testing.T) {
	failure := errors.New("evaluation failed")
	_, err := Resolve("x-"+envSource("S"), func(string) (any, error) { return "ignored", failure })
	require.ErrorIs(t, err, failure)
}

func TestResolveResult(t *testing.T) {
	never := func(string) bool { return false }
	always := func(string) bool { return true }
	mustNotEvaluate := func(string) (any, error) {
		t.Fatal("this result must not be evaluated")
		return nil, nil
	}
	whole := envSource("S")

	t.Run("non-string results pass through", func(t *testing.T) {
		got, err := ResolveResult(7, never, mustNotEvaluate)
		require.NoError(t, err)
		assert.Equal(t, 7, got)
	})
	t.Run("plain text passes through", func(t *testing.T) {
		got, err := ResolveResult("plain", never, mustNotEvaluate)
		require.NoError(t, err)
		assert.Equal(t, "plain", got)
	})
	t.Run("a skipped whole source is left for later", func(t *testing.T) {
		got, err := ResolveResult(whole, always, mustNotEvaluate)
		require.NoError(t, err)
		assert.Equal(t, whole, got)
	})
	for name, input := range map[string]string{"whole source": whole, "embedded source": "x-" + whole} {
		t.Run(name+" is evaluated", func(t *testing.T) {
			got, err := ResolveResult(input, never, func(value string) (any, error) {
				assert.Equal(t, input, value)
				return "resolved", nil
			})
			require.NoError(t, err)
			assert.Equal(t, "resolved", got)
		})
	}
}

func TestIsProvisionalStackName(t *testing.T) {
	transformed := strings.ToUpper(envSource("S"))

	assert.True(t, IsProvisionalStackName("atmos_stack", transformed+"-stack"))
	assert.True(t, IsProvisionalStackName("stack", transformed))
	// Negative paths: any other key, an intact source, plain text, and non-strings.
	assert.False(t, IsProvisionalStackName("name", transformed))
	assert.False(t, IsProvisionalStackName("stack", envSource("S")))
	assert.False(t, IsProvisionalStackName("stack", "dev"))
	assert.False(t, IsProvisionalStackName("stack", 7))
}
