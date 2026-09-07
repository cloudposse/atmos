package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStoreDelete_Force(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "delete", "app-metadata", "image_tag", "--force", "--stack", "dev", "--component", "vpc")
	require.NoError(t, err)

	require.Len(t, svc.deleteCalls, 1)
	assert.Equal(t, "app-metadata", svc.deleteCalls[0].name)
	assert.Equal(t, "dev", svc.deleteCalls[0].stack)
	assert.Equal(t, "vpc", svc.deleteCalls[0].component)
	assert.Equal(t, "image_tag", svc.deleteCalls[0].key)
}

func TestRunStoreDelete_ConfirmedPrompt(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)
	titles := overrideConfirmAction(t, true, nil)

	err := runStoreSubcommand(t, "delete", "app-metadata", "key")
	require.NoError(t, err)

	require.Len(t, svc.deleteCalls, 1)
	require.Len(t, *titles, 1)
}

func TestRunStoreDelete_AbortedPrompt(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)
	overrideConfirmAction(t, false, nil)

	err := runStoreSubcommand(t, "delete", "app-metadata", "key")
	require.NoError(t, err)
	assert.Empty(t, svc.deleteCalls)
}

func TestRunStoreDelete_ConfirmError(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)
	overrideConfirmAction(t, false, assert.AnError)

	err := runStoreSubcommand(t, "delete", "app-metadata", "key")
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, svc.deleteCalls)
}

func TestRunStoreDelete_NotSupported(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	svc.deleteErr = assert.AnError
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "delete", "app-metadata", "key", "--force")
	require.ErrorIs(t, err, assert.AnError)
}
