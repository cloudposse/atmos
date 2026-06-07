package secret

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// writeEnvFile writes a temp .env file with the given content and returns its path.
func writeEnvFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "secrets.env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestRunSecretPush_AllDeclared(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true, "B": true}
	installService(t, svc, nil)

	path := writeEnvFile(t, "A=1\nB=2\n")
	err := runSecretSubcommand(t, "push", "--input", path, "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 2)
	// sortedKeys gives deterministic order: A then B.
	assert.Equal(t, "A", svc.setCalls[0].name)
	assert.Equal(t, "1", svc.setCalls[0].value)
	assert.Equal(t, "B", svc.setCalls[1].name)
	assert.Equal(t, "2", svc.setCalls[1].value)
}

func TestRunSecretPush_UndeclaredFails(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true} // B is not declared.
	installService(t, svc, nil)

	path := writeEnvFile(t, "A=1\nB=2\n")
	err := runSecretSubcommand(t, "push", "--input", path, "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrValidationFailed)
	// Fail-fast: nothing written when any key is undeclared.
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretPush_SetError(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true}
	svc.setErr = errors.New("backend write failed")
	installService(t, svc, nil)

	path := writeEnvFile(t, "A=1\n")
	err := runSecretSubcommand(t, "push", "--input", path, "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.setErr)
}

func TestRunSecretPush_JSONFormat(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true}
	installService(t, svc, nil)

	path := filepath.Join(t.TempDir(), "s.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"A":"1"}`), 0o600))
	err := runSecretSubcommand(t, "push", "--input", path, "--format", "json", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "A", svc.setCalls[0].name)
}

func TestRunSecretPush_MissingFile(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	path := filepath.Join(t.TempDir(), "does-not-exist.env")
	err := runSecretSubcommand(t, "push", "--input", path, "--stack", "dev", "--component", "api")
	require.Error(t, err)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretPush_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	path := writeEnvFile(t, "A=1\n")
	err := runSecretSubcommand(t, "push", "--input", path, "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
