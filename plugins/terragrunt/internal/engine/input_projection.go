package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"
)

const inputsAttribute = "inputs"

var (
	errInputReferenceTarget   = errors.New("backend input projection requires a statically identifiable configuration path")
	errIncludeReferenceTarget = errors.New("backend input projection requires a static exposed include path")
)

// inputReferences tracks input keys used by the probe, including references in
// retained input values. A whole-map or dynamic reference requires all inputs.
// The parser may read included files concurrently.
type inputReferences struct {
	mu       sync.Mutex
	root     string
	files    map[string]*inputSelection
	locals   map[string]map[string]hclsyntax.Expression
	includes map[string]map[string]hclsyntax.Expression
	err      error
	fs       afero.Fs
}

type inputSelection struct {
	keys map[string]bool
	all  bool
}

func newInputReferences(root string, fs afero.Fs) *inputReferences {
	return &inputReferences{fs: fs, root: filepath.Clean(root), files: map[string]*inputSelection{}, locals: map[string]map[string]hclsyntax.Expression{}, includes: map[string]map[string]hclsyntax.Expression{}}
}

// project preserves syntax errors for the upstream parser to diagnose.
func (r *inputReferences) project(name string, data []byte) ([]byte, error) {
	parsed, diags := hclsyntax.ParseConfig(data, name, hcl.InitialPos)
	if diags.HasErrors() {
		return data, nil
	}
	body := parsed.Body.(*hclsyntax.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	name = filepath.Clean(name)
	r.collectBody(name, body)
	selection := r.selection(name)
	inputs := body.Attributes[inputsAttribute]
	if inputs == nil {
		return data, r.err
	}
	file, _ := hclwrite.ParseConfig(data, name, hcl.InitialPos)
	if name == r.root || (!selection.all && len(selection.keys) == 0) {
		file.Body().RemoveAttribute(inputsAttribute)
		return file.Bytes(), r.err
	}
	if selection.all {
		r.collect(name, inputs.Expr)
		return data, r.err
	}
	object, ok := inputs.Expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		// Computed maps cannot be safely pruned without evaluating their expression.
		r.collect(name, inputs.Expr)
		return data, r.err
	}
	r.expand(name, object)
	if selection.all {
		r.collect(name, inputs.Expr)
		return data, r.err
	}
	file.Body().SetAttributeRaw(inputsAttribute, r.objectTokens(object, data, name))
	return file.Bytes(), r.err
}

func (r *inputReferences) collectBody(name string, body *hclsyntax.Body) {
	aliases := map[string]hclsyntax.Expression{}
	includes := map[string]hclsyntax.Expression{}
	for _, block := range body.Blocks {
		if block.Type == "include" && block.Body.Attributes["path"] != nil {
			label := ""
			if len(block.Labels) > 0 {
				label = block.Labels[0]
			}
			includes[label] = block.Body.Attributes["path"].Expr
		}
		if block.Type == "locals" {
			for key, attr := range block.Body.Attributes {
				aliases[key] = attr.Expr
			}
		}
	}
	r.locals[name] = aliases
	r.includes[name] = includes
	for attribute, attr := range body.Attributes {
		if attribute != inputsAttribute {
			r.collect(name, attr.Expr)
		}
	}
	for _, block := range body.Blocks {
		r.collect(name, block.Body)
	}
}

func (r *inputReferences) expand(name string, object *hclsyntax.ObjectConsExpr) {
	selection := r.selection(name)
	// A selected value may reference another input key. Reach a fixed point before
	// pruning so source order does not determine which values survive.
	for {
		count := len(selection.keys)
		for _, item := range object.Items {
			key, diags := item.KeyExpr.Value(nil)
			if diags.HasErrors() || !key.IsKnown() || key.IsNull() || key.Type() != cty.String {
				// Preserve computed keys rather than silently selecting a fallback path.
				selection.all = true
				return
			}
			if selection.keys[key.AsString()] {
				r.collect(name, item.ValueExpr)
			}
		}
		if selection.all {
			return
		}
		if count == len(selection.keys) {
			return
		}
	}
}

func (r *inputReferences) objectTokens(object *hclsyntax.ObjectConsExpr, data []byte, name string) hclwrite.Tokens {
	selection := r.selection(name)
	var attributes []hclwrite.ObjectAttrTokens
	for _, item := range object.Items {
		key, _ := item.KeyExpr.Value(nil)
		if !selection.keys[key.AsString()] {
			continue
		}
		// Reparse the value as an attribute to preserve arbitrary HCL expressions.
		value := item.ValueExpr.Range().SliceBytes(data)
		expression, _ := hclwrite.ParseConfig(append(append([]byte("value = "), value...), '\n'), name, hcl.InitialPos)
		attributes = append(attributes, hclwrite.ObjectAttrTokens{
			Name:  hclwrite.TokensForValue(key),
			Value: expression.Body().GetAttribute("value").Expr().BuildTokens(nil),
		})
	}
	return hclwrite.TokensForObject(attributes)
}

// selection keeps each configuration's requested keys separate.
func (r *inputReferences) selection(name string) *inputSelection {
	if r.files[name] == nil {
		r.files[name] = &inputSelection{keys: map[string]bool{}}
	}
	return r.files[name]
}

func (r *inputReferences) collect(name string, node hclsyntax.Node) {
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		switch expression := node.(type) {
		case *hclsyntax.ScopeTraversalExpr:
			r.traversal(name, nil, expression.Traversal)
		case *hclsyntax.RelativeTraversalExpr:
			r.traversal(name, expression.Source, expression.Traversal)
		}
		return nil
	})
}

func (r *inputReferences) traversal(name string, source hclsyntax.Expression, traversal hcl.Traversal) {
	for i, step := range traversal {
		attribute, _ := traversalName(step)
		if attribute != inputsAttribute {
			continue
		}
		if source == nil && i > 0 {
			source = &hclsyntax.ScopeTraversalExpr{Traversal: traversal[:i]}
		}
		target := r.referenceTarget(name, source, map[string]bool{})
		if target == "" {
			// Ordinary object attributes named inputs are not configuration reads.
			return
		}
		selection := r.selection(target)
		if i+1 == len(traversal) {
			selection.all = true
			return
		}
		key, known := traversalName(traversal[i+1])
		if known {
			selection.keys[key] = true
		} else {
			selection.all = true
		}
		return
	}
}

// referenceTarget follows local aliases without evaluating commands or inputs.
// Literal paths are resolved relative to the configuration containing the call.
func (r *inputReferences) referenceTarget(name string, expression hclsyntax.Expression, seen map[string]bool) string {
	switch expression := expression.(type) {
	case *hclsyntax.ParenthesesExpr:
		return r.referenceTarget(name, expression.Expression, seen)
	case *hclsyntax.ScopeTraversalExpr:
		if len(expression.Traversal) > 0 {
			root, _ := traversalName(expression.Traversal[0])
			if root == "include" {
				return r.includeTarget(name, expression.Traversal)
			}
		}
		if len(expression.Traversal) != 2 {
			return ""
		}
		root, _ := traversalName(expression.Traversal[0])
		key, _ := traversalName(expression.Traversal[1])
		if root != "local" || seen[key] {
			return ""
		}
		seen[key] = true
		return r.referenceTarget(name, r.locals[name][key], seen)
	case *hclsyntax.FunctionCallExpr:
		return r.configCallTarget(name, expression)
	}
	return ""
}

// configCallTarget accepts only a literal path, without executing HCL functions.
func (r *inputReferences) configCallTarget(name string, expression *hclsyntax.FunctionCallExpr) string {
	if expression.Name != "read_terragrunt_config" {
		return ""
	}
	if len(expression.Args) == 0 {
		r.err = fmt.Errorf("%w: %s", errInputReferenceTarget, name)
		return ""
	}
	value, diags := expression.Args[0].Value(nil)
	if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		r.err = fmt.Errorf("%w: %s", errInputReferenceTarget, name)
		return ""
	}
	target := localPath(filepath.Dir(name), value.AsString())
	if info, err := r.fs.Stat(target); err == nil && info.IsDir() {
		target = config.GetDefaultConfigPath(r.fs, target)
	}
	return target
}

func traversalName(step hcl.Traverser) (string, bool) {
	switch step := step.(type) {
	case hcl.TraverseRoot:
		return step.Name, true
	case hcl.TraverseAttr:
		return step.Name, true
	case hcl.TraverseIndex:
		if step.Key.IsKnown() && !step.Key.IsNull() && step.Key.Type() == cty.String {
			return step.Key.AsString(), true
		}
	}
	return "", false
}

// includeTarget resolves only literal paths and the read-only parent lookup;
// unsupported include expressions have a distinct diagnostic from config reads.
func (r *inputReferences) includeTarget(name string, traversal hcl.Traversal) string {
	if len(traversal) > 2 {
		return ""
	}
	label := ""
	if len(traversal) == 2 {
		label, _ = traversalName(traversal[1])
	}
	target := r.includeExpressionTarget(name, r.includes[name][label])
	if target == "" {
		r.err = fmt.Errorf("%w: %s", errIncludeReferenceTarget, name)
	}
	return target
}

func (r *inputReferences) includeExpressionTarget(name string, expression hclsyntax.Expression) string {
	if value, ok := literalString(expression); ok {
		return localPath(filepath.Dir(name), value)
	}
	if call, ok := expression.(*hclsyntax.FunctionCallExpr); ok && call.Name == "find_in_parent_folders" {
		return r.parentInclude(name, call)
	}
	return ""
}

func literalString(expression hclsyntax.Expression) (string, bool) {
	if expression == nil {
		return "", false
	}
	value, diags := expression.Value(nil)
	if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		return "", false
	}
	return value.AsString(), true
}

func (r *inputReferences) parentInclude(name string, call *hclsyntax.FunctionCallExpr) string {
	filename := unitFile
	if len(call.Args) > 0 {
		var ok bool
		filename, ok = literalString(call.Args[0])
		if !ok {
			return ""
		}
	}
	for directory := filepath.Dir(filepath.Dir(name)); ; directory = filepath.Dir(directory) {
		candidate := filepath.Join(directory, filename)
		if info, err := r.fs.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		if parent := filepath.Dir(directory); parent == directory {
			break
		}
	}
	if len(call.Args) == 2 {
		if value, ok := literalString(call.Args[1]); ok {
			return localPath(filepath.Dir(name), value)
		}
	}
	return ""
}
