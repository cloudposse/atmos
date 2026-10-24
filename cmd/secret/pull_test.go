package secret

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestRunSecretPull_EnvStdout(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "A"}, {Name: "B"}}
	svc.getValues["A"] = "1"
	svc.getValues["B"] = "2"
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "pull", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.getCalls, 2)
}

func TestRunSecretPull_JSONStdout(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "A"}}
	svc.getValues["A"] = "1"
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "pull", "--format", "json", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
}

func TestRunSecretPull_FileWrite(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "A"}, {Name: "B"}}
	svc.getValues["A"] = "1"
	svc.getValues["B"] = "2"
	installService(t, svc, nil)

	outPath := filepath.Join(t.TempDir(), "out.env")
	err := runSecretSubcommand(t, "pull", "--output", outPath, "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	content, readErr := os.ReadFile(outPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "A=1")
	assert.Contains(t, string(content), "B=2")
}

func TestRunSecretPull_SkipsOnGetError(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "A"}, {Name: "B"}}
	svc.getValues["A"] = "1"
	svc.getErrs["B"] = errors.New("not initialized")
	installService(t, svc, nil)

	outPath := filepath.Join(t.TempDir(), "out.env")
	err := runSecretSubcommand(t, "pull", "--output", outPath, "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	content, readErr := os.ReadFile(outPath)
	require.NoError(t, readErr)
	// The errored secret is skipped; only A is written.
	assert.Contains(t, string(content), "A=1")
	assert.NotContains(t, string(content), "B=")
}

func TestRunSecretPull_UnsupportedFormat(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "A"}}
	svc.getValues["A"] = "1"
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "pull", "--format", "xml", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, ErrUnsupportedFormat)
}

func TestRunSecretPull_LoadServiceError(t *testing.T) {
	setupIO(t)
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "pull", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
