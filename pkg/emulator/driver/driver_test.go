package driver

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestResolveDriver(t *testing.T) {
	t.Run("resolves the built-in floci/aws driver", func(t *testing.T) {
		d, err := emu.ResolveDriver("floci/aws")
		require.NoError(t, err)
		assert.Equal(t, "floci/aws", d.Name())
		assert.Equal(t, emu.TargetAWS, d.Target())
	})

	t.Run("unknown driver returns ErrUnknownEmulatorDriver", func(t *testing.T) {
		_, err := emu.ResolveDriver("does-not-exist")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	})
}

func TestDrivers_IncludesFlociAWS(t *testing.T) {
	names := emu.Drivers()
	assert.Contains(t, names, "floci/aws")
	// Sorted output.
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, names[i-1], names[i], "Drivers() must be sorted")
	}
}

// TestK3sDriverDefaults verifies the k3s driver supplies the container defaults a
// nested-Kubernetes server needs to start: privileged mode, a node token, and the
// `server` command. Without these the rancher/k3s container exits immediately
// ("--token is required", "--server is required", or refuses to run unprivileged).
func TestK3sDriverDefaults(t *testing.T) {
	d, err := emu.ResolveDriver("k3s")
	require.NoError(t, err)

	defaults := d.Defaults()
	assert.True(t, defaults.Privileged, "k3s must run privileged")
	assert.Equal(t, []string{"server"}, defaults.Command, "k3s must run the server command")
	assert.NotEmpty(t, defaults.Env["K3S_TOKEN"], "k3s requires a node token")
}

func TestRegisteredDrivers_TargetsAndDefaults(t *testing.T) {
	cases := map[string]struct {
		target string
	}{
		"floci/aws":                 {emu.TargetAWS},
		"ministack/aws":             {emu.TargetAWS},
		"localstack/aws":            {emu.TargetAWS},
		"floci/gcp":                 {emu.TargetGCP},
		"floci/az":                  {emu.TargetAzure},
		"k3s":                       {emu.TargetKubernetes},
		"openbao":                   {emu.TargetVault},
		"vault":                     {emu.TargetVault},
		"registry":                  {emu.TargetRegistry},
		"mockoon/1password-connect": {emu.TargetOnePassword},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)
			assert.Equal(t, want.target, d.Target())
			assert.NotEmpty(t, d.Defaults().Image, "driver must supply a default image")
			require.NotEmpty(t, d.Defaults().Ports)
		})
	}
}

func TestFlociDriver_Defaults(t *testing.T) {
	d, err := emu.ResolveDriver("floci/aws")
	require.NoError(t, err)
	defaults := d.Defaults()
	assert.Equal(t, flociAWSImage, defaults.Image)
	require.Len(t, defaults.Ports, 1)
	assert.Equal(t, flociAWSPort, defaults.Ports[0])
}

func TestFlociDriver_Profile(t *testing.T) {
	d, err := emu.ResolveDriver("floci/aws")
	require.NoError(t, err)

	ep := emu.Endpoint{
		Target: emu.TargetAWS,
		Host:   "localhost",
		Ports:  map[int]int{4566: 54321},
		Region: "eu-west-1",
	}
	profile := d.Profile(&ep)

	// Env: live endpoint URL + dummy creds + region.
	assert.Equal(t, "http://127.0.0.1:54321", profile.Env["AWS_ENDPOINT_URL"])
	assert.Equal(t, "test", profile.Env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "test", profile.Env["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "eu-west-1", profile.Env["AWS_REGION"])
	assert.Equal(t, "eu-west-1", profile.Env["AWS_DEFAULT_REGION"])

	// Internal-SDK resolver URL.
	assert.Equal(t, "http://127.0.0.1:54321", profile.ResolverURL)

	// Provider fragment: behavior flags env cannot set.
	assert.Equal(t, true, profile.Provider["skip_requesting_account_id"])
	assert.Equal(t, true, profile.Provider["s3_use_path_style"])
	assert.Equal(t, true, profile.Provider["skip_credentials_validation"])
	assert.Equal(t, "eu-west-1", profile.Provider["region"])

	// No kubeconfig for an AWS target.
	assert.Nil(t, profile.Kubeconfig)
}

func TestFlociDriver_Profile_DefaultRegion(t *testing.T) {
	d, err := emu.ResolveDriver("floci/aws")
	require.NoError(t, err)
	profile := d.Profile(&emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{4566: 4566}})
	assert.Equal(t, "us-east-1", profile.Env["AWS_REGION"], "AWS target default region")
}

// Compile-time guard: the sentinel referenced by ResolveDriver exists.
var _ = errors.Is(errUtils.ErrUnknownEmulatorDriver, errUtils.ErrUnknownEmulatorDriver)
