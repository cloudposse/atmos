package naming

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolverLiteralContextAcrossNamingStrategies(t *testing.T) {
	for _, tc := range []struct {
		name, explicit, template, pattern, want string
	}{
		{name: "manifest", explicit: "explicit", template: "unused", pattern: "{tenant}", want: "explicit"},
		{name: "template", template: "configured template", pattern: "{tenant}", want: "from-template"},
		{name: "pattern", pattern: "{environment}-{stage}", want: "ue2-dev"},
		{name: "filename", want: "deploy/app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]any{
				"stage": literalsource.Encode("dev"), "environment": literalsource.Encode("ue2"),
				"attributes": []any{literalsource.Encode("{{external.attribute}}")},
			}
			before, err := json.Marshal(vars)
			require.NoError(t, err)
			resolver := Resolver{StackTemplate: func(_ *schema.AtmosConfiguration, _, source string, data any, _ bool) (string, error) {
				assert.Equal(t, tc.template, source)
				assert.Equal(t, literalsource.Encode("dev"), data.(map[string]any)["vars"].(map[string]any)["stage"], "template input remains protected")
				return "from-template", nil
			}}
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NameTemplate: tc.template, NamePattern: tc.pattern}}
			info := schema.ConfigAndStacksInfo{
				Stack: "deploy/app", StackManifestName: tc.explicit, ComponentFromArg: "app",
				ComponentVarsSection: vars, ComponentSection: map[string]any{"vars": vars},
				ComponentMetadataSection: map[string]any{"terraform_workspace_pattern": "{environment}-{stage}"},
				Context:                  schema.Context{Stage: "stale", Tenant: "stale"},
			}
			require.NoError(t, resolver.ProcessStackContextPrefix(config, &info, "deploy/app", tc.explicit))
			assert.Equal(t, tc.want, info.ContextPrefix)
			assert.Equal(t, "dev", info.Context.Stage)
			assert.Equal(t, "ue2", info.Context.Environment)
			assert.Empty(t, info.Context.Tenant, "previous component context is cleared")
			assert.Equal(t, []any{"{{external.attribute}}"}, info.Context.Attributes)
			workspace, err := resolver.BuildTerraformWorkspace(config, &info)
			require.NoError(t, err)
			assert.Equal(t, "ue2-dev", workspace)
			require.Len(t, info.Context.Attributes, 1)
			info.Context.Attributes[0] = "changed result"
			after, err := json.Marshal(vars)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			vars["attributes"].([]any)[0] = "changed source"
			assert.Equal(t, []any{"changed result"}, info.Context.Attributes)
		})
	}
}

func TestResolverLiteralPatternAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, want string
	}{
		{name: "pattern", pattern: "{environment}-{stage}", want: "ue2-dev"},
		{name: "fallback", pattern: "{tenant}-{stage}", want: "deploy/app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]any{"stage": literalsource.Encode("dev"), "environment": literalsource.Encode("ue2")}
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: tc.pattern}}
			name, context, err := (Resolver{}).ResolveStackName(config, "deploy/app", "", schema.ConfigAndStacksInfo{}, vars)
			require.NoError(t, err)
			assert.Equal(t, tc.want, name)
			assert.Equal(t, "dev", context.Stage)
			assert.Equal(t, "ue2", context.Environment)
			assert.Equal(t, literalsource.Encode("dev"), vars["stage"])
			assert.Equal(t, literalsource.Encode("ue2"), vars["environment"])
		})
	}
}
