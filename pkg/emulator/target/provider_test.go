package target

import (
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestTerraformProviderName(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		emu.TargetAWS:        {"aws", true},
		emu.TargetGCP:        {"google", true},
		emu.TargetAzure:      {"azurerm", true},
		emu.TargetKubernetes: {"", false},
		emu.TargetVault:      {"", false},
		emu.TargetRegistry:   {"", false},
		"made-up":            {"", false},
	}
	for target, want := range cases {
		t.Run(target, func(t *testing.T) {
			got, ok := TerraformProviderName(target)
			assert.Equal(t, want.ok, ok)
			assert.Equal(t, want.want, got)
		})
	}
}
