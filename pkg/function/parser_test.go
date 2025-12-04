package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testCase represents a single parse test case.
type testCase struct {
	name      string
	input     string
	component string
	stack     string
	expr      string
}

// runTestCases runs a slice of test cases.
func runTestCases(t *testing.T, cases []testCase) {
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c, s, e := ParseArgs(tt.input)
			assert.Equal(t, tt.component, c, "component mismatch")
			assert.Equal(t, tt.stack, s, "stack mismatch")
			assert.Equal(t, tt.expr, e, "expr mismatch")
		})
	}
}

func TestParseArgs_BackwardCompatibility(t *testing.T) {
	cases := []testCase{
		// Simple cases - fully backward compatible.
		{"Simple output", "component-1 vpc_id", "component-1", "", "vpc_id"},
		{"With stack", "component-1 prod vpc_id", "component-1", "prod", "vpc_id"},
		{"Hyphenated names", "my-component-1 my-stack-1 vpc_id", "my-component-1", "my-stack-1", "vpc_id"},
		{"Underscored names", "my_component my_stack output_name", "my_component", "my_stack", "output_name"},
		{"Numbers in names", "component1 stack2 output3", "component1", "stack2", "output3"},
		{"Mixed case", "MyComponent MyStack OutputName", "MyComponent", "MyStack", "OutputName"},
	}
	runTestCases(t, cases)
}

func TestParseArgs_NewSyntax(t *testing.T) {
	cases := []testCase{
		// New clean syntax - dot expressions.
		{"Dot expression no stack", "component-2 .output", "component-2", "", ".output"},
		{"Dot expression with stack", "component-2 prod .output", "component-2", "prod", ".output"},
		{"Nested dot expression", "component-2 .foo.bar.baz", "component-2", "", ".foo.bar.baz"},

		// JSON fallback expressions.
		{"JSON fallback", `component-2 .output // {"key": "val"}`, "component-2", "", `.output // {"key": "val"}`},
		{"JSON with stack", `component-2 prod .output // {"k": "v"}`, "component-2", "prod", `.output // {"k": "v"}`},
		{
			"Complex JSON fallback", `component-2 .output // {"key1": "fallback1", "key2": "fallback2"}`,
			"component-2", "", `.output // {"key1": "fallback1", "key2": "fallback2"}`,
		},

		// Pipe expressions.
		{"Pipe expression", `component-2 .foo | "prefix:" + .`, "component-2", "", `.foo | "prefix:" + .`},
		{"Pipe with stack", `component-2 prod .foo | . + ":suffix"`, "component-2", "prod", `.foo | . + ":suffix"`},
		{
			"JDBC example", `component-2 .foo | "jdbc:postgresql://" + . + ":5432/events"`,
			"component-2", "", `.foo | "jdbc:postgresql://" + . + ":5432/events"`,
		},

		// Array and bracket access.
		{"Array index", `component-2 .[0].name`, "component-2", "", `.[0].name`},
		{"Bracket access", `component-2 ["key"].value`, "component-2", "", `["key"].value`},
		{"Mixed access", `component-2 .items[0]["key"]`, "component-2", "", `.items[0]["key"]`},

		// Edge cases with expression-starting characters.
		{"Starts with pipe", `component-2 | .foo`, "component-2", "", `| .foo`},
		{"Starts with brace", `component-2 {"key": .value}`, "component-2", "", `{"key": .value}`},
		{"Starts with quote", `component-2 "literal"`, "component-2", "", `"literal"`},
		{"Starts with single quote", `component-2 'literal'`, "component-2", "", `'literal'`},
	}
	runTestCases(t, cases)
}

func TestParseArgs_EdgeCases(t *testing.T) {
	cases := []testCase{
		// Empty and minimal inputs.
		{"Empty string", "", "", "", ""},
		{"Only spaces", "   ", "", "", ""},
		{"Just component", "component-1", "component-1", "", ""},
		{"Component with trailing space", "component-1 ", "component-1", "", ""},

		// Whitespace handling - spaces.
		{"Leading whitespace", "  component-1 vpc_id", "component-1", "", "vpc_id"},
		{"Trailing whitespace", "component-1 vpc_id  ", "component-1", "", "vpc_id"},
		{"Multiple spaces between", "component-1   vpc_id", "component-1", "", "vpc_id"},
		{"Multiple spaces with stack", "component-1   prod   vpc_id", "component-1", "prod", "vpc_id"},

		// Whitespace handling - tabs.
		{"Tab between tokens", "component-1\tvpc_id", "component-1", "", "vpc_id"},
		{"Multiple tabs", "component-1\t\tvpc_id", "component-1", "", "vpc_id"},
		{"Tab with stack", "component-1\tprod\tvpc_id", "component-1", "prod", "vpc_id"},
		{"Mixed spaces and tabs", "component-1 \t prod \t vpc_id", "component-1", "prod", "vpc_id"},

		// Whitespace handling - newlines (from YAML block scalars).
		{"Newline between tokens", "component-1\nvpc_id", "component-1", "", "vpc_id"},
		{"Newline with stack", "component-1\nprod\nvpc_id", "component-1", "prod", "vpc_id"},
		{"CRLF handling", "component-1\r\nvpc_id", "component-1", "", "vpc_id"},

		// Expression with internal spaces (should be preserved).
		{
			"Expression with spaces", `component-2 .output // {"key": "value with spaces"}`,
			"component-2", "", `.output // {"key": "value with spaces"}`,
		},
	}
	runTestCases(t, cases)
}

func TestParseArgs_YAMLBlockScalars(t *testing.T) {
	// These test cases simulate what YAML block scalars produce after parsing.
	// Folded block (>-) folds newlines to spaces.
	// Literal block (|) preserves newlines.
	cases := []testCase{
		// Folded block scalar result (newlines become spaces).
		{
			"Folded block result", `component-2 .output // {"key1": "fallback1", "key2": "fallback2"}`,
			"component-2", "", `.output // {"key1": "fallback1", "key2": "fallback2"}`,
		},
		{
			"Folded with stack", `component-2 prod .output // {"key": "val"}`,
			"component-2", "prod", `.output // {"key": "val"}`,
		},
	}
	runTestCases(t, cases)
}
