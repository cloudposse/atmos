package function

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HCLFunctions returns a map of cty functions that wrap Atmos functions.
// Functions are registered in the "atmos::" namespace, allowing both syntaxes:
//   - atmos::env("VAR")  - explicit namespace with double colon
//   - atmos_env("VAR")   - underscore syntax (HCL converts _ to :: for functions)
func HCLFunctions(registry *Registry, execCtx *ExecutionContext) map[string]function.Function {
	defer perf.Track(nil, "function.HCLFunctions")()

	if registry == nil {
		registry = DefaultRegistry(nil)
	}

	funcs := make(map[string]function.Function)

	// Register all functions from the registry in the atmos:: namespace.
	// HCL treats foo_bar() as foo::bar(), so registering as "atmos::env"
	// allows both atmos::env("VAR") and atmos_env("VAR") syntax.
	for _, name := range registry.List() {
		namespacedName := "atmos::" + normalizeHCLName(name)
		funcs[namespacedName] = wrapAtmosFunction(registry, name, execCtx)
	}

	return funcs
}

// HCLEvalContextWithFunctions creates an hcl.EvalContext with Atmos functions.
// Functions are available in the atmos:: namespace: atmos::env("VAR"), atmos::exec("cmd"), etc.
// Due to HCL's underscore-to-namespace conversion, atmos_env("VAR") also works.
// Standard library functions (lower, upper, concat, etc.) are available without namespace prefix.
func HCLEvalContextWithFunctions(registry *Registry, execCtx *ExecutionContext) *hcl.EvalContext {
	defer perf.Track(nil, "function.HCLEvalContextWithFunctions")()

	funcs := HCLFunctions(registry, execCtx)

	// Add stdlib functions (no namespace prefix).
	for name, fn := range StdlibFunctions() {
		funcs[name] = fn
	}

	return &hcl.EvalContext{
		Functions: funcs,
	}
}

// wrapAtmosFunction wraps an Atmos function as a cty function.
func wrapAtmosFunction(registry *Registry, name string, execCtx *ExecutionContext) function.Function {
	return function.New(&function.Spec{
		Description: "Atmos " + name + " function",
		Params: []function.Parameter{
			{
				Name: "args",
				Type: cty.String,
			},
		},
		VarParam: &function.Parameter{
			Name: "extra_args",
			Type: cty.String,
		},
		// Use DynamicPseudoType to allow functions to return various types (string, number, bool, list, object).
		Type: func(args []cty.Value) (cty.Type, error) {
			return cty.DynamicPseudoType, nil
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			fn, err := registry.Get(name)
			if err != nil {
				return cty.NilVal, err
			}

			// Build the args string from all parameters.
			var argsStr string
			for i, arg := range args {
				if i > 0 {
					argsStr += " "
				}
				argsStr += arg.AsString()
			}

			// Execute the function.
			result, err := fn.Execute(context.Background(), argsStr, execCtx)
			if err != nil {
				return cty.NilVal, err
			}

			// Convert result to cty.Value.
			return toCtyValue(result), nil
		},
	})
}

// toCtyValue converts a Go value to a cty.Value.
//
//nolint:cyclop,funlen,gocognit,nolintlint,revive
func toCtyValue(v any) cty.Value {
	switch val := v.(type) {
	case string:
		return cty.StringVal(val)
	case int:
		return cty.NumberIntVal(int64(val))
	case int64:
		return cty.NumberIntVal(val)
	case float64:
		return cty.NumberFloatVal(val)
	case bool:
		return cty.BoolVal(val)
	case []any:
		if len(val) == 0 {
			return cty.ListValEmpty(cty.String)
		}
		vals := make([]cty.Value, len(val))
		for i, item := range val {
			vals[i] = toCtyValue(item)
		}
		return cty.TupleVal(vals)
	case map[string]any:
		if len(val) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value, len(val))
		for k, item := range val {
			vals[k] = toCtyValue(item)
		}
		return cty.ObjectVal(vals)
	default:
		return cty.StringVal("")
	}
}

// normalizeHCLName converts an Atmos function name to a valid HCL identifier.
// Replaces hyphens with underscores since HCL doesn't allow hyphens in identifiers.
func normalizeHCLName(name string) string {
	result := ""
	for _, c := range name {
		if c == '-' {
			result += "_"
		} else {
			result += string(c)
		}
	}
	return result
}
