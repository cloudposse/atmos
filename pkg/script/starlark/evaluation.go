package starlark

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
	"github.com/cloudposse/atmos/pkg/script/starlark/value"
)

const configurationStepLimit = 100_000

// EvaluateValue evaluates a function body with a read-only configuration context.
// Wrapping the parsed AST, rather than the source text, preserves YAML line numbers.
func (e *Engine) EvaluateValue(ctx context.Context, spec script.Evaluation) (any, error) {
	defer perf.Track(nil, "starlark.Engine.EvaluateValue")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	globals, sink := value.Globals(spec.Context)
	globals["sum"] = starlark.NewBuiltin("sum", numericSum)
	globals["round"] = starlark.NewBuiltin("round", numericRound)
	globals["fail"] = starlark.NewBuiltin("fail", raiseFailure)
	program, err := value.Compile(spec.Source, spec.Filename, spec.Line, globals)
	if err != nil {
		return nil, scriptError(ctx, err, "")
	}
	thread := &starlark.Thread{Name: spec.Filename, Print: func(_ *starlark.Thread, message string) { log.Debug("Starlark configuration", "message", message) }}
	threadcontext.Set(ctx, thread)
	thread.SetMaxExecutionSteps(configurationStepLimit)
	stop := context.AfterFunc(ctx, func() { thread.Cancel(ctx.Err().Error()) })
	defer stop()
	module, err := program.Init(thread, globals)
	if err != nil {
		return nil, scriptError(ctx, err, "")
	}
	resultValue, err := starlark.Call(thread, module["__atmos_value"], nil, nil)
	if err != nil {
		return nil, scriptError(ctx, err, "")
	}
	if sink.Err() != nil {
		return nil, scriptError(ctx, sink.Err(), "")
	}
	result, err := value.Result(resultValue)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: !starlark return value: %w", errUtils.ErrStarlark, spec.Filename, err)
	}
	return result, nil
}
