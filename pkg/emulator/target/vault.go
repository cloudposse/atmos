package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// VaultProfile builds the connection profile for a Vault/OpenBao emulator
// (API-compatible): VAULT_ADDR / BAO_ADDR pointing at the published endpoint. The
// root token is dynamic (the file-backed server generates it at init), so the
// manager harvests it from the running container and adds VAULT_TOKEN/BAO_TOKEN in
// Resolve — this builder only sets the address.
func VaultProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.VaultProfile")()

	env := map[string]string{}
	if url := ep.URL("http"); url != "" {
		env["VAULT_ADDR"] = url
		env["BAO_ADDR"] = url
	}
	return emu.Profile{Env: env}
}
