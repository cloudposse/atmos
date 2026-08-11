package io

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPushUIWriter_RedirectsUIStream verifies that a scoped override routes UI-stream
// writes to the provided sink and that restore reinstates the previous sink. The
// restore func must be idempotent.
func TestPushUIWriter_RedirectsUIStream(t *testing.T) {
	require.NoError(t, Initialize())
	ctx := globalContext
	require.NotNil(t, ctx)

	var buf bytes.Buffer
	restore := PushUIWriter(&buf)
	t.Cleanup(restore) // Guard against leaking the global override if a require below fails before restore().
	require.NotNil(t, uiWriterOverride(), "override must be active")

	require.NoError(t, ctx.Write(UIStream, "to-buffer"))
	assert.Equal(t, "to-buffer", buf.String(), "UI write must go to the override sink")

	restore()
	assert.Nil(t, uiWriterOverride(), "restore must clear the override")

	// Idempotent: a second restore is a no-op and must not panic or re-apply.
	restore()
	assert.Nil(t, uiWriterOverride())
}

// TestSuppressUI_DiscardsUIStream verifies SuppressUI discards UI writes for its
// duration and that restore reinstates the prior sink (here, a capturing override).
func TestSuppressUI_DiscardsUIStream(t *testing.T) {
	require.NoError(t, Initialize())
	ctx := globalContext
	require.NotNil(t, ctx)

	// Capture sink stands in for the live UI stream so we can observe what reaches it.
	var captured bytes.Buffer
	restoreCapture := PushUIWriter(&captured)
	defer restoreCapture()

	// SuppressUI stacks on top, routing UI output to io.Discard.
	restore := SuppressUI()
	require.NoError(t, ctx.Write(UIStream, "hidden"))
	restore()

	// After restore, the previous (capturing) sink is reinstated.
	require.NoError(t, ctx.Write(UIStream, "visible"))

	assert.NotContains(t, captured.String(), "hidden", "SuppressUI must discard UI writes")
	assert.Contains(t, captured.String(), "visible", "restore must reinstate the previous sink")
}
