package emulator

import (
	"context"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveEmulator_QualifiedInstance(t *testing.T) {
	mgr := &fakeManager{
		psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "running"}},
		resProfile: emu.Profile{
			Env:        map[string]string{"AWS_ENDPOINT_URL": "http://localhost:54321"},
			Kubeconfig: []byte("kubeconfig-bytes"),
		},
	}
	stubPrepare(t, validSection(), nil, mgr)

	env, kubeconfig, err := emulatorResolver{}.ResolveEmulator(context.Background(), "local/aws")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"AWS_ENDPOINT_URL": "http://localhost:54321"}, env)
	assert.Equal(t, []byte("kubeconfig-bytes"), kubeconfig)
	assert.Equal(t, 1, mgr.resCalls)
	assert.Equal(t, "local", mgr.gotStack)
	assert.Equal(t, "aws", mgr.gotName)
}

func TestResolveEmulator_BareNameRequiresOneRunningInstance(t *testing.T) {
	mgr := &fakeManager{
		psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "running"}},
		resProfile: emu.Profile{Env: map[string]string{"AWS_ENDPOINT_URL": "http://localhost:54321"}},
	}
	stubPrepare(t, validSection(), nil, mgr)

	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "aws")
	require.NoError(t, err)
	assert.Equal(t, "local", mgr.gotStack)
}

func TestResolveEmulator_QualifiedInstanceIgnoresStoppedMatches(t *testing.T) {
	mgr := &fakeManager{psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "not running"}}}
	stubPrepare(t, validSection(), nil, mgr)

	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "local/aws")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
	assert.Contains(t, err.Error(), "local/aws")
	assert.Contains(t, cockroachErrors.GetAllHints(err), "Start it with `atmos emulator up aws -s local`.")
}

func TestResolveEmulator_BareNameIsAmbiguousAcrossStacks(t *testing.T) {
	mgr := &fakeManager{psStatuses: []emu.Status{
		{Name: "aws", Stack: "dev", Status: "running"},
		{Name: "aws", Stack: "local", Status: "running"},
	}}
	stubPrepare(t, validSection(), nil, mgr)

	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "aws")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorAmbiguous)
	assert.Contains(t, err.Error(), "dev/aws")
	assert.Contains(t, err.Error(), "local/aws")
}

func TestResolveEmulator_PrepareError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errBoom
	}
	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "local/aws")
	require.ErrorIs(t, err, errBoom)
}

func TestResolveEmulator_ResolveError(t *testing.T) {
	mgr := &fakeManager{
		psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "running"}},
		resErr:     errBoom,
	}
	stubPrepare(t, validSection(), nil, mgr)
	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "local/aws")
	require.ErrorIs(t, err, errBoom)
}

func TestParseEmulatorReference(t *testing.T) {
	tests := []struct {
		value string
		want  emulatorReference
		valid bool
	}{
		{value: "aws", want: emulatorReference{Name: "aws"}, valid: true},
		{value: " local/aws ", want: emulatorReference{Stack: "local", Name: "aws"}, valid: true},
		{value: "", valid: false},
		{value: "/aws", valid: false},
		{value: "local/", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseEmulatorReference(tt.value)
			if !tt.valid {
				require.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
