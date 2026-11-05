package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

const inheritedComponentFixture = "../../tests/fixtures/scenarios/inherited-component-path"

func TestInheritedComponentPath(t *testing.T) {
	falseValue := false
	tests := []struct {
		name       string
		parents    []any
		localPath  string
		legacyPath string
		inherit    *bool
		want       string
	}{
		{name: "inherited path", parents: []any{"defaults"}, want: "lxd/talos-nodes"},
		{name: "explicit override", parents: []any{"defaults"}, localPath: "explicit", want: "explicit"},
		{name: "legacy path", parents: []any{"defaults"}, legacyPath: "legacy", want: "legacy"},
		{name: "explicit overrides legacy", parents: []any{"defaults"}, localPath: "explicit", legacyPath: "legacy", want: "explicit"},
		{name: "later parent wins", parents: []any{"defaults", "other"}, want: "other-path"},
		{name: "mixin without path preserves inherited path", parents: []any{"defaults", "mixin"}, want: "lxd/talos-nodes"},
		{name: "nested inheritance", parents: []any{"nested"}, want: "lxd/talos-nodes"},
		{name: "inheritance disabled", parents: []any{"defaults"}, inherit: &falseValue, want: "k8s_nodes"},
		{name: "explicit with inheritance disabled", parents: []any{"defaults"}, localPath: "explicit", inherit: &falseValue, want: "explicit"},
		{name: "parent without path", parents: []any{"mixin"}, want: "k8s_nodes"},
		{name: "no parents", want: "k8s_nodes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearBaseComponentConfigCache()
			t.Cleanup(ClearBaseComponentConfigCache)
			metadata := map[string]any{}
			if tt.parents != nil {
				metadata["inherits"] = tt.parents
			}
			if tt.localPath != "" {
				metadata["component"] = tt.localPath
			}
			component := map[string]any{"metadata": metadata}
			if tt.legacyPath != "" {
				component["component"] = tt.legacyPath
			}
			config := &schema.AtmosConfiguration{}
			config.Stacks.Inherit.Metadata = tt.inherit
			result, err := processComponent(&ComponentProcessorOptions{
				AtmosConfig: config, ComponentType: cfg.TerraformComponentType,
				Component: "k8s_nodes", Stack: "test", StackName: "test", ComponentMap: component,
				AllComponentsMap: map[string]any{
					"defaults": map[string]any{"metadata": map[string]any{"type": "abstract", "component": "lxd/talos-nodes"}},
					"other":    map[string]any{"metadata": map[string]any{"type": "abstract", "component": "other-path"}},
					"nested":   map[string]any{"metadata": map[string]any{"type": "abstract", "inherits": []any{"defaults"}}},
					"mixin":    map[string]any{"vars": map[string]any{"enabled": true}},
					"legacy":   map[string]any{"vars": map[string]any{"enabled": true}},
				},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.BaseComponentName)
		})
	}
}

func TestInheritedComponentPathBackend(t *testing.T) {
	for _, tt := range []struct {
		name         string
		metadataName string
		prefix       string
		wantPrefix   string
	}{
		{name: "inherited directory", wantPrefix: "lxd-talos-nodes"},
		{name: "stable metadata name", metadataName: "nodes", wantPrefix: "nodes"},
		{name: "explicit prefix", metadataName: "nodes", prefix: "existing-state", wantPrefix: "existing-state"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ClearBaseComponentConfigCache()
			t.Cleanup(ClearBaseComponentConfigCache)
			config := &schema.AtmosConfiguration{}
			opts := &ComponentProcessorOptions{
				AtmosConfig: config, ComponentType: cfg.TerraformComponentType,
				Component: "k8s_nodes", Stack: "test", StackName: "test",
				ComponentMap: map[string]any{
					"metadata":     map[string]any{"inherits": []any{"defaults"}},
					"backend_type": "s3",
					"backend":      map[string]any{"s3": map[string]any{"workspace_key_prefix": tt.prefix}},
				},
				AllComponentsMap: map[string]any{
					"defaults": map[string]any{"metadata": map[string]any{
						"type": "abstract", "component": "lxd/talos-nodes", "name": tt.metadataName,
					}},
				},
			}
			result, err := processComponent(opts)
			require.NoError(t, err)
			section, _, err := mergeComponentConfigurations(config, opts, result)
			require.NoError(t, err)
			assert.Equal(t, "lxd/talos-nodes", section["component"])
			backend, ok := section["backend"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantPrefix, backend["workspace_key_prefix"])
		})
	}
}

func TestInheritedComponentPathInvalidMetadata(t *testing.T) {
	result := &ComponentProcessorResult{
		ComponentMetadata:     map[string]any{},
		BaseComponentMetadata: map[string]any{"component": 42},
	}
	err := processMetadataInheritance(&ComponentProcessorOptions{
		AtmosConfig: &schema.AtmosConfiguration{}, ComponentType: cfg.TerraformComponentType,
		Component: "k8s_nodes", StackName: "test",
	}, result, &schema.BaseComponentConfig{}, &[]string{})
	require.ErrorIs(t, err, errUtils.ErrInvalidComponentMetadataComponent)
}

func TestDescribeComponentInheritedComponentPath(t *testing.T) {
	t.Chdir(inheritedComponentFixture)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "k8s_nodes", Stack: "test",
	})
	require.NoError(t, err)
	assert.Equal(t, "lxd/talos-nodes", section["component"])
	metadata, ok := section["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "lxd/talos-nodes", metadata["component"])
	assert.NotContains(t, metadata, "type", "the concrete component must not inherit abstract type")
	info, ok := section["component_info"].(map[string]any)
	require.True(t, ok)
	wantPath, err := filepath.Abs("components/terraform/lxd/talos-nodes")
	require.NoError(t, err)
	componentPath, ok := info["component_path"].(string)
	require.True(t, ok)
	assert.Equal(t, filepath.ToSlash(wantPath), filepath.ToSlash(componentPath))
	require.NoDirExists(t, "components/terraform/k8s_nodes")
}

func TestDescribeAffectedInheritedComponentPath(t *testing.T) {
	files := map[string]string{}
	modulePath := "components/terraform/lxd/talos-nodes/main.tf"
	for _, path := range []string{"atmos.yaml", "stacks/test.yaml", modulePath} {
		content, err := os.ReadFile(filepath.Join(inheritedComponentFixture, path))
		require.NoError(t, err)
		files[path] = string(content)
	}
	basePath, baseRepo := affectedPolicyRepo(t, files)
	files[modulePath] = "output \"example\" { value = \"changed\" }\n"
	headPath, headRepo := affectedPolicyRepo(t, files)
	t.Chdir(headPath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	componentFuncSyncMap.Clear()
	t.Cleanup(componentFuncSyncMap.Clear)
	ClearBaseComponentConfigCache()
	t.Cleanup(ClearBaseComponentConfigCache)
	ClearResolutionContext()
	t.Cleanup(ClearResolutionContext)
	executor := NewMockComponentFuncOutputsExecutor(gomock.NewController(t))
	original := componentFuncOutputsExecutor
	componentFuncOutputsExecutor = executor
	t.Cleanup(func() { componentFuncOutputsExecutor = original })
	// Mock only Terraform output execution; stack inheritance and template evaluation remain real.
	executor.EXPECT().ExecuteWithSections(gomock.Any(), "k8s_nodes", "test", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ *schema.AtmosConfiguration, _, _ string, sections map[string]any, _ *schema.AuthContext) (map[string]any, error) {
			assert.Equal(t, "lxd/talos-nodes", sections["component"])
			return map[string]any{"example": "test-output"}, nil
		}).MinTimes(1)

	affected, _, _, err := executeDescribeAffected(
		&config, headPath, basePath, headRepo, baseRepo,
		false, false, "test", true, false, nil, AffectedFilter{}, nil, false, DescribeStacksErrorOptions{},
	)
	require.NoError(t, err)
	var nodes *schema.Affected
	for i := range affected {
		if affected[i].Component == "k8s_nodes" {
			nodes = &affected[i]
			break
		}
	}
	require.NotNil(t, nodes, "the module change must affect the component inheriting its directory")
	assert.Equal(t, "test", nodes.Stack)
	assert.Contains(t, nodes.AffectedAll, "component")
	assert.Equal(t, "components/terraform/lxd/talos-nodes", filepath.ToSlash(nodes.ComponentPath))
}
