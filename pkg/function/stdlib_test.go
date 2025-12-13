package function

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestStdlibFunctions(t *testing.T) {
	t.Run("returns all expected functions", func(t *testing.T) {
		funcs := StdlibFunctions()

		// String functions.
		assert.Contains(t, funcs, "lower")
		assert.Contains(t, funcs, "upper")
		assert.Contains(t, funcs, "title")
		assert.Contains(t, funcs, "chomp")
		assert.Contains(t, funcs, "trimspace")
		assert.Contains(t, funcs, "trim")
		assert.Contains(t, funcs, "trimprefix")
		assert.Contains(t, funcs, "trimsuffix")
		assert.Contains(t, funcs, "strlen")
		assert.Contains(t, funcs, "substr")
		assert.Contains(t, funcs, "replace")
		assert.Contains(t, funcs, "split")
		assert.Contains(t, funcs, "join")
		assert.Contains(t, funcs, "format")
		assert.Contains(t, funcs, "formatlist")
		assert.Contains(t, funcs, "reverse")
		assert.Contains(t, funcs, "indent")

		// Collection functions.
		assert.Contains(t, funcs, "length")
		assert.Contains(t, funcs, "concat")
		assert.Contains(t, funcs, "contains")
		assert.Contains(t, funcs, "distinct")
		assert.Contains(t, funcs, "element")
		assert.Contains(t, funcs, "flatten")
		assert.Contains(t, funcs, "keys")
		assert.Contains(t, funcs, "values")
		assert.Contains(t, funcs, "lookup")
		assert.Contains(t, funcs, "merge")
		assert.Contains(t, funcs, "range")
		assert.Contains(t, funcs, "slice")
		assert.Contains(t, funcs, "sort")
		assert.Contains(t, funcs, "compact")
		assert.Contains(t, funcs, "chunklist")
		assert.Contains(t, funcs, "zipmap")
		assert.Contains(t, funcs, "index")

		// Numeric functions.
		assert.Contains(t, funcs, "abs")
		assert.Contains(t, funcs, "ceil")
		assert.Contains(t, funcs, "floor")
		assert.Contains(t, funcs, "log")
		assert.Contains(t, funcs, "max")
		assert.Contains(t, funcs, "min")
		assert.Contains(t, funcs, "pow")
		assert.Contains(t, funcs, "signum")
		assert.Contains(t, funcs, "parseint")

		// Encoding functions.
		assert.Contains(t, funcs, "jsonencode")
		assert.Contains(t, funcs, "jsondecode")
		assert.Contains(t, funcs, "csvdecode")

		// Date/time functions.
		assert.Contains(t, funcs, "formatdate")
		assert.Contains(t, funcs, "timeadd")

		// Regex functions.
		assert.Contains(t, funcs, "regex")
		assert.Contains(t, funcs, "regexall")

		// Type functions.
		assert.Contains(t, funcs, "coalesce")
		assert.Contains(t, funcs, "coalescelist")

		// Set functions.
		assert.Contains(t, funcs, "setintersection")
		assert.Contains(t, funcs, "setsubtract")
		assert.Contains(t, funcs, "setunion")
		assert.Contains(t, funcs, "setproduct")

		// Boolean functions.
		assert.Contains(t, funcs, "and")
		assert.Contains(t, funcs, "or")
		assert.Contains(t, funcs, "not")

		// Comparison functions.
		assert.Contains(t, funcs, "equal")
		assert.Contains(t, funcs, "notequal")
		assert.Contains(t, funcs, "lessthan")
		assert.Contains(t, funcs, "greaterthan")
	})
}

func TestStdlibFunctionsInHCL(t *testing.T) {
	tests := []struct {
		name     string
		hcl      string
		key      string
		expected any
	}{
		// String functions.
		{
			name:     "lower",
			hcl:      `r = lower("HELLO")`,
			key:      "r",
			expected: "hello",
		},
		{
			name:     "upper",
			hcl:      `r = upper("hello")`,
			key:      "r",
			expected: "HELLO",
		},
		{
			name:     "title",
			hcl:      `r = title("hello world")`,
			key:      "r",
			expected: "Hello World",
		},
		{
			name:     "chomp",
			hcl:      "r = chomp(\"hello\\n\")",
			key:      "r",
			expected: "hello",
		},
		{
			name:     "trimspace",
			hcl:      `r = trimspace("  hi  ")`,
			key:      "r",
			expected: "hi",
		},
		{
			name:     "trim",
			hcl:      `r = trim("?!hello?!", "?!")`,
			key:      "r",
			expected: "hello",
		},
		{
			name:     "trimprefix",
			hcl:      `r = trimprefix("helloworld", "hello")`,
			key:      "r",
			expected: "world",
		},
		{
			name:     "trimsuffix",
			hcl:      `r = trimsuffix("helloworld", "world")`,
			key:      "r",
			expected: "hello",
		},
		{
			name:     "strlen",
			hcl:      `r = strlen("hello")`,
			key:      "r",
			expected: int64(5),
		},
		{
			name:     "substr",
			hcl:      `r = substr("hello", 0, 2)`,
			key:      "r",
			expected: "he",
		},
		{
			name:     "replace",
			hcl:      `r = replace("hello", "l", "L")`,
			key:      "r",
			expected: "heLLo",
		},
		{
			name:     "format",
			hcl:      `r = format("Hello, %s!", "World")`,
			key:      "r",
			expected: "Hello, World!",
		},
		{
			name:     "indent",
			hcl:      `r = indent(2, "hello")`,
			key:      "r",
			expected: "hello",
		},

		// Collection functions.
		{
			name:     "length_list",
			hcl:      `r = length([1, 2, 3])`,
			key:      "r",
			expected: int64(3),
		},
		{
			name:     "length_tuple",
			hcl:      `r = length(["a", "b"])`,
			key:      "r",
			expected: int64(2),
		},
		{
			name:     "contains_true",
			hcl:      `r = contains(["a", "b", "c"], "b")`,
			key:      "r",
			expected: true,
		},
		{
			name:     "contains_false",
			hcl:      `r = contains(["a", "b", "c"], "d")`,
			key:      "r",
			expected: false,
		},
		{
			name:     "element",
			hcl:      `r = element(["a", "b", "c"], 1)`,
			key:      "r",
			expected: "b",
		},

		// Numeric functions.
		{
			name:     "abs_positive",
			hcl:      `r = abs(5)`,
			key:      "r",
			expected: int64(5),
		},
		{
			name:     "abs_negative",
			hcl:      `r = abs(-5)`,
			key:      "r",
			expected: int64(5),
		},
		{
			name:     "ceil",
			hcl:      `r = ceil(4.3)`,
			key:      "r",
			expected: int64(5),
		},
		{
			name:     "floor",
			hcl:      `r = floor(4.7)`,
			key:      "r",
			expected: int64(4),
		},
		{
			name:     "max",
			hcl:      `r = max(1, 5, 3)`,
			key:      "r",
			expected: int64(5),
		},
		{
			name:     "min",
			hcl:      `r = min(1, 5, 3)`,
			key:      "r",
			expected: int64(1),
		},
		{
			name:     "pow",
			hcl:      `r = pow(2, 3)`,
			key:      "r",
			expected: int64(8),
		},
		{
			name:     "signum_positive",
			hcl:      `r = signum(5)`,
			key:      "r",
			expected: int64(1),
		},
		{
			name:     "signum_negative",
			hcl:      `r = signum(-5)`,
			key:      "r",
			expected: int64(-1),
		},
		{
			name:     "signum_zero",
			hcl:      `r = signum(0)`,
			key:      "r",
			expected: int64(0),
		},

		// Boolean functions.
		{
			name:     "and_true",
			hcl:      `r = and(true, true)`,
			key:      "r",
			expected: true,
		},
		{
			name:     "and_false",
			hcl:      `r = and(true, false)`,
			key:      "r",
			expected: false,
		},
		{
			name:     "or_true",
			hcl:      `r = or(true, false)`,
			key:      "r",
			expected: true,
		},
		{
			name:     "or_false",
			hcl:      `r = or(false, false)`,
			key:      "r",
			expected: false,
		},
		{
			name:     "not_true",
			hcl:      `r = not(false)`,
			key:      "r",
			expected: true,
		},
		{
			name:     "not_false",
			hcl:      `r = not(true)`,
			key:      "r",
			expected: false,
		},

		// Comparison functions.
		{
			name:     "equal_true",
			hcl:      `r = equal(5, 5)`,
			key:      "r",
			expected: true,
		},
		{
			name:     "equal_false",
			hcl:      `r = equal(5, 3)`,
			key:      "r",
			expected: false,
		},
		{
			name:     "lessthan_true",
			hcl:      `r = lessthan(3, 5)`,
			key:      "r",
			expected: true,
		},
		{
			name:     "lessthan_false",
			hcl:      `r = lessthan(5, 3)`,
			key:      "r",
			expected: false,
		},
		{
			name:     "greaterthan_true",
			hcl:      `r = greaterthan(5, 3)`,
			key:      "r",
			expected: true,
		},
		{
			name:     "greaterthan_false",
			hcl:      `r = greaterthan(3, 5)`,
			key:      "r",
			expected: false,
		},
	}

	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHCLWithStdlib(t, tt.hcl, evalCtx)
			assert.Equal(t, tt.expected, result[tt.key])
		})
	}
}

func TestStdlibCollectionFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("split", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = split(",", "a,b,c")`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, "a", list[0])
		assert.Equal(t, "b", list[1])
		assert.Equal(t, "c", list[2])
	})

	t.Run("join", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = join("-", ["a", "b", "c"])`, evalCtx)
		assert.Equal(t, "a-b-c", result["r"])
	})

	t.Run("concat", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = concat(["a"], ["b", "c"])`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, "a", list[0])
		assert.Equal(t, "b", list[1])
		assert.Equal(t, "c", list[2])
	})

	t.Run("distinct", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = distinct(["a", "b", "a", "c", "b"])`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
	})

	t.Run("flatten", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = flatten([["a", "b"], ["c"]])`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, "a", list[0])
		assert.Equal(t, "b", list[1])
		assert.Equal(t, "c", list[2])
	})

	t.Run("reverse_string", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = reverse("abc")`, evalCtx)
		assert.Equal(t, "cba", result["r"])
	})

	t.Run("sort", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = sort(["c", "a", "b"])`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, "a", list[0])
		assert.Equal(t, "b", list[1])
		assert.Equal(t, "c", list[2])
	})

	t.Run("compact", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = compact(["a", "", "b", ""])`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 2)
		assert.Equal(t, "a", list[0])
		assert.Equal(t, "b", list[1])
	})

	t.Run("slice", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = slice(["a", "b", "c", "d"], 1, 3)`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 2)
		assert.Equal(t, "b", list[0])
		assert.Equal(t, "c", list[1])
	})

	t.Run("chunklist", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = chunklist(["a", "b", "c", "d"], 2)`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 2)
	})
}

func TestStdlibMapFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("keys", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = keys({a = 1, b = 2})`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 2)
		// Keys are sorted alphabetically.
		assert.Contains(t, list, "a")
		assert.Contains(t, list, "b")
	})

	t.Run("values", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = values({a = 1, b = 2})`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 2)
	})

	t.Run("lookup_found", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = lookup({a = "one", b = "two"}, "a", "default")`, evalCtx)
		assert.Equal(t, "one", result["r"])
	})

	t.Run("lookup_default", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = lookup({a = "one"}, "c", "default")`, evalCtx)
		assert.Equal(t, "default", result["r"])
	})

	t.Run("merge", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = merge({a = 1}, {b = 2})`, evalCtx)
		m := result["r"].(map[string]any)
		assert.Len(t, m, 2)
		assert.Equal(t, int64(1), m["a"])
		assert.Equal(t, int64(2), m["b"])
	})

	t.Run("zipmap", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = zipmap(["a", "b"], [1, 2])`, evalCtx)
		m := result["r"].(map[string]any)
		assert.Len(t, m, 2)
		assert.Equal(t, int64(1), m["a"])
		assert.Equal(t, int64(2), m["b"])
	})
}

func TestStdlibEncodingFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("jsonencode", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = jsonencode({a = 1, b = "two"})`, evalCtx)
		jsonStr := result["r"].(string)
		assert.Contains(t, jsonStr, `"a":1`)
		assert.Contains(t, jsonStr, `"b":"two"`)
	})

	t.Run("jsondecode", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = jsondecode("{\"a\":1,\"b\":\"two\"}")`, evalCtx)
		m := result["r"].(map[string]any)
		assert.Equal(t, int64(1), m["a"])
		assert.Equal(t, "two", m["b"])
	})
}

func TestStdlibSetFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("setunion", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = setunion(["a", "b"], ["b", "c"])`, evalCtx)
		set := result["r"].([]any)
		assert.Len(t, set, 3)
	})

	t.Run("setintersection", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = setintersection(["a", "b", "c"], ["b", "c", "d"])`, evalCtx)
		set := result["r"].([]any)
		assert.Len(t, set, 2)
	})

	t.Run("setsubtract", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = setsubtract(["a", "b", "c"], ["b"])`, evalCtx)
		set := result["r"].([]any)
		assert.Len(t, set, 2)
	})
}

func TestStdlibRegexFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("regex", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = regex("[a-z]+", "hello123")`, evalCtx)
		assert.Equal(t, "hello", result["r"])
	})

	t.Run("regexall", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = regexall("[a-z]+", "hi bye")`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 2)
		assert.Equal(t, "hi", list[0])
		assert.Equal(t, "bye", list[1])
	})
}

func TestStdlibRangeFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("range_simple", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = range(3)`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, int64(0), list[0])
		assert.Equal(t, int64(1), list[1])
		assert.Equal(t, int64(2), list[2])
	})

	t.Run("range_start_end", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = range(1, 4)`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, int64(1), list[0])
		assert.Equal(t, int64(2), list[1])
		assert.Equal(t, int64(3), list[2])
	})
}

func TestStdlibCoalesceFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	// Note: coalesce in go-cty returns the first non-null value, not non-empty.
	// Empty strings are valid values.
	t.Run("coalesce_first_value", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = coalesce("first", "second")`, evalCtx)
		assert.Equal(t, "first", result["r"])
	})

	t.Run("coalesce_empty_string_is_valid", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = coalesce("", "a", "b")`, evalCtx)
		// Empty string is the first non-null value, so it wins.
		assert.Equal(t, "", result["r"])
	})
}

func TestStdlibMathFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("add", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = add(2, 3)`, evalCtx)
		assert.Equal(t, int64(5), result["r"])
	})

	t.Run("subtract", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = subtract(5, 3)`, evalCtx)
		assert.Equal(t, int64(2), result["r"])
	})

	t.Run("multiply", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = multiply(4, 3)`, evalCtx)
		assert.Equal(t, int64(12), result["r"])
	})

	t.Run("divide", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = divide(10, 2)`, evalCtx)
		assert.Equal(t, int64(5), result["r"])
	})

	t.Run("modulo", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = modulo(10, 3)`, evalCtx)
		assert.Equal(t, int64(1), result["r"])
	})

	t.Run("negate", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = negate(5)`, evalCtx)
		assert.Equal(t, int64(-5), result["r"])
	})

	t.Run("log", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = log(100, 10)`, evalCtx)
		assert.InDelta(t, float64(2), result["r"], 0.0001)
	})
}

func TestStdlibIntFunction(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("int_from_float", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = int(4.9)`, evalCtx)
		assert.Equal(t, int64(4), result["r"])
	})
}

func TestStdlibParseIntFunction(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("parseint_decimal", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = parseint("42", 10)`, evalCtx)
		assert.Equal(t, int64(42), result["r"])
	})

	t.Run("parseint_hex", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = parseint("ff", 16)`, evalCtx)
		assert.Equal(t, int64(255), result["r"])
	})

	t.Run("parseint_binary", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = parseint("1010", 2)`, evalCtx)
		assert.Equal(t, int64(10), result["r"])
	})
}

func TestStdlibFormatListFunction(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("formatlist_simple", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = formatlist("Hello, %s!", ["A", "B", "C"])`, evalCtx)
		list := result["r"].([]any)
		assert.Len(t, list, 3)
		assert.Equal(t, "Hello, A!", list[0])
		assert.Equal(t, "Hello, B!", list[1])
		assert.Equal(t, "Hello, C!", list[2])
	})
}

func TestStdlibIndexFunction(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	// The index function in go-cty is for accessing a value by numeric index.
	t.Run("index_tuple_lookup", func(t *testing.T) {
		result := parseHCLWithStdlib(t, `r = index(["a", "b", "c"], 1)`, evalCtx)
		assert.Equal(t, "b", result["r"])
	})
}

// parseHCLWithStdlib is a helper that parses HCL content with stdlib functions.
func parseHCLWithStdlib(t *testing.T, content string, evalCtx *hcl.EvalContext) map[string]any {
	t.Helper()

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(content), "test.hcl")
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	attrs, diags := file.Body.JustAttributes()
	require.False(t, diags.HasErrors(), "attributes error: %s", diags.Error())

	result := make(map[string]any)
	for name, attr := range attrs {
		val, valDiags := attr.Expr.Value(evalCtx)
		require.False(t, valDiags.HasErrors(), "value error for %s: %s", name, valDiags.Error())
		result[name] = ctyValueToGo(val)
	}

	return result
}

// ctyValueToGo converts a cty.Value to a Go value for testing.
func ctyValueToGo(val cty.Value) any {
	if val.IsNull() {
		return nil
	}

	switch {
	case val.Type().Equals(cty.String):
		return val.AsString()
	case val.Type().Equals(cty.Number):
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i, _ := bf.Int64()
			return i
		}
		f, _ := bf.Float64()
		return f
	case val.Type().Equals(cty.Bool):
		return val.True()
	case val.Type().IsListType() || val.Type().IsTupleType() || val.Type().IsSetType():
		var list []any
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			list = append(list, ctyValueToGo(v))
		}
		return list
	case val.Type().IsMapType() || val.Type().IsObjectType():
		m := make(map[string]any)
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			m[k.AsString()] = ctyValueToGo(v)
		}
		return m
	default:
		return val.GoString()
	}
}
