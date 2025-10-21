package exec

import (
	"context"
	"testing"

	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/stretchr/testify/assert"

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
