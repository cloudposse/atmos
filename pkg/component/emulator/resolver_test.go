package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveEmulator_Success(t *testing.T) {
	mgr := &fakeManager{
		resProfile: emu.Profile{
			Env:        map[string]string{"AWS_ENDPOINT_URL": "http://localhost:54321"},
			Kubeconfig: []byte("kubeconfig-bytes"),
		},
	}
	stubPrepare(t, validSection(), nil, mgr)

	env, kubeconfig, err := emulatorResolver{}.ResolveEmulator(context.Background(), "dev", "aws")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"AWS_ENDPOINT_URL": "http://localhost:54321"}, env)
	assert.Equal(t, []byte("kubeconfig-bytes"), kubeconfig)
	assert.Equal(t, 1, mgr.resCalls)
	assert.Equal(t, "dev", mgr.gotStack)
	assert.Equal(t, "aws", mgr.gotName)
}

func TestResolveEmulator_PrepareError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errBoom
	}
	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "dev", "aws")
	require.ErrorIs(t, err, errBoom)
}

func TestResolveEmulator_ResolveError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{resErr: errBoom})
	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "dev", "aws")
	require.ErrorIs(t, err, errBoom)
}
