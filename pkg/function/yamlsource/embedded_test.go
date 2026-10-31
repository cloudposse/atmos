package yamlsource

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRequiresOneCompleteSource(t *testing.T) {
	env := Source{Tag: "!env", Argument: "STAGE"}.Encode()
	exec := Source{Tag: "!exec", Argument: "echo: a:b"}.Encode()

	source, ok := Decode(exec)
	require.True(t, ok)
	assert.Equal(t, "!exec", source.Tag)
	assert.Equal(t, "echo: a:b", source.Argument)

	// Text after a source used to be read as part of its argument.
	for _, value := range []string{env + "-post", "pre-" + env, env + "/" + exec, env + env, env[:len(env)-1]} {
		_, ok := Decode(value)
		assert.False(t, ok, value)
	}
}

func TestDecodeKeepsNestedSourceInArgument(t *testing.T) {
	inner := Source{Tag: "!env", Argument: "STAGE"}.Encode()
	outer := Source{Tag: "!template", Argument: "a-" + inner + "-b"}.Encode()

	source, ok := Decode(outer)
	require.True(t, ok)
	assert.Equal(t, "!template", source.Tag)
	assert.Equal(t, "a-"+inner+"-b", source.Argument)
}

func TestEmbedded(t *testing.T) {
	env := Source{Tag: "!env", Argument: "STAGE"}.Encode()
	for _, value := range []string{"pre-" + env, env + "-post", "pre-" + env + "-post", env + env, strings.ToUpper(env)} {
		assert.True(t, Embedded(value), value)
	}
	for _, value := range []string{env, "plain", "!env STAGE", prefix + "invalid", "!atmos-yaml-source:old-process:eyJ0YWciOiIhZW52In0:NAME"} {
		assert.False(t, Embedded(value), value)
	}
}

func TestReplaceSubstitutesEverySourceInOrder(t *testing.T) {
	env := Source{Tag: "!env", Argument: "STAGE"}.Encode()
	exec := Source{Tag: "!exec", Argument: "echo hi"}.Encode()
	var seen []string

	result, err := Replace("a-"+env+"/"+exec+"-"+env+"-z", func(token string, source Source) (string, error) {
		seen = append(seen, token)
		return "<" + source.Tag + ">", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "a-<!env>/<!exec>-<!env>-z", result)
	require.Len(t, seen, 3)
	assert.Equal(t, env, seen[0])
	assert.Equal(t, exec, seen[1])
	assert.Equal(t, env, seen[2])
}

func TestReplacePropagatesErrorsAndLeavesPlainText(t *testing.T) {
	failure := errors.New("resolver failed")
	env := Source{Tag: "!env", Argument: "STAGE"}.Encode()

	_, err := Replace("x-"+env, func(string, Source) (string, error) { return "", failure })
	require.ErrorIs(t, err, failure)

	plain, err := Replace("no sources", func(string, Source) (string, error) {
		t.Fatal("resolver must not run without a source")
		return "", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "no sources", plain)
}

func TestExpressionShowsEmbeddedSourceReadably(t *testing.T) {
	env := Source{Tag: "!env", Argument: "STAGE", File: "/abs/stack.yaml", Line: 3}.Encode()

	assert.Equal(t, "!env STAGE", Expression(env))
	assert.Equal(t, "x-!env STAGE-y", Expression("x-"+env+"-y"))
	assert.Equal(t, "x-!env STAGE", Display("x-"+env))
	assert.NotContains(t, Expression("x-"+env), "abs/stack.yaml")
}

func TestContainsTagFindsEverySource(t *testing.T) {
	env := Source{Tag: "!env", Argument: "STAGE"}.Encode()
	exec := Source{Tag: "!exec", Argument: "echo hi"}.Encode()
	value := "a-" + env + "/" + exec + "-z"

	assert.True(t, ContainsTag(value, "!env"))
	assert.True(t, ContainsTag(value, "!exec"))
	assert.False(t, ContainsTag(value, "!store"))
	assert.False(t, ContainsTag("plain", "!env"))
}
