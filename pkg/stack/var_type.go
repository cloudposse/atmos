package stack

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// varsPrefix is the component-relative path prefix a Terraform-declared
// variable type can answer for -- variable declarations only ever describe
// vars.<name>, never a nested attribute, list element, or non-vars section.
const varsPrefix = "vars."

// VarNameFromRelPath reports whether relPath addresses a top-level
// vars.<name> value (e.g. "vars.replicas" -> ("replicas", true)) and, if so,
// returns the variable name. Anything else -- non-vars paths, nested
// attribute paths (vars.foo.bar), indexed paths (vars.foo[0]) -- returns
// ("", false): a raw Terraform-declared type string can't tell us the shape
// of a nested attribute or list element without full type-expression
// parsing, which is out of scope for this lookup.
func VarNameFromRelPath(relPath string) (string, bool) {
	defer perf.Track(nil, "stack.VarNameFromRelPath")()

	if !strings.HasPrefix(relPath, varsPrefix) {
		return "", false
	}
	name := relPath[len(varsPrefix):]
	if name == "" || strings.ContainsAny(name, ".[") {
		return "", false
	}
	return name, true
}

// InferVarType maps a Terraform-declared HCL variable type (the raw source
// text captured by terraform-config-inspect, e.g. "string", "number",
// "list(string)") plus the raw CLI value being set to the atmosyaml.TypeXXX
// `stack set --type=auto` should use. Returns ("", false) when hclType is
// empty/"any" (implicit any -- no signal), unrecognized, or -- for "number"
// -- when rawValue doesn't parse as either an int or a float.
func InferVarType(hclType, rawValue string) (string, bool) {
	defer perf.Track(nil, "stack.InferVarType")()

	trimmed := strings.TrimSpace(hclType)
	switch {
	case trimmed == "" || trimmed == "any":
		return "", false
	case trimmed == "string":
		return atmosyaml.TypeString, true
	case trimmed == "bool":
		return atmosyaml.TypeBool, true
	case trimmed == "number":
		return atmosyaml.GuessNumericType(rawValue)
	case isNonScalarHCLType(trimmed):
		return atmosyaml.TypeYAML, true
	default:
		return "", false
	}
}

// nonScalarHCLTypePrefixes are the collection/structural HCL type
// constructors -- a variable declared with any of these can never be
// satisfied by a plain CLI scalar argument, so InferVarType routes them to
// TypeYAML (which effectiveStackValueType/effectiveValueType then reject as
// non-scalar, per ErrTypeInferenceNonScalar) rather than the scalar cases
// above.
var nonScalarHCLTypePrefixes = []string{"list(", "set(", "map(", "object(", "tuple("}

// isNonScalarHCLType reports whether trimmed (already whitespace-trimmed)
// begins with one of nonScalarHCLTypePrefixes.
func isNonScalarHCLType(trimmed string) bool {
	for _, prefix := range nonScalarHCLTypePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}
