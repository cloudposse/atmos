package starlark

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestYAMLDecode(t *testing.T) {
	tests := []struct {
		name, content, expression string
	}{
		{"empty", "", "value == None"},
		{"nested", "checks:\n  - id: sample\n    fix: true\n    count: 150\n    optional: null\n", `value == {"checks": [{"id": "sample", "fix": True, "count": 150, "optional": None}]}`},
		{"scalar", "1.5", "value == 1.5"},
		{"custom tag", "value: !env HOME", `value == {"value": "HOME"}`},
		{"alias", "first: &item [one, two]\nsecond: *item\n", `value["first"] == ["one", "two"] and value["second"] == value["first"]`},
		{"large integer", "9007199254740993", "value == 9007199254740993"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New().Execute(t.Context(), script.Spec{
				Env:    map[string]string{"YAML": tt.content},
				Source: "value = yaml.decode(env[\"YAML\"])\nif not (" + tt.expression + "):\n    fail(str(value))",
			})
			require.NoError(t, err)
		})
	}
}

func TestYAMLDecodeRejectsInvalidData(t *testing.T) {
	for _, content := range []string{
		"a: [invalid", "a: 1\na: 2", "one\n---\ntwo", "one\n---\n",
		"one\n---\n[invalid", "? [a, b]\n: value", "1: value", "nested:\n  - 1: value", ".nan", "loop: &loop [*loop]",
	} {
		t.Run(content, func(t *testing.T) {
			_, err := New().Execute(t.Context(), script.Spec{Env: map[string]string{"YAML": content}, Source: `yaml.decode(env["YAML"])`})
			require.Error(t, err)
		})
	}
}
