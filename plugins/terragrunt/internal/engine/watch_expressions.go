package engine

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// errWatchUnresolvable reports an expression that needs state, the environment,
// or another evaluation input that inventory deliberately never reads.
var errWatchUnresolvable = errors.New("expression cannot be resolved without evaluating the unit")

// watchEvaluator evaluates the pure subset of Terragrunt expressions that name
// files: path helpers, string functions, and local values. Inventory runs without
// state, secrets, or user commands, so nothing here can read outputs, run
// commands, or consult the environment. Anything outside the subset is an error
// and the caller falls back to a conservative watch.
type watchEvaluator struct {
	fs afero.Fs
	// root is the project root. Searches and watches never leave it.
	root string
	// unit is the directory of the unit configuration (get_terragrunt_dir).
	unit string
	// parent is the directory of the first include; empty when not statically known.
	parent string
}

// context builds an evaluation context whose local values are already resolved.
func (e *watchEvaluator) context(locals cty.Value) *hcl.EvalContext {
	return &hcl.EvalContext{Functions: e.functions(), Variables: map[string]cty.Value{"local": locals}}
}

// locals resolves every local value that only depends on the pure subset. Locals
// that need anything else are left out, so references to them become unresolvable.
func (e *watchEvaluator) locals(body *hclsyntax.Body) cty.Value {
	pending := map[string]hclsyntax.Expression{}
	for _, block := range body.Blocks {
		if block.Type != "locals" {
			continue
		}
		for name, attribute := range block.Body.Attributes {
			pending[name] = attribute.Expr
		}
	}
	values := map[string]cty.Value{}
	ctx := e.context(cty.EmptyObjectVal)
	for len(pending) > 0 {
		progress := false
		for _, name := range slices.Sorted(maps.Keys(pending)) {
			ctx.Variables["local"] = cty.ObjectVal(values)
			value, diagnostics := pending[name].Value(ctx)
			if diagnostics.HasErrors() || !value.IsWhollyKnown() || value.IsNull() {
				continue
			}
			values[name] = value
			delete(pending, name)
			progress = true
		}
		if !progress {
			break
		}
	}
	return cty.ObjectVal(values)
}

// stringOf evaluates an expression that must produce a known, non-null string.
func stringOf(expression hclsyntax.Expression, ctx *hcl.EvalContext) (string, bool) {
	value, diagnostics := expression.Value(ctx)
	if diagnostics.HasErrors() || !value.IsWhollyKnown() || value.IsNull() || value.Type() != cty.String {
		return "", false
	}
	return value.AsString(), true
}

func (e *watchEvaluator) functions() map[string]function.Function {
	functions := map[string]function.Function{
		"format":     stdlib.FormatFunc,
		"join":       stdlib.JoinFunc,
		"replace":    stdlib.ReplaceFunc,
		"lower":      stdlib.LowerFunc,
		"upper":      stdlib.UpperFunc,
		"concat":     stdlib.ConcatFunc,
		"trimprefix": stdlib.TrimPrefixFunc,
		"trimsuffix": stdlib.TrimSuffixFunc,
		"dirname":    pathFunction(filepath.Dir),
		"basename":   pathFunction(filepath.Base),
		"abspath": pathFunction(func(path string) string {
			if filepath.IsAbs(path) {
				return filepath.Clean(path)
			}
			return filepath.Join(e.unit, path)
		}),
		"get_terragrunt_dir":          constantFunction(func() (string, error) { return e.unit, nil }),
		"get_original_terragrunt_dir": constantFunction(func() (string, error) { return e.unit, nil }),
		"get_parent_terragrunt_dir":   constantFunction(e.parentDirectory),
		"path_relative_to_include":    constantFunction(func() (string, error) { return e.relativeToParent(false) }),
		"path_relative_from_include":  constantFunction(func() (string, error) { return e.relativeToParent(true) }),
		"get_repo_root":               constantFunction(e.repositoryRoot),
	}
	functions["find_in_parent_folders"] = function.New(&function.Spec{
		VarParam: &function.Parameter{Name: "args", Type: cty.String},
		Type:     function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			found, err := e.findInParents(args)
			return cty.StringVal(found), err
		},
	})
	return functions
}

func pathFunction(transform func(string) string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.StringVal(transform(args[0].AsString())), nil
		},
	})
}

func constantFunction(compute func() (string, error)) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func([]cty.Value, cty.Type) (cty.Value, error) {
			value, err := compute()
			return cty.StringVal(value), err
		},
	})
}

func (e *watchEvaluator) parentDirectory() (string, error) {
	if e.parent == "" {
		return "", fmt.Errorf("%w: no statically known include", errWatchUnresolvable)
	}
	return e.parent, nil
}

func (e *watchEvaluator) relativeToParent(fromParent bool) (string, error) {
	parent, err := e.parentDirectory()
	if err != nil {
		return "", err
	}
	if fromParent {
		return filepath.Rel(e.unit, parent)
	}
	return filepath.Rel(parent, e.unit)
}

// repositoryRoot finds the nearest ancestor holding a .git entry. Linked
// worktrees and submodules use a .git file, so existence is enough.
func (e *watchEvaluator) repositoryRoot() (string, error) {
	for dir := e.unit; ; dir = filepath.Dir(dir) {
		if _, err := e.fs.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("%w: no Git repository", errWatchUnresolvable)
		}
	}
}

// findInParents mirrors find_in_parent_folders: the search starts above the unit
// directory and stops at the project root, then falls back to the second argument.
func (e *watchEvaluator) findInParents(args []cty.Value) (string, error) {
	if len(args) > 2 {
		return "", fmt.Errorf("%w: too many arguments", errWatchUnresolvable)
	}
	name := "terragrunt.hcl"
	if len(args) > 0 {
		name = args[0].AsString()
	}
	for dir := filepath.Dir(e.unit); within(dir, e.root); dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, name)
		if _, err := e.fs.Stat(candidate); err == nil {
			return candidate, nil
		}
		if dir == e.root || filepath.Dir(dir) == dir {
			break
		}
	}
	if len(args) == 2 {
		return args[1].AsString(), nil
	}
	return "", fmt.Errorf("%w: %s was not found in a parent folder", errWatchUnresolvable, name)
}

// includeParent returns the directory of the first include whose path can be
// resolved without a parent directory.
func (e *watchEvaluator) includeParent(body *hclsyntax.Body) string {
	ctx := e.context(e.locals(body))
	for _, block := range body.Blocks {
		if block.Type != "include" || block.Body.Attributes["path"] == nil {
			continue
		}
		if path, ok := stringOf(block.Body.Attributes["path"].Expr, ctx); ok {
			return filepath.Dir(localPath(e.unit, path))
		}
	}
	return ""
}
