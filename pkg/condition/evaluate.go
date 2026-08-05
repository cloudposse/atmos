package condition

import (
	"fmt"
	"strings"
)

// IsZero reports whether no condition was configured.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func (c Condition) IsZero() bool {
	return c.node == nil
}

// Evaluate returns whether the condition matches the supplied context. Empty
// conditions match by default. Evaluation errors return false; callers that
// need diagnostics should use EvaluateE.
//
//nolint:gocritic,lintroller // Public compatibility API keeps Context by value; condition cannot import perf.
func (c Condition) Evaluate(ctx Context) bool {
	ok, err := c.EvaluateE(ctx)
	return err == nil && ok
}

// EvaluateE returns whether the condition matches the supplied context and
// reports CEL runtime errors.
//
//nolint:gocritic,lintroller // Public compatibility API keeps Context by value; condition cannot import perf.
func (c Condition) EvaluateE(ctx Context) (bool, error) {
	if c.node == nil {
		return true, nil
	}
	return c.node.EvaluateE(ctx)
}

// EvaluateWithImplicitSuccess applies the hook-specific default: a condition
// that does not mention lifecycle status also requires success.
//
//nolint:gocritic,lintroller // Public compatibility API keeps Context by value; condition cannot import perf.
func (c Condition) EvaluateWithImplicitSuccess(ctx Context) bool {
	ok, err := c.EvaluateWithImplicitSuccessE(ctx)
	return err == nil && ok
}

// EvaluateWithImplicitSuccessE is EvaluateWithImplicitSuccess with error reporting.
//
//nolint:gocritic,lintroller // Public compatibility API keeps Context by value; condition cannot import perf.
func (c Condition) EvaluateWithImplicitSuccessE(ctx Context) (bool, error) {
	if c.node == nil {
		return ctx.Status == PredicateSuccess, nil
	}
	if !c.MentionsLifecycleStatus() && ctx.Status != PredicateSuccess {
		return false, nil
	}
	return c.EvaluateE(ctx)
}

// MentionsAny reports whether any predicate with one of the supplied names is
// present in the condition tree.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func (c Condition) MentionsAny(names ...string) bool {
	if c.node == nil {
		return false
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return c.node.mentionsAny(wanted)
}

// MentionsLifecycleStatus reports whether the condition explicitly reasons
// about lifecycle status, either through a status predicate or a CEL `status`
// reference.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func (c Condition) MentionsLifecycleStatus() bool {
	if c.node == nil {
		return false
	}
	return c.MentionsAny(PredicateSuccess, PredicateFailure, PredicateAlways) || c.node.mentionsCELStatus()
}

// ValidateStep validates predicates used by workflow and custom command steps.
// Step runners evaluate lifecycle predicates against the current run status so
// cleanup steps can use `failure` and `always`.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func ValidateStep(condition Condition) error {
	return nil
}

//nolint:gocritic,lintroller // Node methods keep value receivers for existing tests and aliases; condition cannot import perf.
func (n Node) Evaluate(ctx Context) bool {
	ok, err := n.EvaluateE(ctx)
	return err == nil && ok
}

//nolint:gocritic,lintroller // Node methods keep value receivers for existing tests and aliases; condition cannot import perf.
func (n Node) EvaluateE(ctx Context) (bool, error) {
	switch n.Kind {
	case kindPredicate:
		return n.evaluatePredicate(ctx), nil
	case kindCEL:
		return n.evaluateCEL(ctx)
	case kindAll:
		return n.evaluateAll(ctx)
	case kindAny:
		return n.evaluateAny(ctx)
	case kindNot:
		if len(n.Children) != 1 {
			return false, nil
		}
		ok, err := n.Children[0].EvaluateE(ctx)
		return !ok, err
	default:
		return false, nil
	}
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) evaluatePredicate(ctx Context) bool {
	predicatesMu.RLock()
	fn := predicates[n.Name]
	predicatesMu.RUnlock()
	if fn == nil {
		return false
	}
	return fn(ctx)
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) evaluateCEL(ctx Context) (bool, error) {
	ast := n.celAst
	if ast == nil {
		var err error
		ast, err = compileCEL(n.Expr)
		if err != nil {
			return false, err
		}
	}
	env, err := conditionCELEnv()
	if err != nil {
		return false, err
	}
	program, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("%w: failed to create CEL program for %q: %w", ErrInvalidWhenCondition, n.Expr, err)
	}
	out, _, err := program.Eval(ctx.activation())
	if err != nil {
		return false, fmt.Errorf("%w: failed to evaluate CEL expression %q: %w", ErrInvalidWhenCondition, n.Expr, err)
	}
	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("%w: CEL expression %q returned %T, expected bool", ErrInvalidWhenCondition, n.Expr, out.Value())
	}
	return result, nil
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) evaluateAll(ctx Context) (bool, error) {
	for _, child := range n.Children {
		ok, err := child.EvaluateE(ctx)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) evaluateAny(ctx Context) (bool, error) {
	for _, child := range n.Children {
		ok, err := child.EvaluateE(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) mentionsAny(names map[string]struct{}) bool {
	if n.Kind == kindPredicate {
		_, ok := names[n.Name]
		return ok
	}
	for _, child := range n.Children {
		if child.mentionsAny(names) {
			return true
		}
	}
	return false
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) mentionsCELStatus() bool {
	if n.Kind == kindCEL {
		return celMentionsIdentifier(n.Expr, "status")
	}
	for _, child := range n.Children {
		if child.mentionsCELStatus() {
			return true
		}
	}
	return false
}

//nolint:gocritic // Node methods keep value receivers for existing tests and aliases.
func (n Node) value() any {
	switch n.Kind {
	case kindPredicate:
		return n.Name
	case kindCEL:
		return CELTag + " " + n.Expr
	case kindAll:
		values := make([]any, 0, len(n.Children))
		for _, child := range n.Children {
			values = append(values, child.value())
		}
		return map[string]any{kindAll: values}
	case kindAny:
		values := make([]any, 0, len(n.Children))
		for _, child := range n.Children {
			values = append(values, child.value())
		}
		return map[string]any{kindAny: values}
	case kindNot:
		if len(n.Children) == 1 {
			return map[string]any{kindNot: n.Children[0].value()}
		}
	}
	return nil
}
