package target

import (
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestVaultProfile_Branches(t *testing.T) {
	t.Run("bound port sets VAULT_ADDR and BAO_ADDR", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetVault, Host: "localhost", Ports: map[int]int{8200: 8200}}
		p := VaultProfile(ep)

		assert.Equal(t, "http://127.0.0.1:8200", p.Env["VAULT_ADDR"])
		assert.Equal(t, "http://127.0.0.1:8200", p.Env["BAO_ADDR"])
		// The token is dynamic and harvested by the manager in Resolve, not set here.
		assert.NotContains(t, p.Env, "VAULT_TOKEN")
	})

	t.Run("no bound port omits the address", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetVault, Host: "localhost", Ports: map[int]int{}}
		p := VaultProfile(ep)

		// Without a live URL, VAULT_ADDR/BAO_ADDR are absent.
		assert.NotContains(t, p.Env, "VAULT_ADDR")
		assert.NotContains(t, p.Env, "BAO_ADDR")
	})
}
