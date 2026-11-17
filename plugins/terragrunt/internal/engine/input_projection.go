package engine

import (
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

const inputsAttribute = "inputs"

// inputReferences tracks input keys used by the probe, including references in
// retained input values. A whole-map or dynamic reference requires all inputs.
// The parser may read included files concurrently.
type inputReferences struct {
	mu   sync.Mutex
	root string
	keys map[string]bool
	all  bool
}

func newInputReferences(root string) *inputReferences {
	return &inputReferences{root: root, keys: map[string]bool{}}
}

// project preserves syntax errors for the upstream parser to diagnose.
func (r *inputReferences) project(name string, data []byte) []byte {
	parsed, diags := hclsyntax.ParseConfig(data, name, hcl.InitialPos)
	if diags.HasErrors() {
		return data
	}
	body := parsed.Body.(*hclsyntax.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectBody(body)
	inputs := body.Attributes[inputsAttribute]
	if inputs == nil {
		return data
	}
	file, _ := hclwrite.ParseConfig(data, name, hcl.InitialPos)
	if name == r.root || (!r.all && len(r.keys) == 0) {
		file.Body().RemoveAttribute(inputsAttribute)
		return file.Bytes()
	}
	if r.all {
		return data
	}
	object, ok := inputs.Expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		// Computed maps cannot be safely pruned without evaluating their expression.
		r.collect(inputs.Expr)
		return data
	}
	r.expand(object)
	if r.all {
		return data
	}
	file.Body().SetAttributeRaw(inputsAttribute, r.objectTokens(object, data, name))
	return file.Bytes()
}

func (r *inputReferences) collectBody(body *hclsyntax.Body) {
	for name, attr := range body.Attributes {
		if name != inputsAttribute {
			r.collect(attr.Expr)
		}
	}
	for _, block := range body.Blocks {
		r.collect(block.Body)
	}
}

func (r *inputReferences) expand(object *hclsyntax.ObjectConsExpr) {
	// A selected value may reference another input key. Reach a fixed point before
	// pruning so source order does not determine which values survive.
	for {
		count := len(r.keys)
		for _, item := range object.Items {
			key, diags := item.KeyExpr.Value(nil)
			if diags.HasErrors() || !key.IsKnown() || key.IsNull() || key.Type() != cty.String {
				// Preserve computed keys rather than silently selecting a fallback path.
				r.all = true
				return
			}
			if r.keys[key.AsString()] {
				r.collect(item.ValueExpr)
			}
		}
		if r.all {
			return
		}
		if count == len(r.keys) {
			return
		}
	}
}

func (r *inputReferences) objectTokens(object *hclsyntax.ObjectConsExpr, data []byte, name string) hclwrite.Tokens {
	var attributes []hclwrite.ObjectAttrTokens
	for _, item := range object.Items {
		key, _ := item.KeyExpr.Value(nil)
		if !r.keys[key.AsString()] {
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

func (r *inputReferences) collect(node hclsyntax.Node) {
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		switch expression := node.(type) {
		case *hclsyntax.ScopeTraversalExpr:
			r.traversal(expression.Traversal)
		case *hclsyntax.RelativeTraversalExpr:
			r.traversal(expression.Traversal)
		}
		return nil
	})
}

func (r *inputReferences) traversal(traversal hcl.Traversal) {
	for i, step := range traversal {
		name, _ := traversalName(step)
		if name != inputsAttribute {
			continue
		}
		if i+1 == len(traversal) {
			r.all = true
			return
		}
		key, known := traversalName(traversal[i+1])
		if known {
			r.keys[key] = true
		} else {
			r.all = true
		}
	}
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
