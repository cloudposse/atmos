package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestInferValueType(t *testing.T) {
	tests := []struct {
		name     string
		dotPath  string
		wantType string
		wantOK   bool
	}{
		{name: "bool field", dotPath: "mcp.enabled", wantType: atmosyaml.TypeBool, wantOK: true},
		{name: "bool field case-insensitive", dotPath: "MCP.Enabled", wantType: atmosyaml.TypeBool, wantOK: true},
		{name: "string field", dotPath: "base_path", wantType: atmosyaml.TypeString, wantOK: true},
		{name: "unknown top-level segment", dotPath: "nonexistent.field", wantOK: false},
		{name: "unknown nested segment", dotPath: "mcp.nonexistent", wantOK: false},
		{name: "empty path", dotPath: "", wantOK: false},
		{name: "path with array index not supported", dotPath: "mcp.servers[0].command", wantOK: false},
		{name: "trailing dot is invalid", dotPath: "mcp.", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := InferValueType(tt.dotPath)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantType, got)
			}
		})
	}
}

func TestInferValueType_MapAndStructFallToYAML(t *testing.T) {
	// mcp.servers is a map[string]MCPServerConfig -- a "complex" field that
	// should resolve to TypeYAML (raw literal), not one of the scalar types.
	got, ok := InferValueType("mcp.servers")
	assert.True(t, ok)
	assert.Equal(t, atmosyaml.TypeYAML, got)
}
