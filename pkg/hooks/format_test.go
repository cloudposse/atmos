package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// noopHandler is a stand-in ResultHandler used to assert registry wiring without
// parsing anything.
func noopHandler(*ExecContext) (*Summary, error) { return nil, nil }

// RegisterFormatHandler rejects empty formats, nil handlers, and duplicates, and
// otherwise records the handler for later lookup.
func TestRegisterFormatHandler(t *testing.T) {
	require.ErrorIs(t, RegisterFormatHandler("", noopHandler), errUtils.ErrNilParam)
	require.ErrorIs(t, RegisterFormatHandler("anything", nil), errUtils.ErrNilParam)

	// First registration of a unique format succeeds; re-registering is rejected.
	const format = "test-format-register"
	require.NoError(t, RegisterFormatHandler(format, noopHandler))
	require.ErrorIs(t, RegisterFormatHandler(format, noopHandler), errUtils.ErrInvalidConfig)
}

// formatHandlerFor returns the registered handler for a known format and reports
// absence for an unknown one.
func TestFormatHandlerFor(t *testing.T) {
	const format = "test-format-lookup"
	require.NoError(t, RegisterFormatHandler(format, noopHandler))

	h, ok := formatHandlerFor(format)
	assert.True(t, ok)
	assert.NotNil(t, h)

	h, ok = formatHandlerFor("test-format-absent")
	assert.False(t, ok)
	assert.Nil(t, h)
}

// resolveResultHandler prefers the kind's own handler, falls back to a
// format handler matching the hook's format, and returns nil when neither
// applies (including nil ctx/kind).
func TestResolveResultHandler(t *testing.T) {
	const format = "test-format-resolve"
	require.NoError(t, RegisterFormatHandler(format, noopHandler))

	assert.Nil(t, resolveResultHandler(nil), "nil ctx")
	assert.Nil(t, resolveResultHandler(&ExecContext{}), "nil kind")

	// Kind's own handler wins.
	kindHandler := func(*ExecContext) (*Summary, error) { return &Summary{Kind: "own"}, nil }
	ctx := &ExecContext{Kind: &Kind{ResultHandler: kindHandler}}
	got := resolveResultHandler(ctx)
	require.NotNil(t, got)
	s, _ := got(nil)
	require.NotNil(t, s)
	assert.Equal(t, "own", s.Kind, "kind's own handler takes precedence")

	// No kind handler, but the hook declares a registered format.
	ctx = &ExecContext{Kind: &Kind{}, Hook: &Hook{Format: format}}
	assert.NotNil(t, resolveResultHandler(ctx), "format handler resolves")

	// Unknown format → nil.
	ctx = &ExecContext{Kind: &Kind{}, Hook: &Hook{Format: "test-format-unknown"}}
	assert.Nil(t, resolveResultHandler(ctx))
}
