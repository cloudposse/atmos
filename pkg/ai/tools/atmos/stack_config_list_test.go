package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewStackConfigListTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewStackConfigListTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestStackConfigListTool_Name(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_stack_config_list", tool.Name())
}

func TestStackConfigListTool_Description(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestStackConfigListTool_Parameters(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 3)
	assert.Equal(t, paramStack, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramComponent, params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "pattern", params[2].Name)
	assert.False(t, params[2].Required)
}

func TestStackConfigListTool_RequiresPermission(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestStackConfigListTool_IsRestricted(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestStackConfigListTool_Execute_MissingStack(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramComponent: "vpc",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

func TestStackConfigListTool_Execute_MissingComponent(t *testing.T) {
	tool := NewStackConfigListTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack: "dev",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

func TestStackConfigListTool_Execute_ListsAllPaths(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	entries, ok := result.Data["entries"].([]map[string]interface{})
	require.True(t, ok)

	byPath := make(map[string]map[string]interface{}, len(entries))
	for _, e := range entries {
		byPath[e["path"].(string)] = e
	}

	// vars.foo is overridden by dev.yaml, so it must resolve there.
	require.Contains(t, byPath, "vars.foo")
	assert.Equal(t, "dev-override", byPath["vars.foo"]["value"])
	assert.Equal(t, "dev.yaml", byPath["vars.foo"]["file"])

	// vars.region is only ever declared in the catalog manifest with no
	// override. list (unlike get/set/delete) does not verify the path is
	// literally present in the resolved file -- it reports the last
	// provenance entry unconditionally, matching cmd/stack/config.go's
	// provenanceFileForComponentPath -- so it is still listed, attributed to
	// the importing manifest.
	require.Contains(t, byPath, "vars.region")
	assert.Equal(t, "us-east-1", byPath["vars.region"]["value"])
	assert.Equal(t, "dev.yaml", byPath["vars.region"]["file"])
}

func TestStackConfigListTool_Execute_PatternFilter(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"pattern":      "vars.f*",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	entries, ok := result.Data["entries"].([]map[string]interface{})
	require.True(t, ok)
	for _, e := range entries {
		assert.Regexp(t, "^vars\\.f", e["path"].(string))
	}
	assert.NotEmpty(t, entries)
}

func TestStackConfigListTool_Execute_InvalidStack(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "does-not-exist",
		paramComponent: "vpc",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

func TestStackConfigListPatternRegexp(t *testing.T) {
	re := stackConfigListPatternRegexp("vars.*")
	assert.True(t, re.MatchString("vars.region"))
	assert.False(t, re.MatchString("settings.enabled"))

	re = stackConfigListPatternRegexp("vars.re?ion")
	assert.True(t, re.MatchString("vars.region"))
	assert.False(t, re.MatchString("vars.reggion"))
}
