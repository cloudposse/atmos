package secret

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// overrideKeygenConfig swaps the loadKeygenConfig seam for the test and restores it via Cleanup.
func overrideKeygenConfig(t *testing.T, atmosConfig *schema.AtmosConfiguration, err error) {
	t.Helper()

	orig := loadKeygenConfig
	loadKeygenConfig = func() (schema.AtmosConfiguration, error) { return *atmosConfig, err }
	t.Cleanup(func() { loadKeygenConfig = orig })
}

// keygenCommand builds a minimal cobra command carrying the --force flag runSecretKeygen reads.
func keygenCommand(t *testing.T, force bool) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "keygen"}
	cmd.Flags().Bool("force", force, "")
	return cmd
}

// ageVaultConfig returns a config with a single sops/age vault that writes its private identity to
// keyFile (returned) and its recipient under BasePath/.sops.yaml. No recipients/inline key are
// pinned, so the vault is eligible for in-process key generation.
func ageVaultConfig(t *testing.T) (schema.AtmosConfiguration, string) {
	t.Helper()

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: dir,
		Secrets: schema.SecretsConfig{Providers: map[string]schema.SecretProviderConfig{
			"dev-sops": {Kind: "sops/age", Spec: map[string]any{
				"file":         filepath.Join(dir, "secrets.enc.yaml"),
				"age_key_file": keyFile,
			}},
		}},
	}
	return atmosConfig, keyFile
}

func writeAgeIdentity(t *testing.T, keyFile string) {
	t.Helper()

	id, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyFile, []byte(id.String()+"\n"), 0o600))
}

func TestRunSecretKeygen_GeneratesKey(t *testing.T) {
	setupIO(t)
	atmosConfig, keyFile := ageVaultConfig(t)
	overrideKeygenConfig(t, &atmosConfig, nil)

	require.NoError(t, runSecretKeygen(keygenCommand(t, false), nil))

	data, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "AGE-SECRET-KEY", "private identity must be written to the key file")
}

func TestRunSecretKeygen_AlreadyHasKeyNoForce(t *testing.T) {
	setupIO(t)
	atmosConfig, keyFile := ageVaultConfig(t)
	writeAgeIdentity(t, keyFile)
	before, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	overrideKeygenConfig(t, &atmosConfig, nil)

	require.NoError(t, runSecretKeygen(keygenCommand(t, false), nil))

	after, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.Equal(t, before, after, "without --force an existing key must not be modified")
}

func TestRunSecretKeygen_ForceRegenerates(t *testing.T) {
	setupIO(t)
	atmosConfig, keyFile := ageVaultConfig(t)
	writeAgeIdentity(t, keyFile)
	before, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	overrideKeygenConfig(t, &atmosConfig, nil)

	require.NoError(t, runSecretKeygen(keygenCommand(t, true), nil))

	after, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.NotEqual(t, before, after, "--force must append newly generated material")
	assert.Contains(t, string(after), "AGE-SECRET-KEY")
}

func TestRunSecretKeygen_InitConfigError(t *testing.T) {
	setupIO(t)
	overrideKeygenConfig(t, &schema.AtmosConfiguration{}, errors.New("config boom"))

	err := runSecretKeygen(keygenCommand(t, false), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestRunSecretKeygen_NoVaults(t *testing.T) {
	setupIO(t)
	overrideKeygenConfig(t, &schema.AtmosConfiguration{}, nil)

	err := runSecretKeygen(keygenCommand(t, false), nil)
	require.ErrorIs(t, err, ErrNoVault)
}

func TestRunSecretKeygen_UnsupportedKind(t *testing.T) {
	setupIO(t)
	atmosConfig := schema.AtmosConfiguration{
		Secrets: schema.SecretsConfig{Providers: map[string]schema.SecretProviderConfig{
			"weird": {Kind: "bogus/thing"},
		}},
	}
	overrideKeygenConfig(t, &atmosConfig, nil)

	// An unregistered backend kind is reported as "not implemented" (friendly), not a hard error.
	assert.NoError(t, runSecretKeygen(keygenCommand(t, false), nil))
}
