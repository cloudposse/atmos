package secret

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
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

// TestRunSecretImport_FromStoreMode proves any --from-* flag flips the positional argument from
// FILE to NAME and the parsed source reaches the service verbatim (including defaults left empty
// for the service to resolve).
func TestRunSecretImport_FromStoreMode(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"SHARED_CLIENT_SECRET": true}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "import", "SHARED_CLIENT_SECRET",
		"--from-stack", "atmos", "--from-component", "shared",
		"--stack", "prod", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.importCalls, 1)
	call := svc.importCalls[0]
	assert.Equal(t, "SHARED_CLIENT_SECRET", call.name)
	assert.Equal(t, secrets.ImportSource{Stack: "atmos", Component: "shared"}, call.src)
	assert.False(t, call.dryRun)
	assert.Empty(t, svc.setCalls, "from-store mode must not run the file-import path")
}

// TestRunSecretImport_FromStoreMode_AllFlags proves explicit --from-store/--from-key pass through.
func TestRunSecretImport_FromStoreMode_AllFlags(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"SHARED_CLIENT_SECRET": true}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "import", "SHARED_CLIENT_SECRET",
		"--from-store", "legacy", "--from-stack", "atmos", "--from-key", "client_secret",
		"--stack", "prod", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.importCalls, 1)
	assert.Equal(t, secrets.ImportSource{Store: "legacy", Stack: "atmos", Key: "client_secret"}, svc.importCalls[0].src)
}

// TestRunSecretImport_FromStoreMode_DryRun proves dry-run is forwarded to the service.
func TestRunSecretImport_FromStoreMode_DryRun(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"SHARED_CLIENT_SECRET": true}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "import", "SHARED_CLIENT_SECRET",
		"--from-stack", "atmos", "--dry-run",
		"--stack", "prod", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.importCalls, 1)
	assert.True(t, svc.importCalls[0].dryRun)
}

// TestRunSecretImport_FromStoreMode_FormatConflict proves --format (a FILE-mode flag) is rejected
// in store-coordinate mode.
func TestRunSecretImport_FromStoreMode_FormatConflict(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "import", "SHARED_CLIENT_SECRET",
		"--from-stack", "atmos", "--format", "json",
		"--stack", "prod", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
	assert.Empty(t, svc.importCalls)
}

// TestRunSecretImport_FromStoreMode_ServiceError proves a service-side import failure propagates.
func TestRunSecretImport_FromStoreMode_ServiceError(t *testing.T) {
	svc := newFakeSecretService()
	svc.importErr = errors.New("source unreadable")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "import", "SHARED_CLIENT_SECRET",
		"--from-stack", "atmos",
		"--stack", "prod", "--component", "api")
	require.ErrorIs(t, err, svc.importErr)
}
