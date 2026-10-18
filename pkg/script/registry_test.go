package script

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_engine_test.go -package script github.com/cloudposse/atmos/pkg/script Engine

func TestRegistryOwnershipAndReplacement(t *testing.T) {
	t.Parallel()
	r := newRegistry()
	first := NewMockEngine(gomock.NewController(t))
	second := NewMockEngine(gomock.NewController(t))
	extensions := []string{".star"}
	option := WithExtensions(extensions...)
	extensions[0] = ".changed"
	require.NoError(t, r.register(" starlark ", first, option, WithAtmosShebang()))
	require.NoError(t, r.register("starlark", second))
	assert.Same(t, second, r.engines["starlark"])
	assert.Equal(t, "starlark", r.extensions[".star"])
	assert.NotContains(t, r.extensions, ".changed")
	assert.Equal(t, "starlark", r.shebang)
	// Failed registrations must not install an engine or partially claim extensions.
	require.ErrorIs(t, r.register("other", first, WithExtensions(".other", ".star")), errUtils.ErrScriptRegistration)
	assert.NotContains(t, r.engines, "other")
	assert.NotContains(t, r.extensions, ".other")
	require.ErrorIs(t, r.register("other", first, WithExtensions(".other"), WithAtmosShebang()), errUtils.ErrScriptRegistration)
	assert.NotContains(t, r.extensions, ".other")
	require.NoError(t, r.register("other", first, WithExtensions(".other", ".OTHER")))
	assert.Equal(t, "other", r.extensions[".other"])
	assert.Equal(t, "starlark", r.shebang)
}

func TestRegistryRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	engine := NewMockEngine(gomock.NewController(t))
	for _, extension := range []string{"", ".", "star", ".tar.gz", ".a/b", ".a\\b", ".a b", ".a\tb"} {
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			r := newRegistry()
			require.ErrorIs(t, r.register("test", engine, WithExtensions(".valid", extension)), errUtils.ErrScriptRegistration)
			assert.Empty(t, r.engines)
			assert.Empty(t, r.extensions)
		})
	}
	require.ErrorIs(t, newRegistry().register(" ", engine), errUtils.ErrScriptRegistration)
	require.ErrorIs(t, newRegistry().register("test", nil), errUtils.ErrScriptRegistration)
	assert.Panics(t, func() { Register("", engine) })
}
