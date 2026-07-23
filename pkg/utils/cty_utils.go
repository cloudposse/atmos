package utils

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/pkg/perf"
)

// GoToCty converts Go types to cty.Value recursively.
//
//nolint:revive // Cyclomatic complexity is justified for comprehensive type conversion.
func GoToCty(value any) cty.Value {
	defer perf.Track(nil, "utils.GoToCty")()

	if value == nil {
		return cty.NilVal
	}

	switch v := value.(type) {
	case string:
		return cty.StringVal(v)

	case bool:
		return cty.BoolVal(v)

	case int:
		return cty.NumberIntVal(int64(v))

	case int64:
		return cty.NumberIntVal(v)

	case uint64:
		return cty.NumberUIntVal(v)

	case float64:
		return cty.NumberFloatVal(v)

	case map[string]any:
		// Convert map to cty object.
		objMap := make(map[string]cty.Value, len(v))
		for k, val := range v {
			objMap[k] = GoToCty(val)
		}
		return cty.ObjectVal(objMap)

	case []any:
		// Convert slice to cty tuple.
		if len(v) == 0 {
			return cty.EmptyTupleVal
		}
		tupleVals := make([]cty.Value, len(v))
		for i, val := range v {
			tupleVals[i] = GoToCty(val)
		}
		return cty.TupleVal(tupleVals)

	default:
		// For unsupported types, return NilVal.
		return cty.NilVal
	}
}
