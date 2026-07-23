package driver

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
)

// OpenBao and HashiCorp Vault are API-compatible secret servers (same VAULT_ADDR /
// VAULT_TOKEN). OpenBao (MPL, the open-source fork) is the default; Vault (BSL) is
// opt-in. Both run a file-storage server so secrets persist across `down`/`up`; the
// manager initializes, unseals, and enables KV v2 after the container starts (see
// pkg/emulator/vault.go), since a file-backed server boots sealed and uninitialized.
const (
	openbaoImage = "openbao/openbao:latest"
	vaultImage   = "hashicorp/vault:latest"
	vaultPort    = 8200
	// The vaultDataDir is the in-container file-storage path, bind-mounted from the
	// XDG cache for persistence (kept in sync with pkg/emulator.vaultDataDir).
	vaultDataDir = "/openbao/file"
)

// vaultServerCommand starts a non-dev server; the image entrypoint composes its
// config from BAO_LOCAL_CONFIG / VAULT_LOCAL_CONFIG (see vaultLocalConfig).
var vaultServerCommand = []string{"server"}

// vaultLocalConfig is the inline server config the image entrypoint materializes:
// a file storage backend (so state persists) and an all-interfaces TLS-disabled
// listener (reachable through the published host port). Memory locking is disabled
// because the file backend does not require it and it avoids a CAP_IPC_LOCK need.
const vaultLocalConfig = `storage "file" {
  path = "` + vaultDataDir + `"
}
listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = "true"
}
disable_mlock = true`

// vaultEnv supplies the inline config under both the OpenBao and Vault env var
// names, since one command/config serves both images.
func vaultEnv() map[string]string {
	return map[string]string{
		"BAO_LOCAL_CONFIG":   vaultLocalConfig,
		"VAULT_LOCAL_CONFIG": vaultLocalConfig,
	}
}

func init() {
	// No default health check: vault/openbao boot sealed+uninitialized, so the
	// manager's bootstrap (init/unseal) is the real readiness gate, not a container
	// health probe. Users can still set `container.healthcheck` explicitly.
	emu.RegisterDriver(&builtinDriver{name: "openbao", target: emu.TargetVault, image: openbaoImage, ports: []int{vaultPort}, dataDir: vaultDataDir, env: vaultEnv(), command: vaultServerCommand, restart: defaultEmulatorRestart, profile: target.VaultProfile})
	emu.RegisterDriver(&builtinDriver{name: "vault", target: emu.TargetVault, image: vaultImage, ports: []int{vaultPort}, dataDir: vaultDataDir, env: vaultEnv(), command: vaultServerCommand, restart: defaultEmulatorRestart, profile: target.VaultProfile})
}
