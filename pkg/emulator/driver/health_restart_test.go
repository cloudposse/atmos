package driver

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

// TestBuiltinDrivers_RestartDefault asserts every built-in driver defaults to the
// `unless-stopped` restart policy, since emulators are long-lived local services.
func TestBuiltinDrivers_RestartDefault(t *testing.T) {
	for _, name := range []string{
		"floci/aws", "floci/gcp", "floci/az", "registry", "k3s",
		"openbao", "vault", "ministack/aws", "localstack/aws",
	} {
		t.Run(name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)
			r := d.Defaults().Restart
			require.NotNil(t, r, "%s should default to a restart policy", name)
			assert.Equal(t, "unless-stopped", r.Policy)
		})
	}
}

// TestBuiltinDrivers_HealthCheckDefault asserts the verified lightweight emulators
// ship a default health check (so `up` can gate on readiness), and that the
// drivers whose readiness is handled elsewhere (vault bootstrap, k3s API wait) or
// whose image probe is unverified (ministack/localstack) do NOT — a wrong default
// probe would otherwise brick `up`.
func TestBuiltinDrivers_HealthCheckDefault(t *testing.T) {
	withHealthCheck := map[string]string{
		"floci/aws": "4566",
		"floci/gcp": "4588",
		"floci/az":  "4577",
		"registry":  "/v2/",
	}
	for name, probe := range withHealthCheck {
		t.Run("has/"+name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)
			hc := d.Defaults().HealthCheck
			require.NotNil(t, hc, "%s should ship a default health check", name)
			require.Len(t, hc.Test, 2)
			assert.Equal(t, "CMD-SHELL", hc.Test[0])
			assert.Contains(t, hc.Test[1], probe)
			assert.NotEmpty(t, hc.Interval)
		})
	}

	for _, name := range []string{"k3s", "openbao", "vault", "ministack/aws", "localstack/aws"} {
		t.Run("none/"+name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)
			assert.Nil(t, d.Defaults().HealthCheck, "%s should not ship a default health check", name)
		})
	}
}

// TestFlociHealthCheck_UsesItsOwnPort guards against a port/healthcheck mismatch
// across the Floci variants (each probes its own edge port).
func TestFlociHealthCheck_UsesItsOwnPort(t *testing.T) {
	cases := map[string]string{"floci/aws": "4566", "floci/gcp": "4588", "floci/az": "4577"}
	for name, port := range cases {
		d, err := emu.ResolveDriver(name)
		require.NoError(t, err)
		hc := d.Defaults().HealthCheck
		require.NotNil(t, hc)
		assert.True(t, strings.Contains(hc.Test[1], ":"+port+"/"),
			"%s health check should probe port %s, got %q", name, port, hc.Test[1])
	}
}
