package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GitProfile builds the connection profile for a Git server emulator (Gitea): it
// surfaces the live HTTP base URL so config and templates can reference the
// running endpoint (e.g. composing a clone/push URL when the host port is
// auto-assigned). The repository credentials are throwaway-local and embedded in
// the configured remote URL, so the profile carries no secret.
func GitProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.GitProfile")()

	env := map[string]string{}
	if url := ep.URL("http"); url != "" {
		env["ATMOS_GIT_EMULATOR_URL"] = url
	}
	return emu.Profile{Env: env}
}
