package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"

	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestVarNameFromRelPath(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		want    string
		wantOK  bool
	}{
		{"top-level var", "vars.replicas", "replicas", true},
		{"nested attribute path", "vars.foo.bar", "", false},
		{"indexed path", "vars.foo[0]", "", false},
		{"non-vars path", "settings.enabled", "", false},
		{"bare vars with nothing after it", "vars", "", false},
		{"empty path", "", "", false},
		{"metadata path", "metadata.component", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := VarNameFromRelPath(tt.relPath)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferVarType(t *testing.T) {
	tests := []struct {
		name     string
		hclType  string
		rawValue string
		want     string
		wantOK   bool
	}{
		{"string type always string, even for a numeric-looking value", "string", "1.0", atmosyaml.TypeString, true},
		{"bool type", "bool", "true", atmosyaml.TypeBool, true},
		{"number type with int-shaped value", "number", "5", atmosyaml.TypeInt, true},
		{"number type with float-shaped value", "number", "5.5", atmosyaml.TypeFloat, true},
		{"number type with unparseable value", "number", "not-a-number", "", false},
		{"list type", "list(string)", "ignored", atmosyaml.TypeYAML, true},
		{"set type", "set(string)", "ignored", atmosyaml.TypeYAML, true},
		{"map type", "map(string)", "ignored", atmosyaml.TypeYAML, true},
		{"object type", "object({foo=string})", "ignored", atmosyaml.TypeYAML, true},
		{"tuple type", "tuple([string])", "ignored", atmosyaml.TypeYAML, true},
		{"empty type (implicit any)", "", "5", "", false},
		{"explicit any", "any", "5", "", false},
		{"unrecognized type text", "some_future_hcl_type", "5", "", false},
		{"whitespace-padded type text", "  string  ", "hello", atmosyaml.TypeString, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := InferVarType(tt.hclType, tt.rawValue)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
