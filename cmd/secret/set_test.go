package secret

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
	t.Cleanup(func() { os.Stdin = orig })

	// A trailing newline must be trimmed.
	_, writeErr := w.WriteString("piped-secret\n")
	require.NoError(t, writeErr)
	require.NoError(t, w.Close())

	got, err := resolveSetValue("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "piped-secret", got)
}

func TestRunSecretSet_Inline(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "v1", svc.setCalls[0].value)
}

func TestRunSecretSet_Prompt(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	overridePromptForValue(t, "prompted-secret", nil)

	// No inline value and no --stdin → the prompt path is used.
	err := runSecretSubcommand(t, "set", "API_KEY", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "prompted-secret", svc.setCalls[0].value)
}

func TestRunSecretSet_PromptError(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	sentinel := errors.New("prompt aborted")
	overridePromptForValue(t, "", sentinel)

	err := runSecretSubcommand(t, "set", "API_KEY", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, sentinel)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_EmptyName(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// "=v1" cuts to an empty name.
	err := runSecretSubcommand(t, "set", "=v1", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_SetError(t *testing.T) {
	svc := newFakeSecretService()
	svc.setErr = errors.New("backend write failed")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.setErr)
	require.Len(t, svc.setCalls, 1)
}

func TestRunSecretSet_MissingScope(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// No --stack/--component → parseScope rejects before loading the service.
	err := runSecretSubcommand(t, "set", "API_KEY=v1")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_LoadServiceError(t *testing.T) {
	loadErr := errors.New("failed to load service")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
