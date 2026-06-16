package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSecretDelete_ConfirmYes(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	titles := overrideConfirmAction(t, true, nil)

	err := runSecretSubcommand(t, "delete", "API_KEY", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.deletedNames, 1)
	assert.Equal(t, "API_KEY", svc.deletedNames[0])
	require.Len(t, *titles, 1)
	assert.Contains(t, (*titles)[0], "API_KEY")
}

func TestRunSecretDelete_ConfirmNo(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	overrideConfirmAction(t, false, nil)

	err := runSecretSubcommand(t, "delete", "API_KEY", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// Declining the confirmation must not delete anything.
	assert.Empty(t, svc.deletedNames)
}

func TestRunSecretDelete_ConfirmError(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	sentinel := errors.New("confirm prompt failed")
	overrideConfirmAction(t, false, sentinel)

	err := runSecretSubcommand(t, "delete", "API_KEY", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, sentinel)
	assert.Empty(t, svc.deletedNames)
}

func TestRunSecretDelete_ForceSkipsConfirm(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	// confirmActionFn must NOT be called when --force is given; make it fail loudly if it is.
	titles := overrideConfirmAction(t, false, errors.New("should not be called"))

	err := runSecretSubcommand(t, "delete", "API_KEY", "--force", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.deletedNames, 1)
	assert.Equal(t, "API_KEY", svc.deletedNames[0])
	assert.Empty(t, *titles)
}

func TestRunSecretDelete_DeleteError(t *testing.T) {
	svc := newFakeSecretService()
	svc.deleteErr = errors.New("backend delete failed")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "delete", "API_KEY", "--force", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.deleteErr)
}

func TestRunSecretDelete_MissingName(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// No NAME and no --all.
	err := runSecretSubcommand(t, "delete", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, ErrSecretNameRequired)
	assert.Empty(t, svc.deletedNames)
}

func TestRunSecretDeleteAll_ConfirmYes(t *testing.T) {
	svc := newFakeSecretService()
	svc.deleteAllCount = 3
	installService(t, svc, nil)
	titles := overrideConfirmAction(t, true, nil)

	err := runSecretSubcommand(t, "delete", "--all", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	assert.Equal(t, 1, svc.deleteAllCalls)
	require.Len(t, *titles, 1)
	assert.Contains(t, (*titles)[0], "ALL")
}

func TestRunSecretDeleteAll_ConfirmNo(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	overrideConfirmAction(t, false, nil)

	err := runSecretSubcommand(t, "delete", "--all", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
	assert.Zero(t, svc.deleteAllCalls)
}

func TestRunSecretDeleteAll_ForceAndError(t *testing.T) {
	svc := newFakeSecretService()
	svc.deleteAllErr = errors.New("delete all failed")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "delete", "--all", "--force", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.deleteAllErr)
	assert.Equal(t, 1, svc.deleteAllCalls)
}

func TestRunSecretDeleteAll_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "delete", "--all", "--force", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
