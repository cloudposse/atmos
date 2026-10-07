package starlark

import (
	"context"
	"fmt"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

const configurationStepLimit = 100_000

// EvaluateValue evaluates a function body with a read-only configuration context.
// Wrapping the parsed AST, rather than the source text, preserves YAML line numbers.
func (e *Engine) EvaluateValue(ctx context.Context, spec script.Evaluation) (any, error) {
	defer perf.Track(nil, "starlark.Engine.EvaluateValue")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sink := &itemErrors{}
	globals := starlark.StringDict{
		"ctx":   &configurationMap{values: spec.Context, attributes: true, sink: sink},
		"json":  starjson.Module,
		"sum":   starlark.NewBuiltin("sum", numericSum),
		"round": starlark.NewBuiltin("round", numericRound),
	}
	program, err := configurationProgram(spec, globals)
	if err != nil {
		return nil, scriptError(ctx, err, "")
	}
	thread := &starlark.Thread{Name: spec.Filename, Print: func(_ *starlark.Thread, message string) { log.Debug("Starlark configuration", "message", message) }}
	thread.SetLocal(contextKey, ctx)
	thread.SetMaxExecutionSteps(configurationStepLimit)
	stop := context.AfterFunc(ctx, func() { thread.Cancel(ctx.Err().Error()) })
	defer stop()
	module, err := program.Init(thread, globals)
	if err != nil {
		return nil, scriptError(ctx, err, "")
	}
	value, err := starlark.Call(thread, module["__atmos_value"], nil, nil)
	if err != nil {
		return nil, scriptError(ctx, err, "")
	}
	if sink.err != nil {
		return nil, scriptError(ctx, sink.err, "")
	}
	result, err := configurationResult(value, make(map[starlark.Value]bool))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: !starlark return value: %w", errUtils.ErrStarlark, spec.Filename, err)
	}
	return result, nil
}

func configurationProgram(spec script.Evaluation, globals starlark.StringDict) (*starlark.Program, error) {
	line := spec.Line
	if line < 1 {
		line = 1
	}
	file, err := fileOptions.Parse(spec.Filename, syntax.FilePortion{Content: []byte(spec.Source), FirstLine: line, FirstCol: 1}, 0)
	if err != nil {
		return nil, err
	}
	if len(file.Stmts) == 0 {
		return nil, fmt.Errorf("%w: !starlark requires a function body with a return value", errUtils.ErrStarlark)
	}
	pos, _ := file.Stmts[0].Span()
	file.Stmts = []syntax.Stmt{&syntax.DefStmt{
		Def: pos, Name: &syntax.Ident{NamePos: pos, Name: "__atmos_value"}, Lparen: pos, Rparen: pos, Body: file.Stmts,
	}}
	return starlark.FileProgram(file, globals.Has)
}
