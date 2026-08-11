package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

const onePasswordMockToken = "mockoon-token"

// OnePasswordProfile builds the connection profile for a 1Password Connect mock.
func OnePasswordProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.OnePasswordProfile")()

	env := map[string]string{
		"OP_CONNECT_TOKEN": onePasswordMockToken,
	}
	if url := ep.URL("http"); url != "" {
		env["OP_CONNECT_HOST"] = url
	}
	return emu.Profile{Env: env}
}
