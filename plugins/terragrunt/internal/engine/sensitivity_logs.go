package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"

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
func redactUnitInputs(sink *LogSink, fsys afero.Fs, files []string, unit string, cfg *config.TerragruntConfig) [][]string {
	paths := sensitiveInputs(fsys, files, cfg.Inputs, unit, cfg.ProcessedIncludes)
	redactInputs(sink, cfg.Inputs, paths)
	if sink.collecting() && (unverifiable(fsys, unit) || transformsSensitiveInputs(fsys, unit)) {
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
		for i, item := range object.Items {
			if s.transformsSensitiveInput(item.KeyExpr) || s.sensitiveKeyMayLog(item.KeyExpr) || s.transformsSensitiveInput(item.ValueExpr) || s.replacedSensitiveMember(object.Items, i) {
				return true
			}
		}
		return false
	}
	before := len(s.paths)
	s.walk(expression, nil, 0)
	if len(s.paths) > before && s.inputTransformsRaw(expression) {
		return true
	}
	// A sensitive predicate is unknown to the sink whether or not either branch is sensitive.
	return s.sensitivePredicateMayLog(expression)
}

// sensitiveKeyMayLog reports whether an object key is a sensitive value that may have been logged. The sink
// registers the values of the inputs, never their keys, so such a key can never be masked afterwards.
func (s *sensitivityScanner) sensitiveKeyMayLog(key hclsyntax.Expression) bool {
	return s.containsSensitive(key, 0) && s.mayLogValue(key, 0)
}

// sensitivePredicateMayLog reports whether any conditional below expression tests a sensitive value that
// may have been logged. Only the selected branch is registered, so the tested value never is. A literal
// predicate cannot have logged anything.
func (s *sensitivityScanner) sensitivePredicateMayLog(expression hclsyntax.Expression) bool {
	found := false
	_ = hclsyntax.VisitAll(expression, func(node hclsyntax.Node) hcl.Diagnostics {
		if conditional, ok := node.(*hclsyntax.ConditionalExpr); ok && !found {
			found = s.containsSensitive(conditional.Condition, 0) && s.mayLogValue(conditional.Condition, 0)
		}
		return nil
	})
	return found
}

// replacedSensitiveMember reports whether a later member replaces a sensitive member that may have logged
// its value. HCL keeps the last member of a repeated key, so only the replacement reaches the sink and the
// discarded sensitive result, which Terragrunt may already have logged, cannot be registered. A key that is
// not static cannot be compared and is treated as a possible replacement.
func (s *sensitivityScanner) replacedSensitiveMember(items []hclsyntax.ObjectConsItem, index int) bool {
	before := len(s.paths)
	s.walk(items[index].ValueExpr, nil, 0)
	sensitive := len(s.paths) > before
	if !sensitive || !s.mayLogValue(items[index].ValueExpr, 0) {
		return false
	}
	key, known := staticKey(items[index].KeyExpr)
	for _, later := range items[index+1:] {
		laterKey, laterKnown := staticKey(later.KeyExpr)
		if !known || !laterKnown || laterKey == key {
			return true
		}
	}
	return false
}

// inputTransformsRaw treats selecting a branch as preserving its raw value. A sensitive
// condition itself is unknown to the sink because only the selected result is registered.
func (s *sensitivityScanner) inputTransformsRaw(expression hclsyntax.Expression) bool {
	expression = unwrapParentheses(expression)
	if conditional, ok := expression.(*hclsyntax.ConditionalExpr); ok {
		return s.conditionalDiscardsSensitive(conditional, false, 0) || s.containsSensitive(conditional.Condition, 0) ||
			s.inputTransformsRaw(conditional.TrueResult) || s.inputTransformsRaw(conditional.FalseResult)
	}
	if call, ok := expression.(*hclsyntax.FunctionCallExpr); ok && call.Name == "sensitive" && len(call.Args) == 1 {
		if conditional, ok := unwrapParentheses(call.Args[0]).(*hclsyntax.ConditionalExpr); ok {
			return s.wrappedConditionalTransforms(call, conditional)
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
	if sink.collecting() && (unverifiable(fsys, unit) || transformsSensitiveLocal(fsys, unit)) {
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
		if scanner.sensitivePredicateMayLog(expression) {
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
			transformed = callTransformsRaw(typed, wrapped) || (typed.Name == "merge" && s.mergeOverwritesSensitive(typed, depth))
		case *hclsyntax.ConditionalExpr:
			transformed = s.conditionalDiscardsSensitive(typed, wrapped[typed], depth)
		case *hclsyntax.TemplateExpr:
			transformed = !isLiteralTemplate(typed)
		case *hclsyntax.ForExpr, *hclsyntax.BinaryOpExpr, *hclsyntax.UnaryOpExpr:
			transformed = true
		case *hclsyntax.ScopeTraversalExpr:
			transformed = s.referenceTransformsRaw(typed, depth)
		case *hclsyntax.IndexExpr:
			transformed = s.indexDiscardsSensitive(typed, depth)
		case *hclsyntax.RelativeTraversalExpr:
			transformed = s.relativeDiscardsSensitive(typed, depth)
		case *hclsyntax.SplatExpr:
			transformed = s.splatDiscardsSensitive(typed, depth)
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
		if _, err := fsys.Stat(file); err != nil {
			if errors.Is(err, errNotEvaluated) {
				// The evaluated contents are unknown, so the file may call `sensitive`.
				return true
			}
			continue
		}
		// JSON sensitivity is not representable by the native HCL AST scanner.
		if isJSONConfiguration(file) {
			return true
		}
		body, ok := parseBody(fsys, file)
		if !ok {
			// A file Terragrunt evaluated as HCL parses. One that does not, with a suffix that is not
			// `.hcl`, is data the unit read (a YAML file, for example), not configuration.
			if filepath.Ext(file) == ".hcl" {
				return true
			}
			continue
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

// resolveUnit evaluates one unit: it guards destroy, parses the unit, applies
// policies, and assembles the resolution with its execution data, backend, overlay,
// and generated files.
func (u *unitEvaluator) resolveUnit(ctx context.Context, scan *sensitivityScan) (*model.Resolution, error) {
	pctx, request, path := u.pctx, u.request, u.path
	if err := u.checkDestroyGuard(ctx); err != nil {
		return nil, err
	}
	cfg, err := u.parseUnit(ctx)
	if err != nil {
		return nil, err
	}
	var sensitiveInputs [][]string
	if request.Phase != phaseBackend {
		sensitiveInputs = redactUnitInputs(logSinkFrom(ctx), u.checkedFS(), append(pctx.FilesRead.Paths(), path), path, cfg)
		redactLocals(logSinkFrom(ctx), u.checkedFS(), path, cfg.Locals)
		scan.registered = true
	}
	if err := checkUnitPolicies(ctx, request, cfg, path); err != nil {
		return nil, err
	}
	return u.buildResolution(ctx, cfg, &unitSensitivity{inputs: sensitiveInputs, scan: scan})
}

// buildResolution assembles the resolution of a parsed unit. The execution is
// set before the backend resolves so the backend step sees the final result.
func (u *unitEvaluator) buildResolution(ctx context.Context, cfg *config.TerragruntConfig, sensitivity *unitSensitivity) (*model.Resolution, error) {
	logger, environment, request, path := u.logger, u.environment, u.request, u.path
	execution, applyFiles, err := u.unitExecution(ctx, cfg, sensitivity.scan)
	if err != nil {
		return nil, err
	}
	result, err := u.unitResolution(ctx, cfg, execution)
	if err != nil {
		return nil, err
	}
	result.SensitiveInputs = sensitivity.inputs
	result.Files = append(result.Files, applyFiles...)
	if err := resolveBackend(cfg, result, request); err != nil {
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

// mergeOverwritesSensitive detects values evaluated before a later merge argument replaces them.
func (s *sensitivityScanner) mergeOverwritesSensitive(call *hclsyntax.FunctionCallExpr, depth int) bool {
	operands, known := s.mergeOperands(call, depth)
	if !known {
		return true
	}
	for i, argument := range operands {
		scan := *s
		scan.paths = nil
		scan.walk(argument, nil, depth)
		for _, later := range operands[i+1:] {
			if s.overwritesSensitivePaths(later, scan.paths, depth) {
				return true
			}
		}
	}
	return false
}

// mergeOperands lists the objects a merge call combines. An expanded final argument supplies several
// operands: the members of a literal tuple are separate operands, while any other expression has members the
// scan cannot enumerate, which is reported as unknown when the call may hold a sensitive value.
func (s *sensitivityScanner) mergeOperands(call *hclsyntax.FunctionCallExpr, depth int) ([]hclsyntax.Expression, bool) {
	if !call.ExpandFinal || len(call.Args) == 0 {
		return call.Args, true
	}
	last := len(call.Args) - 1
	if tuple, ok := unwrapParentheses(call.Args[last]).(*hclsyntax.TupleConsExpr); ok {
		return append(slices.Clone(call.Args[:last]), tuple.Exprs...), true
	}
	if slices.ContainsFunc(call.Args, func(argument hclsyntax.Expression) bool { return s.containsSensitive(argument, depth) }) {
		return nil, false
	}
	return call.Args[:last], true
}

func (s *sensitivityScanner) overwritesSensitivePaths(expression hclsyntax.Expression, paths [][]string, depth int) bool {
	if len(paths) == 0 {
		return false
	}
	keys, known := s.objectKeys(expression, depth)
	if !known {
		return true
	}
	for _, path := range paths {
		if len(path) == 0 {
			return len(keys) != 0
		}
		if keys[path[0]] {
			return true
		}
	}
	return false
}

// objectKeys proves shallow merge keys without evaluating functions or invoking effects.
func (s *sensitivityScanner) objectKeys(expression hclsyntax.Expression, depth int) (map[string]bool, bool) {
	if depth >= maxLocalDepth {
		return nil, false
	}
	switch node := unwrapParentheses(expression).(type) {
	case *hclsyntax.ObjectConsExpr:
		keys := make(map[string]bool)
		for _, item := range node.Items {
			key, ok := staticKey(item.KeyExpr)
			if !ok {
				return nil, false
			}
			keys[key] = true
		}
		return keys, true
	case *hclsyntax.ScopeTraversalExpr:
		reference, ok := s.reference(node.Traversal)
		if !ok || len(reference.rest) != 0 {
			return nil, false
		}
		next := *s
		next.locals = reference.locals
		return next.objectKeys(reference.expression, depth+1)
	}
	return nil, false
}

// conditionalDiscardsSensitive accounts for HCL evaluating both branches before selection.
func (s *sensitivityScanner) conditionalDiscardsSensitive(expression *hclsyntax.ConditionalExpr, wrapped bool, depth int) bool {
	for _, branch := range []hclsyntax.Expression{expression.TrueResult, expression.FalseResult} {
		if (wrapped || s.containsSensitive(branch, depth)) && s.mayLogValue(branch, depth) {
			return true
		}
	}
	return false
}

// mayLogValue conservatively follows calls and references; literal branches cannot emit logs.
func (s *sensitivityScanner) mayLogValue(expression hclsyntax.Expression, depth int) bool {
	if depth >= maxLocalDepth {
		return true
	}
	possible := false
	_ = hclsyntax.VisitAll(expression, func(node hclsyntax.Node) hcl.Diagnostics {
		if possible {
			return nil
		}
		switch typed := node.(type) {
		case *hclsyntax.FunctionCallExpr:
			possible = typed.Name != "sensitive"
		case *hclsyntax.ScopeTraversalExpr:
			reference, found := s.reference(typed.Traversal)
			if !found {
				possible = true
				break
			}
			nested := *s
			nested.locals = reference.locals
			possible = nested.mayLogValue(reference.expression, depth+1)
		}
		return nil
	})
	return possible
}

func (s *sensitivityScanner) wrappedConditionalTransforms(call *hclsyntax.FunctionCallExpr, conditional *hclsyntax.ConditionalExpr) bool {
	selected := *call
	selected.Args = []hclsyntax.Expression{conditional.TrueResult}
	other := *call
	other.Args = []hclsyntax.Expression{conditional.FalseResult}
	return s.conditionalDiscardsSensitive(conditional, true, 0) || s.containsSensitive(conditional.Condition, 0) || s.inputTransformsRaw(&selected) || s.inputTransformsRaw(&other)
}

// errNotEvaluated marks a file whose evaluated contents were not captured, or changed while the request
// was evaluated. A sensitivity check cannot vouch for it, so the values it may declare are protected.
var errNotEvaluated = errors.New("the file contents were not captured while the unit was evaluated")

// evaluatedFS records the bytes Terragrunt reads while it evaluates a request. The sensitivity checks
// run after the evaluation, and the files may have changed on disk since: they scan what was evaluated.
type evaluatedFS struct {
	afero.Fs
	mu    sync.Mutex
	files map[string]*evaluatedFile
}

type evaluatedFile struct {
	content []byte
	// changed is set when a later read of the same file returned different bytes.
	changed bool
}

func newEvaluatedFS(fsys afero.Fs) *evaluatedFS {
	return &evaluatedFS{Fs: fsys, files: map[string]*evaluatedFile{}}
}

func (f *evaluatedFS) Open(name string) (afero.File, error) {
	file, err := f.Fs.Open(name)
	return f.record(file, err, name)
}

func (f *evaluatedFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := f.Fs.OpenFile(name, flag, perm)
	if err != nil || flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return file, err
	}
	return f.record(file, err, name)
}

// record wraps a regular file so its sequentially read bytes are captured.
func (f *evaluatedFS) record(file afero.File, err error, name string) (afero.File, error) {
	if err != nil {
		return file, err
	}
	if info, statErr := file.Stat(); statErr != nil || info.IsDir() {
		return file, nil
	}
	return &recordingFile{File: file, owner: f, path: filepath.Clean(name), sequential: true}, nil
}

func (f *evaluatedFS) capture(path string, content []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, found := f.files[path]
	switch {
	case !found:
		f.files[path] = &evaluatedFile{content: bytes.Clone(content)}
	case !bytes.Equal(existing.content, content):
		existing.changed = true
	}
}

// Evaluated returns a read-only view of the captured files. Any other file reads as errNotEvaluated.
func (f *evaluatedFS) Evaluated() afero.Fs {
	f.mu.Lock()
	defer f.mu.Unlock()
	memory := afero.NewMemMapFs()
	view := &evaluatedView{Fs: afero.NewReadOnlyFs(memory), disk: f.Fs, captured: map[string]bool{}, changed: map[string]bool{}}
	for path, file := range f.files {
		if file.changed {
			// Remember it: a file that changed while it was evaluated stays unverifiable even after it is deleted.
			view.changed[path] = true
			continue
		}
		if err := afero.WriteFile(memory, path, file.content, 0o600); err == nil {
			view.captured[path] = true
		}
	}
	return view
}

// recordingFile captures the bytes read from the start of a file up to its end.
type recordingFile struct {
	afero.File
	owner      *evaluatedFS
	path       string
	buffer     bytes.Buffer
	sequential bool
}

func (r *recordingFile) Read(p []byte) (int, error) {
	n, err := r.File.Read(p)
	if r.sequential {
		r.buffer.Write(p[:n])
		if errors.Is(err, io.EOF) {
			r.owner.capture(r.path, r.buffer.Bytes())
		}
	}
	return n, err
}

// Seek keeps the capture only for a rewind to the start; any other position makes the bytes incomplete.
func (r *recordingFile) Seek(offset int64, whence int) (int64, error) {
	position, err := r.File.Seek(offset, whence)
	if err == nil {
		r.sequential = position == 0
		r.buffer.Reset()
	}
	return position, err
}

// evaluatedView serves only the files that were captured. A file that was not captured and does not
// exist reads as missing, as it did for Terragrunt; one that exists, or that changed while it was
// evaluated, reads as errNotEvaluated.
type evaluatedView struct {
	afero.Fs
	disk     afero.Fs
	captured map[string]bool
	changed  map[string]bool
}

func (v *evaluatedView) Open(name string) (afero.File, error) {
	if !v.captured[filepath.Clean(name)] {
		return nil, v.missing("open", name)
	}
	return v.Fs.Open(name)
}

func (v *evaluatedView) Stat(name string) (os.FileInfo, error) {
	if !v.captured[filepath.Clean(name)] {
		return nil, v.missing("stat", name)
	}
	return v.Fs.Stat(name)
}

func (v *evaluatedView) missing(op, name string) error {
	if v.changed[filepath.Clean(name)] {
		return &os.PathError{Op: op, Path: name, Err: errNotEvaluated}
	}
	if _, err := v.disk.Stat(name); err != nil {
		return &os.PathError{Op: op, Path: name, Err: err}
	}
	return &os.PathError{Op: op, Path: name, Err: errNotEvaluated}
}

// checkedFS returns the filesystem the sensitivity checks read: the evaluated contents when the
// evaluation captured them, otherwise the evaluation's own filesystem.
func (e *unitEvaluator) checkedFS() afero.Fs {
	if captured, ok := e.environment.FS.(*evaluatedFS); ok {
		return captured.Evaluated()
	}
	return e.environment.FS
}

// unverifiable reports whether the file's evaluated contents are unavailable.
func unverifiable(fsys afero.Fs, file string) bool {
	_, err := fsys.Stat(file)
	return errors.Is(err, errNotEvaluated)
}
