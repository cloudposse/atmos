package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveProfile_Success(t *testing.T) {
	mgr := &fakeManager{
		resEndp:    emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{4566: 54321}},
		resProfile: emu.Profile{Env: map[string]string{"AWS_ENDPOINT_URL": "http://localhost:54321"}},
	}
	stubPrepare(t, validSection(), nil, mgr)

	endpoint, profile, err := resolveProfile(&schema.AtmosConfiguration{}, "aws", "dev", nil)
	require.NoError(t, err)
	assert.Equal(t, emu.TargetAWS, endpoint.Target)
	assert.Equal(t, 54321, endpoint.Ports[4566])
	assert.Equal(t, "http://localhost:54321", profile.Env["AWS_ENDPOINT_URL"])
	assert.Equal(t, 1, mgr.resCalls)
	assert.Equal(t, "dev", mgr.gotStack)
	assert.Equal(t, "aws", mgr.gotName)
}

func TestResolveProfile_PrepareError(t *testing.T) {
	stubPrepare(t, map[string]any{}, nil, &fakeManager{}) // invalid spec -> prepare fails.
	_, _, err := resolveProfile(&schema.AtmosConfiguration{}, "aws", "dev", nil)
	require.Error(t, err)
}

func TestResolveProfile_ResolveError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{resErr: errBoom})
	_, _, err := resolveProfile(&schema.AtmosConfiguration{}, "aws", "dev", nil)
	require.ErrorIs(t, err, errBoom)
}
