package condition

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
)

func compileCEL(expr string) (*cel.Ast, error) {
	env, err := conditionCELEnv()
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("%w: invalid CEL expression %q: %w", ErrInvalidWhenCondition, expr, issues.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("%w: CEL expression %q returns %s, expected bool", ErrInvalidWhenCondition, expr, ast.OutputType())
	}
	return ast, nil
}

func conditionCELEnv() (*cel.Env, error) {
	celEnvOnce.Do(func() {
		celEnv, celEnvErr = cel.NewEnv(
			cel.Variable("ci", cel.BoolType),
			cel.Variable("status", cel.StringType),
			cel.Variable("stack", cel.StringType),
			cel.Variable("component", cel.StringType),
			cel.Variable("workflow", cel.StringType),
			cel.Variable("step", cel.StringType),
			cel.Variable("hook", cel.StringType),
			cel.Variable("event", cel.StringType),
			cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)),
		)
		if celEnvErr != nil {
			celEnvErr = fmt.Errorf("%w: failed to initialize CEL environment: %w", ErrInvalidWhenCondition, celEnvErr)
		}
	})
	return celEnv, celEnvErr
}

//nolint:gocritic // Public compatibility API keeps Context by value.
func (ctx Context) activation() map[string]any {
	env := ctx.Env
	if env == nil {
		env = map[string]string{}
	}
	return map[string]any{
		"ci":        ctx.CI,
		"status":    ctx.Status,
		"stack":     ctx.Stack,
		"component": ctx.Component,
		"workflow":  ctx.Workflow,
		"step":      ctx.Step,
		"hook":      ctx.Hook,
		"event":     ctx.Event,
		"env":       env,
	}
}

func celMentionsIdentifier(expr, ident string) bool {
	for _, token := range strings.FieldsFunc(expr, func(r rune) bool {
		return r != '_' &&
			r != '.' &&
			(r < '0' || r > '9') &&
			(r < 'A' || r > 'Z') &&
			(r < 'a' || r > 'z')
	}) {
		if token == ident {
			return true
		}
	}
	return false
}
