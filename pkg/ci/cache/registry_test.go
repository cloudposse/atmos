package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRegisterAndNewBackend(t *testing.T) {
	const typeName = "test-registry-backend"
	fake := newFakeBackend()
	Register(typeName, func(_ Options) (Backend, error) { return fake, nil })

	got, err := NewBackend(typeName, Options{})
	require.NoError(t, err)
	assert.Same(t, fake, got)

	assert.Contains(t, GetRegisteredTypes(), typeName)
}

func TestNewBackend_NotFound(t *testing.T) {
	_, err := NewBackend("does-not-exist", Options{})
	require.ErrorIs(t, err, errUtils.ErrCacheBackendNotFound)
}

func TestRegister_PanicsOnEmptyType(t *testing.T) {
	assert.Panics(t, func() {
		Register("", func(_ Options) (Backend, error) { return nil, nil })
	})
}

func TestRegister_PanicsOnNilFactory(t *testing.T) {
	assert.Panics(t, func() {
		Register("nil-factory", nil)
	})
}
