package target

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestAzureProfile_Branches(t *testing.T) {
	t.Run("bound port sets connection string", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetAzure, Host: "localhost", Ports: map[int]int{4577: 30002}}
		p := AzureProfile(ep)

		assert.Equal(t, azuriteAccount, p.Env["AZURE_STORAGE_ACCOUNT"])
		assert.Equal(t, azuriteKey, p.Env["AZURE_STORAGE_KEY"])

		wantConn := fmt.Sprintf(
			"DefaultEndpointsProtocol=http;AccountName=%s;AccountKey=%s;BlobEndpoint=http://localhost:30002/%s;",
			azuriteAccount, azuriteKey, azuriteAccount,
		)
		assert.Equal(t, wantConn, p.Env["AZURE_STORAGE_CONNECTION_STRING"])

		// Targets never carry a Terraform provider fragment for Azure storage.
		assert.Nil(t, p.Provider)
		assert.Empty(t, p.ResolverURL)
	})

	t.Run("no bound port omits connection string", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetAzure, Host: "localhost", Ports: map[int]int{}}
		p := AzureProfile(ep)

		// Account and key are always advertised.
		assert.Equal(t, azuriteAccount, p.Env["AZURE_STORAGE_ACCOUNT"])
		assert.Equal(t, azuriteKey, p.Env["AZURE_STORAGE_KEY"])

		// The connection string requires a live authority, so it is absent.
		assert.NotContains(t, p.Env, "AZURE_STORAGE_CONNECTION_STRING")
	})
}
