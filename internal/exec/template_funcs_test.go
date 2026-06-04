package exec

import (
	"context"
	"testing"

	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestFuncMap(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	gomplateData := &data.Data{}

	fm := FuncMap(atmosConfig, configAndStacksInfo, ctx, gomplateData)

	// Verify the function map contains expected keys.
	keys := u.StringKeysFromMap(fm)
	assert.Equal(t, 1, len(keys), "FuncMap should return exactly one key")
	assert.Equal(t, "atmos", keys[0], "FuncMap should return 'atmos' key")

	// Verify the atmos function is callable and returns AtmosFuncs.
	atmosFunc, ok := fm["atmos"]
	assert.True(t, ok, "FuncMap should contain 'atmos' key")
	assert.NotNil(t, atmosFunc, "atmos function should not be nil")

	// Call the function to verify it returns AtmosFuncs instance.
	atmosFuncsInterface := atmosFunc.(func() any)()
	atmosFuncs, ok := atmosFuncsInterface.(*AtmosFuncs)
	assert.True(t, ok, "atmos function should return *AtmosFuncs")
	assert.NotNil(t, atmosFuncs, "AtmosFuncs should not be nil")

	// Verify AtmosFuncs has the expected configuration.
	assert.Equal(t, atmosConfig, atmosFuncs.atmosConfig)
	assert.Equal(t, configAndStacksInfo, atmosFuncs.configAndStacksInfo)
}

func TestAtmosFuncs_Component(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	gomplateData := &data.Data{}

	atmosFuncs := &AtmosFuncs{
		atmosConfig:         atmosConfig,
		configAndStacksInfo: configAndStacksInfo,
		ctx:                 ctx,
		gomplateData:        gomplateData,
	}

	// Test with empty parameters - should return error.
	_, err := atmosFuncs.Component("", "")
	assert.Error(t, err, "Component() should return error for empty parameters")
}

func TestAtmosFuncs_GomplateDatasource(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	gomplateData := &data.Data{}

	atmosFuncs := &AtmosFuncs{
		atmosConfig:         atmosConfig,
		configAndStacksInfo: configAndStacksInfo,
		ctx:                 ctx,
		gomplateData:        gomplateData,
	}

	// Test with invalid alias - should return error.
	_, err := atmosFuncs.GomplateDatasource("nonexistent-alias")
	assert.Error(t, err, "GomplateDatasource() should return error for invalid alias")
}

func TestAtmosFuncs_Resolve(t *testing.T) {
	atmosFuncs := &AtmosFuncs{
		atmosConfig:         &schema.AtmosConfiguration{},
		configAndStacksInfo: &schema.ConfigAndStacksInfo{},
		ctx:                 context.TODO(),
		gomplateData:        &data.Data{},
	}

	t.Run("plain untagged string is returned unchanged", func(t *testing.T) {
		result, err := atmosFuncs.Resolve("just-a-string")
		require.NoError(t, err)
		assert.Equal(t, "just-a-string", result)
	})

	t.Run("resolves an !env YAML function at template time", func(t *testing.T) {
		t.Setenv("ATMOS_RESOLVE_TEST_VAR", "resolved-value")
		result, err := atmosFuncs.Resolve("!env ATMOS_RESOLVE_TEST_VAR")
		require.NoError(t, err)
		assert.Equal(t, "resolved-value", result)
	})

	t.Run("does not panic with nil configAndStacksInfo", func(t *testing.T) {
		funcs := &AtmosFuncs{
			atmosConfig:  &schema.AtmosConfiguration{},
			ctx:          context.TODO(),
			gomplateData: &data.Data{},
		}
		result, err := funcs.Resolve("plain")
		require.NoError(t, err)
		assert.Equal(t, "plain", result)
	})
}
