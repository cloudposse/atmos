package emulator

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Azure emulator environment variables carried in the resolved emulator profile.
// Azurite is connection-string-shaped: AZURE_STORAGE_CONNECTION_STRING carries the
// blob endpoint together with the account name + key.
const envAzureStorageConnectionString = "AZURE_STORAGE_CONNECTION_STRING"

// setAzureAuthContext populates params.AuthContext.Azure for an azure/emulator identity.
//
// In-process Azure SDK clients — currently the azurerm Terraform-state backend reader
// used by `!terraform.state`/`!terraform.output` — build their config from
// AuthContext.Azure, NOT from the subprocess environment that PrepareEnvironment injects
// for Terraform. We copy the Azurite storage connection string straight from the
// resolved emulator profile so those in-process consumers reach the emulator just like
// Terraform does. No new var names are invented — the profile env is the contract.
func (i *Identity) setAzureAuthContext(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "emulator.Identity.setAzureAuthContext")()

	env, err := i.resolveEmulatorEnvForContext(ctx, params)
	if err != nil {
		return err
	}
	if env == nil {
		return nil
	}

	connStr, ok := env[envAzureStorageConnectionString]
	if !ok || connStr == "" {
		return nil
	}

	params.AuthContext.Azure = &schema.AzureAuthContext{
		Profile:                 params.IdentityName,
		StorageConnectionString: connStr,
	}
	return nil
}
