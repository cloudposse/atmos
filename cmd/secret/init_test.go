package secret

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

func TestRunSecretInit_PromptsMissing(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: false},
		{Declaration: secrets.Declaration{Name: "DB"}, Initialized: true}, // Already set → decline the update prompt.
	}
	installService(t, svc, nil)
	overridePromptForValue(t, "entered", nil)
	overrideConfirmAction(t, false, nil) // decline rotating the already-initialized secret.

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// The missing secret is set; the already-initialized one is skipped (update declined).
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "entered", svc.setCalls[0].value)
}

// TestRunSecretInit_RotateUpdate proves an already-initialized secret is rotated when the user
// accepts the update prompt (the manual-rotation flow).
func TestRunSecretInit_RotateUpdate(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "DB"}, Initialized: true},
	}
	installService(t, svc, nil)
	overridePromptForValue(t, "rotated", nil)
	overrideConfirmAction(t, true, nil) // accept the update (rotate).

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "DB", svc.setCalls[0].name)
	assert.Equal(t, "rotated", svc.setCalls[0].value)
}

func TestRunSecretInit_Force(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "A"}, Initialized: true},
		{Declaration: secrets.Declaration{Name: "B"}, Initialized: true},
	}
	installService(t, svc, nil)
	overridePromptForValue(t, "v", nil)

	err := runSecretSubcommand(t, "init", "--force", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// --force re-prompts and overwrites both already-initialized secrets.
	require.Len(t, svc.setCalls, 2)
	assert.Equal(t, "A", svc.setCalls[0].name)
	assert.Equal(t, "B", svc.setCalls[1].name)
}

func TestRunSecretInit_DryRun(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: false},
	}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "init", "--dry-run", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// Dry-run reports but never writes.
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretInit_InputFile(t *testing.T) {
	_, stderr := setupIOCapture(t)
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: false},
		{Declaration: secrets.Declaration{Name: "DB_CONFIG"}, Initialized: false},
	}
	svc.declared["API_KEY"] = true
	svc.declared["DB_CONFIG"] = true
	installService(t, svc, nil)

	path := filepath.Join(t.TempDir(), ".env.local")
	require.NoError(t, os.WriteFile(path, []byte("# local development\nAPI_KEY=from-file # comment\nUNDECLARED=ignored\n"), 0o600))

	err := runSecretSubcommand(t, "init", "--input", path, "--stack", "dev", "--component", "api")
	require.NoError(t, err)
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "from-file", svc.setCalls[0].value)
	assert.Contains(t, stderr.String(), "Skipping undeclared input key UNDECLARED")
}

func TestRunSecretInit_InputFileStrictRejectsUndeclaredKey(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: false}}
	svc.declared["API_KEY"] = true
	installService(t, svc, nil)

	path := filepath.Join(t.TempDir(), ".env.local")
	require.NoError(t, os.WriteFile(path, []byte("API_KEY=from-file\nTYPO=value\n"), 0o600))

	err := runSecretSubcommand(t, "init", "--input", path, "--mode", "strict", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrValidationFailed)
	assert.Empty(t, svc.setCalls, "strict validation must fail before writing any values")
}

func TestReadInitInput_RedirectedStdin(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	originalStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		_ = r.Close()
		os.Stdin = originalStdin
	})
	originalIsTTY := initStdinIsTTY
	initStdinIsTTY = func() bool { return false }
	t.Cleanup(func() { initStdinIsTTY = originalIsTTY })

	_, err = w.WriteString("# comment\nAPI_KEY=from-stdin\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	values, err := readInitInput("")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"API_KEY": "from-stdin"}, values)
}

func TestRunSecretInit_PromptError(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: false},
	}
	installService(t, svc, nil)
	sentinel := errors.New("prompt aborted")
	overridePromptForValue(t, "", sentinel)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, sentinel)
}

func TestRunSecretInit_OfferKeygen(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.missingVaults = []secrets.GenerableVault{{Track: "sops", Name: "dev-sops"}}
	svc.keygenResults["dev-sops"] = &providers.KeygenResult{
		Vault:   "dev-sops",
		Kind:    "sops/age",
		Summary: "Generated an age key pair.",
		Outputs: []providers.KeygenOutput{{Label: "private identity", Location: "keys.txt", Sensitive: true}},
	}
	installService(t, svc, nil)
	overrideConfirmAction(t, true, nil)
	overridePromptForValue(t, "v", nil)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.generatedVault, 1)
	assert.Equal(t, "dev-sops", svc.generatedVault[0].Name)
}

func TestRunSecretInit_OfferKeygenDeclined(t *testing.T) {
	svc := newFakeSecretService()
	svc.missingVaults = []secrets.GenerableVault{{Track: "sops", Name: "dev-sops"}}
	installService(t, svc, nil)
	overrideConfirmAction(t, false, nil)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// Declining keygen must not generate.
	assert.Empty(t, svc.generatedVault)
}

func TestRunSecretInit_OfferKeygenDryRun(t *testing.T) {
	svc := newFakeSecretService()
	svc.missingVaults = []secrets.GenerableVault{{Track: "sops", Name: "dev-sops"}}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "init", "--dry-run", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// Dry-run only reports; no keygen, no prompt.
	assert.Empty(t, svc.generatedVault)
}

func TestRunSecretInit_VaultsMissingError(t *testing.T) {
	svc := newFakeSecretService()
	svc.missingErr = errors.New("cannot inspect vaults")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.missingErr)
}

func TestRunSecretInit_KeygenError(t *testing.T) {
	svc := newFakeSecretService()
	svc.missingVaults = []secrets.GenerableVault{{Track: "sops", Name: "dev-sops"}}
	svc.keygenErr = errors.New("keygen failed")
	installService(t, svc, nil)
	overrideConfirmAction(t, true, nil)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.keygenErr)
}

func TestRunSecretInit_KeygenConfirmError(t *testing.T) {
	svc := newFakeSecretService()
	svc.missingVaults = []secrets.GenerableVault{{Track: "sops", Name: "dev-sops"}}
	installService(t, svc, nil)
	sentinel := errors.New("confirm failed")
	overrideConfirmAction(t, false, sentinel)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, sentinel)
}

func TestRunSecretInit_SetError(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: false},
	}
	svc.setErr = errors.New("backend write failed")
	installService(t, svc, nil)
	overridePromptForValue(t, "v", nil)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.setErr)
}

func TestRunSecretInit_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "init", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
