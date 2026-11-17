package engine

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// sensitivityScan records what the evaluation of a request told the log sink, so the files it read can be
// scanned for sensitive values the sink never learned once the evaluation ends.
type sensitivityScan struct {
	// registered is set once the unit's own inputs and locals are registered with the sink.
	registered bool
	// applyFiles are the files read only when the unit is evaluated for apply, which can declare
	// sensitive values too.
	applyFiles []string
}

// unitSensitivity carries what a unit's resolution reports about its sensitive values: the input
// paths the host masks, and the scan the deferred log check reads.
type unitSensitivity struct {
	inputs [][]string
	scan   *sensitivityScan
}

// redactUnitInputs registers final values and withholds logs when a sensitive input transforms
// a raw value that Terragrunt may already have logged while evaluating the unit.
func redactUnitInputs(sink *LogSink, fsys afero.Fs, files []string, unit string, inputs map[string]any) [][]string {
	paths := SensitiveInputs(fsys, files, inputs)
	redactInputs(sink, inputs, paths)
	if sink.collecting() && transformsSensitiveInputs(fsys, unit) {
		sink.Withhold()
	}
	return paths
}

func transformsSensitiveInputs(fsys afero.Fs, unit string) bool {
	body, ok := parseBody(fsys, unit)
	if !ok || body.Attributes[inputsName] == nil {
		return false
	}
	scanner := &sensitivityScanner{locals: localExpressions(body)}
	return scanner.transformsSensitiveInput(body.Attributes[inputsName].Expr)
}

// transformsSensitiveInput checks object members separately so a non-sensitive sibling's
// transformation does not cause an otherwise maskable sensitive input to withhold logs.
func (s *sensitivityScanner) transformsSensitiveInput(expression hclsyntax.Expression) bool {
	if object, ok := unwrapParentheses(expression).(*hclsyntax.ObjectConsExpr); ok {
		for _, item := range object.Items {
			if s.transformsSensitiveInput(item.KeyExpr) || s.transformsSensitiveInput(item.ValueExpr) {
				return true
			}
		}
		return false
	}
	before := len(s.paths)
	s.walk(expression, nil, 0)
	return len(s.paths) > before && s.inputTransformsRaw(expression)
}

// inputTransformsRaw treats selecting a branch as preserving its raw value. A sensitive
// condition itself is unknown to the sink because only the selected result is registered.
func (s *sensitivityScanner) inputTransformsRaw(expression hclsyntax.Expression) bool {
	expression = unwrapParentheses(expression)
	if conditional, ok := expression.(*hclsyntax.ConditionalExpr); ok {
		return s.containsSensitive(conditional.Condition, 0) ||
			s.inputTransformsRaw(conditional.TrueResult) || s.inputTransformsRaw(conditional.FalseResult)
	}
	if call, ok := expression.(*hclsyntax.FunctionCallExpr); ok && call.Name == "sensitive" && len(call.Args) == 1 {
		if conditional, ok := unwrapParentheses(call.Args[0]).(*hclsyntax.ConditionalExpr); ok {
			selected := *call
			selected.Args = []hclsyntax.Expression{conditional.TrueResult}
			other := *call
			other.Args = []hclsyntax.Expression{conditional.FalseResult}
			return s.containsSensitive(conditional.Condition, 0) || s.inputTransformsRaw(&selected) || s.inputTransformsRaw(&other)
		}
	}
	return s.transformsRaw(expression, 0)
}

// sensitiveLocals returns the paths, from the top of the unit's own locals, of the values its locals block
// declares sensitive. Terragrunt logs a value as it evaluates it, for example the output of
// sensitive(run_cmd(...)), even when the value never reaches the inputs, so these values are redacted too.
func sensitiveLocals(fsys afero.Fs, unit string, locals map[string]any) [][]string {
	body, ok := parseBody(fsys, unit)
	if !ok {
		return nil
	}
	scanner := &sensitivityScanner{locals: localExpressions(body)}
	for name, expression := range scanner.locals {
		scanner.walk(expression, []string{name}, 0)
	}
	return existingPaths(uniquePaths(scanner.paths), locals)
}

// redactLocals registers the values of the unit's sensitive locals with the sink. The sink learns only the
// final value of a local, so a sensitive local built by transforming the output of a call, such as
// sensitive(upper(run_cmd(...))), leaves the raw output, which Terragrunt may already have logged,
// unredacted. The raw value cannot be recovered from the final one, so the events are withheld instead.
func redactLocals(sink *LogSink, fsys afero.Fs, unit string, locals map[string]any) {
	redactInputs(sink, locals, sensitiveLocals(fsys, unit, locals))
	if sink.collecting() && transformsSensitiveLocal(fsys, unit) {
		sink.Withhold()
	}
}

// transformsSensitiveLocal reports whether any sensitive local of the unit derives its value from another
// value, so that the registered final value differs from the raw value. A local that is only
// sensitive(<call>), an object, or a reference to such a local keeps its raw value, and the sink masks it.
func transformsSensitiveLocal(fsys afero.Fs, unit string) bool {
	body, ok := parseBody(fsys, unit)
	if !ok {
		return false
	}
	scanner := &sensitivityScanner{locals: localExpressions(body)}
	for name, expression := range scanner.locals {
		before := len(scanner.paths)
		scanner.walk(expression, []string{name}, 0)
		if len(scanner.paths) > before && scanner.transformsRaw(expression, 0) {
			return true
		}
	}
	return false
}

// TransformsRaw reports whether the value of expression is derived from other values by anything other than
// selecting them: a function that is not `sensitive` or `merge` and does not directly wrap the value, a
// template, a for expression, or an operator. It follows local references, and fails closed on a reference
// it cannot follow and at the depth bound.
func (s *sensitivityScanner) transformsRaw(expression hclsyntax.Expression, depth int) bool {
	if depth >= maxLocalDepth {
		return true
	}
	wrapped := map[hclsyntax.Node]bool{}
	transformed := false
	_ = hclsyntax.VisitAll(expression, func(node hclsyntax.Node) hcl.Diagnostics {
		if transformed {
			return nil
		}
		switch typed := node.(type) {
		case *hclsyntax.FunctionCallExpr:
			transformed = callTransformsRaw(typed, wrapped)
		case *hclsyntax.TemplateExpr:
			transformed = !isLiteralTemplate(typed)
		case *hclsyntax.ForExpr, *hclsyntax.BinaryOpExpr, *hclsyntax.UnaryOpExpr:
			transformed = true
		case *hclsyntax.ScopeTraversalExpr:
			transformed = s.referenceTransformsRaw(typed, depth)
		}
		return nil
	})
	return transformed
}

// callTransformsRaw records sensitive wrappers and identifies transforming calls.
func callTransformsRaw(call *hclsyntax.FunctionCallExpr, wrapped map[hclsyntax.Node]bool) bool {
	switch call.Name {
	case "sensitive":
		for _, argument := range call.Args {
			wrapped[unwrapParentheses(argument)] = true
		}
	case "merge":
	default:
		return !wrapped[call]
	}
	return false
}

// referenceTransformsRaw follows a local reference to the expression that defines it.
func (s *sensitivityScanner) referenceTransformsRaw(traversal *hclsyntax.ScopeTraversalExpr, depth int) bool {
	switch traversal.Traversal.RootName() {
	case localName, "include":
	default:
		return false
	}
	reference, found := s.reference(traversal.Traversal)
	if !found {
		// A local of an included file, or an unknown one, cannot be inspected.
		return true
	}
	previous := s.locals
	s.locals = reference.locals
	defer func() { s.locals = previous }()
	return s.transformsRaw(reference.expression, depth+1)
}

// unwrapParentheses returns the expression inside any number of parentheses.
func unwrapParentheses(expression hclsyntax.Expression) hclsyntax.Expression {
	for {
		parenthesized, ok := expression.(*hclsyntax.ParenthesesExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.Expression
	}
}

// isLiteralTemplate reports whether a template only spells out literal text.
func isLiteralTemplate(template *hclsyntax.TemplateExpr) bool {
	for _, part := range template.Parts {
		if _, ok := part.(*hclsyntax.LiteralValueExpr); !ok {
			return false
		}
	}
	return true
}

// UnredactedSensitivity reports whether any of the files calls `sensitive` where the sink cannot learn
// the value. When the unit was resolved, the values of its inputs and of its own locals are known and
// registered, so only calls elsewhere count: the locals or inputs of an included file, or another attribute
// or block. An included file's inputs count because a child can override them, so only the merged value is
// registered while the included value may already have been logged when Terragrunt parsed that file. When it was not resolved, every call counts. A file that cannot be read is skipped, since
// Terragrunt could not evaluate it either, and one that cannot be parsed counts, since nothing is known
// about it.
func unredactedSensitivity(fsys afero.Fs, files []string, unit string, resolved bool) bool {
	for _, file := range files {
		if filepath.Ext(file) != ".hcl" {
			continue
		}
		if _, err := fsys.Stat(file); err != nil {
			continue
		}
		body, ok := parseBody(fsys, file)
		if !ok {
			return true
		}
		if bodyHasUnredactedSensitivity(body, resolved && file == unit) {
			return true
		}
	}
	return false
}

// bodyHasUnredactedSensitivity skips only the resolved unit's registered values.
func bodyHasUnredactedSensitivity(body *hclsyntax.Body, registered bool) bool {
	for name, attribute := range body.Attributes {
		if registered && name == inputsName {
			continue
		}
		if callsSensitive(attribute.Expr) {
			return true
		}
	}
	for _, block := range body.Blocks {
		if registered && block.Type == "locals" {
			continue
		}
		if callsSensitive(block.Body) {
			return true
		}
	}
	return false
}

// parseBody reads and parses one HCL file.
func parseBody(fsys afero.Fs, file string) (*hclsyntax.Body, bool) {
	content, err := afero.ReadFile(fsys, file)
	if err != nil {
		return nil, false
	}
	parsed, diags := hclsyntax.ParseConfig(content, file, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, false
	}
	body, ok := parsed.Body.(*hclsyntax.Body)
	return body, ok
}

// CallsSensitive reports whether node calls `sensitive` anywhere below it.
func callsSensitive(node hclsyntax.Node) bool {
	found := false
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok && call.Name == "sensitive" {
			found = true
		}
		return nil
	})
	return found
}

// resolve evaluates one unit: it guards destroy, parses the unit, applies
// policies, and assembles the resolution with its execution data, backend, overlay,
// and generated files.
func (u *unitEvaluator) resolve(ctx context.Context, scan *sensitivityScan) (*model.Resolution, error) {
	logger, environment, pctx, request, path := u.logger, u.environment, u.pctx, u.request, u.path
	if err := checkDestroyGuard(ctx, logger, environment, pctx, request, path); err != nil {
		return nil, err
	}
	cfg, err := parseUnit(ctx, logger, environment, pctx, request, path)
	if err != nil {
		return nil, err
	}
	var sensitiveInputs [][]string
	if request.Phase != phaseBackend {
		sensitiveInputs = redactUnitInputs(logSinkFrom(ctx), environment.FS, append(pctx.FilesRead.Paths(), path), path, cfg.Inputs)
		redactLocals(logSinkFrom(ctx), environment.FS, path, cfg.Locals)
		scan.registered = true
	}
	if err := checkUnitPolicies(ctx, &request, cfg, path); err != nil {
		return nil, err
	}
	return u.buildResolution(ctx, cfg, &unitSensitivity{inputs: sensitiveInputs, scan: scan})
}

// buildResolution assembles the resolution of a parsed unit. The execution is
// set before the backend resolves so the backend step sees the final result.
func (u *unitEvaluator) buildResolution(ctx context.Context, cfg *config.TerragruntConfig, sensitivity *unitSensitivity) (*model.Resolution, error) {
	logger, environment, pctx, request, path := u.logger, u.environment, u.pctx, u.request, u.path
	execution, applyFiles, err := unitExecution(ctx, logger, environment, pctx, request, cfg, path)
	sensitivity.scan.applyFiles = applyFiles
	if err != nil {
		return nil, err
	}
	result, err := (&inventoryReader{ctx: ctx, logger: logger, environment: environment}).unitResolution(pctx, &request, cfg, path)
	if err != nil {
		return nil, err
	}
	result.Execution = execution
	result.SensitiveInputs = sensitivity.inputs
	result.Files = append(result.Files, applyFiles...)
	if err := resolveBackend(cfg, result, &request); err != nil {
		return nil, err
	}
	if request.Phase != phaseBackend {
		result.Overlay, result.OverlayModes, err = atmosbridge.UnitOverlay(logger, environment, filepath.Dir(path), cfg.Terraform)
		if err != nil {
			return nil, err
		}
	}
	if err := generatedFiles(ctx, logger, cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}
