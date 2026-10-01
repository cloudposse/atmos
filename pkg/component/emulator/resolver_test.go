package emulator

import (
	"context"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
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

func TestResolveEmulator_QualifiedInstanceNotConfigured(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		return info, errUtils.ErrInvalidComponent
	}

	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "local/aws")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotConfigured)
	assert.Contains(t, err.Error(), "local/aws")
	assert.Contains(t, cockroachErrors.GetAllHints(err), "Configure emulator \"aws\" in stack \"local\" before starting it.")
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

func TestEmulatorReferenceErrorsAndAddresses(t *testing.T) {
	assert.Equal(t, "aws", (emulatorReference{Name: "aws"}).String())
	assert.Equal(t, "dev/aws", (emulatorReference{Stack: "dev", Name: "aws"}).String())
	assert.Equal(t, "dev/aws", emulatorInstanceAddress("dev", "aws"))

	bareConfigured := emulatorNotConfiguredError(emulatorReference{Name: "aws"})
	require.ErrorIs(t, bareConfigured, errUtils.ErrEmulatorNotConfigured)
	assert.Contains(t, cockroachErrors.GetAllHints(bareConfigured), "Run `atmos emulator list` to find configured emulator instances.")

	bareStopped := emulatorNotRunningError(emulatorReference{Name: "aws"})
	require.ErrorIs(t, bareStopped, errUtils.ErrEmulatorNotRunning)
	assert.Contains(t, cockroachErrors.GetAllHints(bareStopped), "Start it with `atmos emulator up aws -s <stack>`.")
}

func TestRunningEmulatorsMatchingFiltersAndSorts(t *testing.T) {
	matches := runningEmulatorsMatching([]emu.Status{
		{Name: "aws", Stack: "prod", Container: "z", Status: "running"},
		{Name: "aws", Stack: "dev", Container: "b", Status: "running"},
		{Name: "aws", Stack: "dev", Container: "a", Status: "running"},
		{Name: "aws", Stack: "dev", Container: "stopped", Status: "exited"},
		{Name: "gcp", Stack: "dev", Container: "other", Status: "running"},
	}, emulatorReference{Name: "aws"})

	require.Equal(t, []string{"dev/a", "dev/b", "prod/z"}, []string{
		emulatorInstanceAddress(matches[0].Stack, matches[0].Container),
		emulatorInstanceAddress(matches[1].Stack, matches[1].Container),
		emulatorInstanceAddress(matches[2].Stack, matches[2].Container),
	})

	qualified := runningEmulatorsMatching(matches, emulatorReference{Stack: "prod", Name: "aws"})
	require.Equal(t, []emu.Status{{Name: "aws", Stack: "prod", Container: "z", Status: "running"}}, qualified)
}

func TestResolveEmulator_RuntimeListingError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{psErr: errBoom})

	_, _, err := emulatorResolver{}.ResolveEmulator(context.Background(), "aws")
	require.ErrorIs(t, err, errBoom)
}
