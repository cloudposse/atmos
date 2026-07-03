package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmulatorNetworkName(t *testing.T) {
	cases := map[string]string{
		"local":       "atmos-emulator-local",
		"deploy/prod": "atmos-emulator-deploy-prod",
		"a b":         "atmos-emulator-a-b",
		"":            "atmos-emulator-default",
	}
	for in, want := range cases {
		assert.Equal(t, want, emulatorNetworkName(in), "stack %q", in)
	}
}

func TestSanitizeNetworkToken(t *testing.T) {
	// Allowed characters pass through; everything else collapses to '-'.
	assert.Equal(t, "ue2-prod_1.2", sanitizeNetworkToken("ue2-prod_1.2"))
	assert.Equal(t, "x--y", sanitizeNetworkToken("x/?y"))
	assert.Equal(t, "default", sanitizeNetworkToken(""))
}

func TestEmulatorNetworkAliasScopesByStack(t *testing.T) {
	assert.Equal(t, "dev-aws", emulatorNetworkAlias("dev", "aws"))
	assert.Equal(t, "deploy-prod-aws", emulatorNetworkAlias("deploy/prod", "aws"))
}
