package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	gcpDefaultProject = "floci-local"
	gcpDummyToken     = "test"
)

// GCPProfile builds the connection profile for a GCP-target emulator. GCP is
// per-service: each SDK reads its own *_EMULATOR_HOST (a bare host:port, except
// GCS which wants a URL). A single-port emulator points them all at one address.
// The Terraform provider fragment mirrors the provider's custom endpoint knobs
// for the Floci REST services that Terraform can drive.
func GCPProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.GCPProfile")()

	project := ep.Project
	if project == "" {
		project = gcpDefaultProject
	}

	env := map[string]string{
		"CLOUDSDK_AUTH_DISABLE_CREDENTIALS": "true",
		"CLOUDSDK_CORE_PROJECT":             project,
		"GOOGLE_CLOUD_PROJECT":              project,
	}
	provider := map[string]any{
		"project":               project,
		"access_token":          gcpDummyToken,
		"user_project_override": false,
	}
	if authority := ep.Authority(); authority != "" {
		storageURL := "http://" + authority
		versionedV1 := storageURL + "/v1/"

		env["STORAGE_EMULATOR_HOST"] = storageURL // GCS expects a URL.
		env["PUBSUB_EMULATOR_HOST"] = authority
		env["FIRESTORE_EMULATOR_HOST"] = authority
		env["BIGTABLE_EMULATOR_HOST"] = authority
		env["DATASTORE_EMULATOR_HOST"] = authority

		provider["storage_custom_endpoint"] = storageURL + "/storage/v1/"
		provider["secret_manager_custom_endpoint"] = versionedV1
		provider["cloud_resource_manager_custom_endpoint"] = versionedV1
		provider["pubsub_custom_endpoint"] = versionedV1
		provider["kms_custom_endpoint"] = versionedV1
		provider["logging_custom_endpoint"] = storageURL + "/v2/"
		// The Google provider's IAM client appends v1 itself; using /v1/ here
		// produces /v1/v1/... and fails against Floci.
		provider["iam_custom_endpoint"] = storageURL + "/"
		provider["iam_credentials_custom_endpoint"] = storageURL + "/"
	}
	return emu.Profile{Env: env, Provider: provider}
}
