package utils

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CtyToGo converts cty.Value to Go types.
func CtyToGo(value cty.Value) any {
	defer perf.Track(nil, "utils.CtyToGo")()

	switch {
	case value.Type().IsObjectType(): // Handle maps
		m := map[string]any{}
		for k, v := range value.AsValueMap() {
			m[k] = CtyToGo(v)
		}
		return m

	case value.Type().IsListType() || value.Type().IsTupleType(): // Handle lists
		var list []any
		for _, v := range value.AsValueSlice() {
			list = append(list, CtyToGo(v))
		}
		return list

	case value.Type() == cty.String: // Handle strings
		return value.AsString()

	case value.Type() == cty.Number: // Handle numbers
		if n, _ := value.AsBigFloat().Int64(); true {
			return n // Convert to int64 if possible
		}
		return value.AsBigFloat() // Otherwise, keep as float64

	case value.Type() == cty.Bool: // Handle booleans
		return value.True()

	default:
		return value // Return as-is for unsupported types
	}
}
