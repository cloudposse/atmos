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
			cel.Variable("answers", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("os", cel.StringType),
			cel.Variable("arch", cel.StringType),
			cel.Variable("platform", cel.StringType),
			cel.Variable("checksum", cel.MapType(cel.StringType, cel.BoolType)),
			cel.Variable("timestamp", cel.MapType(cel.StringType, cel.BoolType)),
			cel.Variable("preconditions", cel.MapType(cel.StringType, cel.BoolType)),
			cel.Variable("sources", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
			cel.Variable("artifacts", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
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
	answers := ctx.Answers
	if answers == nil {
		answers = map[string]any{}
	}
	return map[string]any{
		"ci":            ctx.CI,
		"status":        ctx.Status,
		"stack":         ctx.Stack,
		"component":     ctx.Component,
		"workflow":      ctx.Workflow,
		"step":          ctx.Step,
		"hook":          ctx.Hook,
		"event":         ctx.Event,
		"env":           env,
		"answers":       answers,
		"os":            ctx.OS,
		"arch":          ctx.Arch,
		"platform":      ctx.Platform,
		"checksum":      map[string]bool{"changed": ctx.ChecksumChanged},
		"timestamp":     map[string]bool{"changed": ctx.TimestampChanged},
		"preconditions": map[string]bool{"success": ctx.PreconditionsSuccess},
		"sources":       fileFactsOrEmpty(ctx.Sources),
		"artifacts":     fileFactsOrEmpty(ctx.Artifacts),
	}
}

// fileFactsOrEmpty converts FileFacts to the []map[string]any shape CEL's list-of-maps adapter
// expects, or an empty (non-nil) slice -- CEL's list type adapter rejects a nil slice, so a step
// with no inputs.sources/artifacts.paths must still activate cleanly.
func fileFactsOrEmpty(facts []FileFact) []map[string]any {
	records := make([]map[string]any, len(facts))
	for i, f := range facts {
		records[i] = map[string]any{"path": f.Path, "mtime": f.Mtime, "checksum": f.Checksum}
	}
	return records
}

func celMentionsIdentifier(expr, ident string) bool {
	// A `.` is kept as part of the token (not a delimiter), so a dotted field-selection chain
	// like "checksum.changed" tokenizes as one token, not two. Match both a bare identifier
	// (e.g. "status") and an identifier used as the root of a field-selection chain (e.g.
	// "checksum" in "checksum.changed"), since freshness facts are exposed as small maps
	// (checksum.changed, timestamp.changed, preconditions.success) rather than flat scalars.
	prefix := ident + "."
	for _, token := range strings.FieldsFunc(expr, func(r rune) bool {
		return r != '_' &&
			r != '.' &&
			(r < '0' || r > '9') &&
			(r < 'A' || r > 'Z') &&
			(r < 'a' || r > 'z')
	}) {
		if token == ident || strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}
