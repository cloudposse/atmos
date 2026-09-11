package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/templating"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestFuncMap(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	reader := templating.NewMockDatasourceReader(gomock.NewController(t))

	fm := FuncMap(atmosConfig, configAndStacksInfo, ctx, reader)

	// Verify the function map contains expected keys: "atmos" plus every
	// function templatefuncs.FuncMap() registers (e.g. "collectKeys").
	keys := u.StringKeysFromMap(fm)
	assert.ElementsMatch(t, []string{"atmos", "collectKeys"}, keys)

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
	assert.Equal(t, reader, atmosFuncs.datasources)
}

func TestAtmosFuncs_Component(t *testing.T) {
	atmosFuncs := &AtmosFuncs{
		atmosConfig:         &schema.AtmosConfiguration{},
		configAndStacksInfo: &schema.ConfigAndStacksInfo{},
		ctx:                 context.TODO(),
	}

	// Test with empty parameters - should return error.
	_, err := atmosFuncs.Component("", "")
	assert.Error(t, err, "Component() should return error for empty parameters")
}

func TestAtmosFuncs_GomplateDatasource(t *testing.T) {
	t.Run("delegates alias and args to the datasource reader", func(t *testing.T) {
		reader := templating.NewMockDatasourceReader(gomock.NewController(t))
		reader.EXPECT().
			Datasource("config", "sub", "path").
			Return(map[string]any{"name": "atmos"}, nil)

		atmosFuncs := &AtmosFuncs{
			atmosConfig:         &schema.AtmosConfiguration{},
			configAndStacksInfo: &schema.ConfigAndStacksInfo{},
			ctx:                 context.TODO(),
			datasources:         reader,
		}

		result, err := atmosFuncs.GomplateDatasource("config", "sub", "path")
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"name": "atmos"}, result)
	})

	t.Run("propagates reader errors", func(t *testing.T) {
		readErr := errors.New("boom")
		reader := templating.NewMockDatasourceReader(gomock.NewController(t))
		reader.EXPECT().Datasource("nonexistent-alias").Return(nil, readErr)

		atmosFuncs := &AtmosFuncs{
			atmosConfig: &schema.AtmosConfiguration{},
			ctx:         context.TODO(),
			datasources: reader,
		}

		_, err := atmosFuncs.GomplateDatasource("nonexistent-alias")
		require.ErrorIs(t, err, readErr)
	})

	t.Run("nil reader is unavailable", func(t *testing.T) {
		atmosFuncs := &AtmosFuncs{
			atmosConfig: &schema.AtmosConfiguration{},
			ctx:         context.TODO(),
		}

		_, err := atmosFuncs.GomplateDatasource("config")
		require.ErrorIs(t, err, errUtils.ErrGomplateDatasourceUnavailable)
	})
}

func TestAtmosFuncs_Resolve(t *testing.T) {
	atmosFuncs := &AtmosFuncs{
		atmosConfig:         &schema.AtmosConfiguration{},
		configAndStacksInfo: &schema.ConfigAndStacksInfo{},
		ctx:                 context.TODO(),
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
			atmosConfig: &schema.AtmosConfiguration{},
			ctx:         context.TODO(),
		}
		result, err := funcs.Resolve("plain")
		require.NoError(t, err)
		assert.Equal(t, "plain", result)
	})
}
