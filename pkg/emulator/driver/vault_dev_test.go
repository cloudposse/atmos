package driver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

// TestVaultDrivers_FileBackendDefaults pins the file-backend container contract for
// the openbao and vault drivers: both run a non-dev `server` whose config comes from
// the inline *_LOCAL_CONFIG env (a file storage backend at the persistence data dir
// plus an all-interfaces TLS-disabled listener). The file backend is what lets the
// manager persist secrets; the manager initializes/unseals it after start.
func TestVaultDrivers_FileBackendDefaults(t *testing.T) {
	for _, name := range []string{"openbao", "vault"} {
		t.Run(name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)

			defaults := d.Defaults()
			assert.Equal(t, []string{"server"}, defaults.Command, "must run a non-dev server")
			require.Len(t, defaults.Ports, 1)
			assert.Equal(t, vaultPort, defaults.Ports[0])
			assert.Equal(t, vaultDataDir, defaults.DataDir, "data dir must be the file-storage path for persistence")

			// The inline config (under both OpenBao and Vault env names) must select the
			// file backend at the data dir and bind all interfaces.
			for _, key := range []string{"BAO_LOCAL_CONFIG", "VAULT_LOCAL_CONFIG"} {
				cfg := defaults.Env[key]
				assert.Contains(t, cfg, `storage "file"`, "%s must use the file backend", key)
				assert.Contains(t, cfg, vaultDataDir, "%s must store at the data dir", key)
				assert.Contains(t, cfg, "0.0.0.0:8200", "%s must bind all interfaces", key)
			}
		})
	}
}

// TestVaultDrivers_DistinctImages guards that openbao and vault remain distinct images
// on the same Vault target — a swap would silently run the wrong server.
func TestVaultDrivers_DistinctImages(t *testing.T) {
	openbao, err := emu.ResolveDriver("openbao")
	require.NoError(t, err)
	vault, err := emu.ResolveDriver("vault")
	require.NoError(t, err)

	assert.Equal(t, openbaoImage, openbao.Defaults().Image)
	assert.Equal(t, vaultImage, vault.Defaults().Image)
	assert.NotEqual(t, openbao.Defaults().Image, vault.Defaults().Image, "openbao and vault must use distinct images")
}
