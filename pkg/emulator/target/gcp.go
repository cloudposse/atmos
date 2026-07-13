package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GCPProfile builds the connection profile for a GCP-target emulator. GCP is
// per-service: each SDK reads its own *_EMULATOR_HOST (a bare host:port, except
// GCS which wants a URL). A single-port emulator points them all at one address.
func GCPProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.GCPProfile")()

	env := map[string]string{
		"CLOUDSDK_AUTH_DISABLE_CREDENTIALS": "true",
	}
	if authority := ep.Authority(); authority != "" {
		env["STORAGE_EMULATOR_HOST"] = "http://" + authority // GCS expects a URL.
		env["PUBSUB_EMULATOR_HOST"] = authority
		env["FIRESTORE_EMULATOR_HOST"] = authority
		env["BIGTABLE_EMULATOR_HOST"] = authority
		env["DATASTORE_EMULATOR_HOST"] = authority
	}
	if ep.Project != "" {
		env["CLOUDSDK_CORE_PROJECT"] = ep.Project
		env["GOOGLE_CLOUD_PROJECT"] = ep.Project
	}
	return emu.Profile{Env: env}
}
