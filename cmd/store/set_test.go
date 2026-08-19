package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSetValue_Inline(t *testing.T) {
	got, err := resolveSetValue("inline-val", true, false)
	require.NoError(t, err)
	assert.Equal(t, "inline-val", got)
}

func TestResolveSetValue_Prompt(t *testing.T) {
	overridePromptForValue(t, "from-prompt", nil)
	got, err := resolveSetValue("", false, false)
	require.NoError(t, err)
	assert.Equal(t, "from-prompt", got)
}

func TestResolveSetValue_Stdin(t *testing.T) {
	// Replace os.Stdin with a pipe so resolveSetValue reads the value we write (cross-platform).
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		_ = r.Close()
		os.Stdin = orig
	})

	// A trailing newline must be trimmed.
	_, writeErr := w.WriteString("piped-value\n")
	require.NoError(t, writeErr)
	require.NoError(t, w.Close())

	got, err := resolveSetValue("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "piped-value", got)
}

func TestRunStoreSet_Inline(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "set", "app-metadata", "image_tag", "v1.2.3", "--stack", "dev", "--component", "vpc")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "app-metadata", svc.setCalls[0].name)
	assert.Equal(t, "dev", svc.setCalls[0].stack)
	assert.Equal(t, "vpc", svc.setCalls[0].component)
	assert.Equal(t, "image_tag", svc.setCalls[0].key)
	assert.Equal(t, "v1.2.3", svc.setCalls[0].value)
}

func TestRunStoreSet_NoScope(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "set", "app-metadata", "global_key", "hello")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Empty(t, svc.setCalls[0].stack)
	assert.Empty(t, svc.setCalls[0].component)
}

func TestRunStoreSet_ServiceError(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	svc.setErr = assert.AnError
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "set", "app-metadata", "key", "value")
	require.ErrorIs(t, err, assert.AnError)
}
