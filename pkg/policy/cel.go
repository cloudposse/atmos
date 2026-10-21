package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/interpreter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CELEngine is the explicit registry key for the CEL predicate adapter.
const CELEngine = "cel"

const (
	defaultCELCostLimit = 100_000
	maxExpressionBytes  = 64 * 1024
	interruptFrequency  = 100
)

// CEL evaluates boolean predicates with no custom functions or I/O capabilities.
// The zero value uses a bounded default runtime cost. Compile size is bounded too.
type CEL struct{}

var _ Evaluator = (*CEL)(nil)

// Evaluate exposes input, params, ci, and request metadata (kind and revisions).
// It intentionally does not reuse workflow/hook when: activations where ci is boolean.
func (c *CEL) Evaluate(ctx context.Context, evaluation Evaluation) (bool, error) {
	defer perf.Track(nil, "policy.CEL.Evaluate")()

	if err := validateRequest(ctx, evaluation.Request); err != nil {
		return false, err
	}
	if len(evaluation.Expression) > maxExpressionBytes {
		return false, fmt.Errorf("%w: CEL expression exceeds size limit", errUtils.ErrPolicyInvalid)
	}
	activation, err := evaluationActivation(evaluation)
	if err != nil {
		return false, err
	}
	program, err := compileCEL(evaluation.Expression)
	if err != nil {
		return false, err
	}
	value, _, err := program.ContextEval(ctx, activation)
	if contextErr := ctx.Err(); contextErr != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrPolicyEvaluation, contextErr)
	}
	if err != nil {
		var canceled interpreter.EvalCancelledError
		if errors.As(err, &canceled) && canceled.Cause == interpreter.CostLimitExceeded {
			return false, fmt.Errorf("%w: CEL cost limit exceeded", errUtils.ErrPolicyEvaluation)
		}
		return false, fmt.Errorf("%w: CEL evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	result, ok := value.(types.Bool)
	if !ok {
		return false, fmt.Errorf("%w: CEL predicate must return bool", errUtils.ErrPolicyEvaluation)
	}
	return bool(result), nil
}

func compileCEL(expression string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Variable("env", cel.DynType),
		cel.Variable("atmos", cel.DynType),
		cel.Variable("runtime", cel.DynType),
		cel.Variable("time", cel.DynType),
		cel.Variable("checks", cel.DynType),
		cel.Variable("input", cel.DynType),
		cel.Variable("params", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("ci", cel.DynType),
		cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: CEL environment: %w", errUtils.ErrPolicyEvaluation, err)
	}
	ast, issues := env.Compile(expression)
	if issues.Err() != nil {
		return nil, fmt.Errorf("%w: CEL compilation: %w", errUtils.ErrPolicyEvaluation, issues.Err())
	}
	if ast.OutputType() != cel.BoolType && ast.OutputType() != cel.DynType {
		return nil, fmt.Errorf("%w: CEL predicate must return bool", errUtils.ErrPolicyInvalid)
	}
	program, err := env.Program(ast, cel.CostLimit(defaultCELCostLimit), cel.InterruptCheckFrequency(interruptFrequency))
	if err != nil {
		return nil, fmt.Errorf("%w: CEL program: %w", errUtils.ErrPolicyEvaluation, err)
	}
	return program, nil
}

func evaluationActivation(evaluation Evaluation) (map[string]any, error) {
	parameters := evaluation.Parameters
	if len(parameters) == 0 {
		parameters = json.RawMessage(`{}`)
	}
	if _, err := decodeObject(parameters); err != nil {
		return nil, err
	}
	// Encoding the entire activation enforces one materialized JSON boundary and
	// preserves unknown provider fields, including integer identifiers.
	data, err := json.Marshal(map[string]any{
		"input":   evaluation.Request.Input,
		"params":  parameters,
		"ci":      evaluation.Request.CI,
		"request": map[string]any{"kind": evaluation.Request.Kind, "revisions": evaluation.Request.Revisions, "subject": evaluation.Request.Subject, "file": evaluation.Request.File},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	activation, err := decodeObject(data)
	if err != nil {
		return nil, err
	}
	return extendActivation(activation, evaluation)
}
