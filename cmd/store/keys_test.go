package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStoreKeys(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	svc.keys = []string{"region", "image_tag"}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "keys", "app-metadata", "--stack", "dev", "--component", "vpc")
	require.NoError(t, err)

	require.Len(t, svc.keysCalls, 1)
	assert.Equal(t, "app-metadata", svc.keysCalls[0].name)
	assert.Equal(t, "dev", svc.keysCalls[0].stack)
	assert.Equal(t, "vpc", svc.keysCalls[0].component)
	assert.Equal(t, "image_tag\nregion\n", stdout.String())
}

func TestRunStoreKeys_JSON(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	svc.keys = []string{"region", "image_tag"}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "keys", "app-metadata", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, `["image_tag","region"]`+"\n", stdout.String())
}

func TestRunStoreKeys_Empty(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "keys", "app-metadata")
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}

func TestRunStoreKeys_NotSupported(t *testing.T) {
	captureStdout(t)
	svc := newFakeStoreService()
	svc.keysErr = assert.AnError
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "keys", "app-metadata")
	require.ErrorIs(t, err, assert.AnError)
}
