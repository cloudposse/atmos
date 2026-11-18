package schema

import (
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRemoteReadIdentityFields verifies the flat `identity:` field decodes from YAML and from the
// map form Viper and the stack processor hand to mapstructure, on every remote-read declaration.
func TestRemoteReadIdentityFields(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		target any
		get    func(any) string
	}{
		{name: "vendor.yaml source", yaml: "component: vpc\nsource: github.com/acme/modules\nidentity: ops\n", target: &AtmosVendorSource{}, get: func(v any) string { return v.(*AtmosVendorSource).Identity }},
		{name: "component source", yaml: "uri: github.com/acme/modules\nidentity: ops\n", target: &VendorComponentSource{}, get: func(v any) string { return v.(*VendorComponentSource).Identity }},
		{name: "stack import", yaml: "path: github.com/acme/stacks//catalog\nidentity: ops\n", target: &StackImport{}, get: func(v any) string { return v.(*StackImport).Identity }},
		{name: "vendor section default", yaml: "identity: ops\n", target: &Vendor{}, get: func(v any) string { return v.(*Vendor).Identity }},
		{name: "imports section default", yaml: "identity: ops\n", target: &ImportsSettings{}, get: func(v any) string { return v.(*ImportsSettings).Identity }},
	}
	for _, tt := range tests {
		t.Run(tt.name+" from yaml", func(t *testing.T) {
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), tt.target))
			assert.Equal(t, "ops", tt.get(tt.target))
		})
		t.Run(tt.name+" from mapstructure", func(t *testing.T) {
			var raw map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &raw))
			require.NoError(t, mapstructure.Decode(raw, tt.target))
			assert.Equal(t, "ops", tt.get(tt.target))
		})
	}
}

func TestRemoteReadIdentityOmittedWhenEmpty(t *testing.T) {
	out, err := yaml.Marshal(&AtmosVendorSource{Component: "vpc"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "identity")
}
