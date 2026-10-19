package policy

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/value"
)

// StarlarkEngine is the registry key for restricted Starlark policy predicates.
const StarlarkEngine = "starlark"

const starlarkStepLimit = 100_000

// Starlark evaluates boolean function bodies over detached, read-only JSON data.
// It has no automation host, module loader, filesystem, network, or logging API.
type Starlark struct{}

var _ Evaluator = (*Starlark)(nil)

// Evaluate returns a strict boolean, never Starlark truthiness or executable effects.
func (s *Starlark) Evaluate(ctx context.Context, evaluation Evaluation) (bool, error) {
	defer perf.Track(nil, "policy.Starlark.Evaluate")()

	if err := validateRequest(ctx, evaluation.Request); err != nil {
		return false, err
	}
	activation, err := evaluationActivation(evaluation)
	if err != nil {
		return false, err
	}
	globals, sink := value.Globals(value.StaticMap(activation))
	// Shadow the universal print builtin. Policy inputs can contain credentials.
	globals["print"] = starlark.None
	program, err := compileStarlark(evaluation.Expression, globals)
	if err != nil {
		return false, err
	}
	result, err := evaluateStarlark(ctx, program, globals)
	if contextErr := ctx.Err(); contextErr != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrPolicyEvaluation, contextErr)
	}
	if err != nil || sink.Err() != nil {
		// Do not expose fail(), runtime values, source text, or traceback details.
		return false, fmt.Errorf("%w: Starlark evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	matched, ok := result.(starlark.Bool)
	if !ok {
		return false, fmt.Errorf("%w: Starlark predicate must return bool", errUtils.ErrPolicyEvaluation)
	}
	return bool(matched), nil
}

func compileStarlark(source string, globals starlark.StringDict) (*starlark.Program, error) {
	if len(source) > maxExpressionBytes {
		return nil, fmt.Errorf("%w: Starlark expression exceeds size limit", errUtils.ErrPolicyInvalid)
	}
	if globals == nil {
		globals, _ = value.Globals(nil)
		globals["print"] = starlark.None
	}
	program, err := value.Compile(source, "policy.star", 1, globals)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid Starlark function body (source details withheld)", errUtils.ErrPolicyInvalid)
	}
	return program, nil
}

func evaluateStarlark(ctx context.Context, program *starlark.Program, globals starlark.StringDict) (starlark.Value, error) {
	thread := &starlark.Thread{Name: "policy", Print: func(*starlark.Thread, string) {}}
	thread.SetMaxExecutionSteps(starlarkStepLimit)
	stop := context.AfterFunc(ctx, func() { thread.Cancel("policy evaluation canceled") })
	defer stop()
	module, err := program.Init(thread, globals)
	if err != nil {
		return nil, err
	}
	return starlark.Call(thread, module["__atmos_value"], nil, nil)
}
