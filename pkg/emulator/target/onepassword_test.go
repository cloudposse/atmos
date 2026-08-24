package target

import (
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestOnePasswordProfile(t *testing.T) {
	profile := OnePasswordProfile(&emu.Endpoint{
		Target: emu.TargetOnePassword,
		Host:   "localhost",
		Ports:  map[int]int{3000: 18080},
	})

	assert.Equal(t, "http://127.0.0.1:18080", profile.Env["OP_CONNECT_HOST"])
	assert.Equal(t, onePasswordMockToken, profile.Env["OP_CONNECT_TOKEN"])
	assert.Nil(t, profile.Provider)
	assert.Empty(t, profile.ResolverURL)
}
