package emulator

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GCP emulator environment variables carried in the resolved emulator profile.
// GCP is per-service: each SDK reads its own *_EMULATOR_HOST. The only in-process
// Atmos consumer today (the GCS Terraform-state backend reader) needs the storage
// endpoint, so that's what we carry into the auth context.
const (
	envGCPStorageEmulatorHost = "STORAGE_EMULATOR_HOST"
	envGCPProject             = "GOOGLE_CLOUD_PROJECT"
	envGCPCoreProject         = "CLOUDSDK_CORE_PROJECT"
	envGCPDisableCredentials  = "CLOUDSDK_AUTH_DISABLE_CREDENTIALS"
)

// setGCPAuthContext populates params.AuthContext.GCP for a gcp/emulator identity.
//
// In-process GCP SDK clients — currently the GCS Terraform-state backend reader used
// by `!terraform.state`/`!terraform.output` — build their config from AuthContext.GCP,
// NOT from the subprocess environment that PrepareEnvironment injects for Terraform. We
// copy the storage endpoint straight from the resolved emulator profile (the standard
// STORAGE_EMULATOR_HOST var) so those in-process consumers reach the emulator just like
// Terraform does. No new var names are invented — the profile env is the contract.
func (i *Identity) setGCPAuthContext(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "emulator.Identity.setGCPAuthContext")()

	env, err := i.resolveEmulatorEnvForContext(ctx, params)
	if err != nil {
		return err
	}
	if env == nil {
		return nil
	}

	project := env[envGCPProject]
	if project == "" {
		project = env[envGCPCoreProject]
	}

	params.AuthContext.GCP = &schema.GCPAuthContext{
		ProjectID:             project,
		StorageEmulatorHost:   env[envGCPStorageEmulatorHost],
		WithoutAuthentication: env[envGCPDisableCredentials] == "true",
	}
	return nil
}
