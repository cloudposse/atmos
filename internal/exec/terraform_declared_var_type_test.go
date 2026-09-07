package exec

import (
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
)

func TestTerraformDeclaredVarType(t *testing.T) {
	componentSection := map[string]any{
		componentInfoKey: map[string]any{
			terraformConfigKey: &tfconfig.Module{Variables: map[string]*tfconfig.Variable{
				"replicas": {Name: "replicas", Type: "number"},
				"enabled":  {Name: "enabled", Type: "bool"},
				"anyvar":   {Name: "anyvar", Type: ""},
				"tags":     {Name: "tags", Type: "list(string)"},
			}},
		},
	}

	t.Run("declared type found", func(t *testing.T) {
		got, ok := TerraformDeclaredVarType(componentSection, "replicas")
		assert.True(t, ok)
		assert.Equal(t, "number", got)
	})

	t.Run("declared bool type found", func(t *testing.T) {
		got, ok := TerraformDeclaredVarType(componentSection, "enabled")
		assert.True(t, ok)
		assert.Equal(t, "bool", got)
	})

	t.Run("declared list type found", func(t *testing.T) {
		got, ok := TerraformDeclaredVarType(componentSection, "tags")
		assert.True(t, ok)
		assert.Equal(t, "list(string)", got)
	})

	t.Run("not declared", func(t *testing.T) {
		_, ok := TerraformDeclaredVarType(componentSection, "does_not_exist")
		assert.False(t, ok)
	})

	t.Run("declared with no explicit type (implicit any)", func(t *testing.T) {
		_, ok := TerraformDeclaredVarType(componentSection, "anyvar")
		assert.False(t, ok)
	})

	t.Run("non-terraform component (no component_info)", func(t *testing.T) {
		_, ok := TerraformDeclaredVarType(map[string]any{}, "replicas")
		assert.False(t, ok)
	})

	t.Run("component_info present but no terraform_config", func(t *testing.T) {
		_, ok := TerraformDeclaredVarType(map[string]any{
			componentInfoKey: map[string]any{},
		}, "replicas")
		assert.False(t, ok)
	})

	t.Run("nil module", func(t *testing.T) {
		_, ok := TerraformDeclaredVarType(map[string]any{
			componentInfoKey: map[string]any{
				terraformConfigKey: (*tfconfig.Module)(nil),
			},
		}, "replicas")
		assert.False(t, ok)
	})

	t.Run("nil componentSection", func(t *testing.T) {
		_, ok := TerraformDeclaredVarType(nil, "replicas")
		assert.False(t, ok)
	})
}
