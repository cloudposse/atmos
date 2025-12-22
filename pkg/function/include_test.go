package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncludeFunction_Execute(t *testing.T) {
	fn := NewIncludeFunction()

	// Include function always returns ErrSpecialYAMLHandling.
	result, err := fn.Execute(context.Background(), "path/to/file.yaml", nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrSpecialYAMLHandling)
	assert.Contains(t, err.Error(), "include")
}

func TestIncludeRawFunction_Execute(t *testing.T) {
	fn := NewIncludeRawFunction()

	// Include.raw function always returns ErrSpecialYAMLHandling.
	result, err := fn.Execute(context.Background(), "path/to/file.txt", nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrSpecialYAMLHandling)
	assert.Contains(t, err.Error(), "include.raw")
}

func TestNewIncludeFunction(t *testing.T) {
	fn := NewIncludeFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagInclude, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestNewIncludeRawFunction(t *testing.T) {
	fn := NewIncludeRawFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagIncludeRaw, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}
