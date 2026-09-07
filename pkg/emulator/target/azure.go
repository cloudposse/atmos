package target

import (
	"fmt"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Azurite's well-known development account name and key (public, fixed values).
const (
	azuriteAccount = "devstoreaccount1"
	azuriteKey     = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="

	azureDummySubscriptionID = "00000000-0000-0000-0000-000000000000"
	azureDummyTenantID       = "00000000-0000-0000-0000-000000000000"
	azureDummyClientID       = "00000000-0000-0000-0000-000000000001"
	azureDummyClientSecret   = "test"
)

// AzureProfile builds the connection profile for an Azure-target emulator:
// Azurite-compatible storage env plus an azurerm provider fragment pointed at
// Floci's Azure metadata endpoint.
func AzureProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.AzureProfile")()

	env := map[string]string{
		"AZURE_STORAGE_ACCOUNT": azuriteAccount,
		"AZURE_STORAGE_KEY":     azuriteKey,
	}
	provider := map[string]any{
		"features":                   []map[string]any{{}},
		"skip_provider_registration": true,
		"subscription_id":            azureDummySubscriptionID,
		"tenant_id":                  azureDummyTenantID,
		"client_id":                  azureDummyClientID,
		"client_secret":              azureDummyClientSecret,
	}
	if authority := ep.Authority(); authority != "" {
		env["AZURE_STORAGE_CONNECTION_STRING"] = fmt.Sprintf(
			"DefaultEndpointsProtocol=http;AccountName=%s;AccountKey=%s;BlobEndpoint=http://%s/%s;",
			azuriteAccount, azuriteKey, authority, azuriteAccount,
		)
		provider["metadata_host"] = authority
	}
	return emu.Profile{Env: env, Provider: provider}
}
