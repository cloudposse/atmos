package secret

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSecretImport_DeclaredAndSkipped(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true} // B undeclared → warned and skipped.
	installService(t, svc, nil)

	path := writeEnvFile(t, "A=1\nB=2\n")
	err := runSecretSubcommand(t, "import", path, "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "A", svc.setCalls[0].name)
	assert.Equal(t, "1", svc.setCalls[0].value)
}

func TestRunSecretImport_DryRun(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true, "B": true}
	installService(t, svc, nil)

	path := writeEnvFile(t, "A=1\nB=2\n")
	err := runSecretSubcommand(t, "import", path, "--dry-run", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// Dry-run previews but never writes.
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretImport_SetError(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"A": true}
	svc.setErr = errors.New("backend write failed")
	installService(t, svc, nil)

	path := writeEnvFile(t, "A=1\n")
	err := runSecretSubcommand(t, "import", path, "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.setErr)
}

func TestRunSecretImport_MissingFile(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	missingPath := filepath.Join(t.TempDir(), "does-not-exist.env")
	err := runSecretSubcommand(t, "import", missingPath, "--stack", "dev", "--component", "api")
	require.Error(t, err)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretImport_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	path := writeEnvFile(t, "A=1\n")
	err := runSecretSubcommand(t, "import", path, "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
