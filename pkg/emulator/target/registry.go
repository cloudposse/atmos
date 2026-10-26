package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegistryProfile builds the connection profile for an OCI / Terraform registry
// emulator: the live host:port, surfaced for vendoring / the registry cache
// (consumed via the !emulator function in V1).
func RegistryProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.RegistryProfile")()

	env := map[string]string{}
	if authority := ep.Authority(); authority != "" {
		env["ATMOS_REGISTRY_HOST"] = authority
	}
	return emu.Profile{Env: env}
}

// KubernetesProfile is the placeholder profile for kubernetes-target drivers. The
// kubeconfig is not built from the endpoint alone — it is harvested from the
// running container by the kubernetes/emulator identity (which has runtime
// access). The driver only supplies image/port defaults.
func KubernetesProfile(_ *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.KubernetesProfile")()

	return emu.Profile{}
}
