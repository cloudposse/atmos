package value

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

type testEvaluation struct {
	Source  string
	Context ValueMap
}

func evaluateTestValue(_ context.Context, spec testEvaluation) (any, error) {
	globals, sink := Globals(spec.Context)
	program, err := Compile(spec.Source, "test.star", 0, globals)
	if err != nil {
		return nil, err
	}
	thread := &starlark.Thread{Name: "test"}
	thread.SetMaxExecutionSteps(100_000)
	module, err := program.Init(thread, globals)
	if err != nil {
		return nil, err
	}
	result, err := starlark.Call(thread, module["__atmos_value"], nil, nil)
	if err != nil {
		return nil, err
	}
	if sink.Err() != nil {
		return nil, sink.Err()
	}
	return Result(result)
}

func TestProgramCompilationAndEmptyContext(t *testing.T) {
	for _, source := range []string{"", "return (", "load('module.star', 'x')\nreturn x"} {
		_, err := evaluateTestValue(t.Context(), testEvaluation{Source: source})
		require.Error(t, err)
	}
	for _, source := range []string{"return len(ctx) == 0", "return not ctx", "return dict(ctx) == {}", `return str(ctx) == "<configuration>"`} {
		result, err := evaluateTestValue(t.Context(), testEvaluation{Source: source})
		require.NoError(t, err)
		require.Equal(t, true, result)
	}
	_, err := evaluateTestValue(t.Context(), testEvaluation{Source: "return ctx['missing']"})
	require.Error(t, err)
}
