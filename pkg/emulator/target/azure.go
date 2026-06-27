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
)

// AzureProfile builds the connection profile for an Azure-target emulator
// (Azurite-compatible storage): an Azure Storage connection string + account
// credentials pointed at the live blob endpoint.
func AzureProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.AzureProfile")()

	env := map[string]string{
		"AZURE_STORAGE_ACCOUNT": azuriteAccount,
		"AZURE_STORAGE_KEY":     azuriteKey,
	}
	if authority := ep.Authority(); authority != "" {
		env["AZURE_STORAGE_CONNECTION_STRING"] = fmt.Sprintf(
			"DefaultEndpointsProtocol=http;AccountName=%s;AccountKey=%s;BlobEndpoint=http://%s/%s;",
			azuriteAccount, azuriteKey, authority, azuriteAccount,
		)
	}
	return emu.Profile{Env: env}
}
