package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// processContextProbe evaluates one !starlark expression with the given component info and returns its result.
func processContextProbe(t *testing.T, info *schema.ConfigAndStacksInfo, expression string) any {
	t.Helper()

	section := map[string]any{}
	for key, value := range info.ComponentSection {
		section[key] = value
	}
	vars := map[string]any{}
	if existing, ok := section["vars"].(map[string]any); ok {
		for key, value := range existing {
			vars[key] = value
		}
	}
	vars["probe"] = starlarkTestSource(expression)
	section["vars"] = vars
	info.ComponentSection = section

	result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, section, "dev", nil, info)
	require.NoError(t, err)
	return result["vars"].(map[string]any)["probe"]
}

func TestStarlarkContextComponentIsInstanceName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "instance name wins over the base component",
			info: schema.ConfigAndStacksInfo{ComponentFromArg: "ctxecho", Component: "mock", ComponentSection: map[string]any{"component": "mock"}},
			want: "ctxecho",
		},
		{
			name: "falls back to the component when no instance name is set",
			info: schema.ConfigAndStacksInfo{Component: "mock", ComponentSection: map[string]any{"component": "mock"}},
			want: "mock",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, processContextProbe(t, &tc.info, "return ctx.component"))
		})
	}
}

func TestStarlarkContextIdentityFields(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "ctxecho",
		ComponentType:    "terraform",
		ComponentSection: map[string]any{"component": "mock", "stack": "other", "component_type": "helmfile"},
	}
	assert.Equal(t, "terraform", processContextProbe(t, info, "return ctx.component_type"))
	assert.Equal(t, "dev", processContextProbe(t, info, "return ctx.stack"))
}

func TestStarlarkContextLocalsLayering(t *testing.T) {
	tests := []struct {
		name           string
		stackLocals    map[string]any
		componentLocal map[string]any
		want           map[string]any
	}{
		{
			name:           "component locals override stack locals",
			stackLocals:    map[string]any{"owner": "stack", "suffix": "blue"},
			componentLocal: map[string]any{"owner": "component", "extra": "x"},
			want:           map[string]any{"owner": "component", "suffix": "blue", "extra": "x"},
		},
		{
			name:        "stack locals alone",
			stackLocals: map[string]any{"owner": "stack"},
			want:        map[string]any{"owner": "stack"},
		},
		{
			name:           "component locals alone",
			componentLocal: map[string]any{"owner": "component"},
			want:           map[string]any{"owner": "component"},
		},
		{
			name: "no locals",
			want: map[string]any{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			section := map[string]any{}
			if tc.componentLocal != nil {
				section["locals"] = tc.componentLocal
			}
			info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", ComponentSection: section, StackLocalsSection: tc.stackLocals}
			assert.Equal(t, tc.want, processContextProbe(t, info, "return ctx.locals"))
		})
	}
}

func TestStarlarkContextLocalsDoNotMutateInputs(t *testing.T) {
	stackLocals := map[string]any{"owner": "stack"}
	componentLocals := map[string]any{"owner": "component"}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"locals": componentLocals}, StackLocalsSection: stackLocals}
	processContextProbe(t, info, "return ctx.locals")
	assert.Equal(t, map[string]any{"owner": "stack"}, stackLocals)
	assert.Equal(t, map[string]any{"owner": "component"}, componentLocals)
}

func TestStackLocalsForComponent(t *testing.T) {
	raw := map[string]map[string]any{
		"deploy/dev": {"stack": map[string]any{
			"locals":    map[string]any{"owner": "global", "shared": "global"},
			"terraform": map[string]any{"locals": map[string]any{"shared": "terraform"}},
			"helmfile":  map[string]any{"locals": map[string]any{"shared": "helmfile"}},
		}},
		"deploy/none": {"stack": map[string]any{"vars": map[string]any{"a": 1}}},
	}
	assert.Equal(t, map[string]any{"owner": "global", "shared": "terraform"}, stackLocalsForComponent(raw, "deploy/dev", "terraform"))
	assert.Equal(t, map[string]any{"owner": "global", "shared": "helmfile"}, stackLocalsForComponent(raw, "deploy/dev", "helmfile"))
	assert.Nil(t, stackLocalsForComponent(raw, "deploy/none", "terraform"))
	assert.Nil(t, stackLocalsForComponent(raw, "deploy/missing", "terraform"))

	got := stackLocalsForComponent(raw, "deploy/dev", "terraform")
	got["owner"] = "mutated"
	assert.Equal(t, "global", raw["deploy/dev"]["stack"].(map[string]any)["locals"].(map[string]any)["owner"], "result must not alias the manifest")
}
