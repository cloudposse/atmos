package emulator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// healthDriver is an in-package driver that ships a default health check and
// restart policy, so the precedence/wiring tests don't depend on a concrete
// product driver (which lives in pkg/emulator/driver).
type healthDriver struct{}

func (healthDriver) Name() string   { return "test/health" }
func (healthDriver) Target() string { return TargetAWS }

func (healthDriver) Defaults() ContainerDefaults {
	return ContainerDefaults{
		Image:       "test/health:latest",
		Ports:       []int{4566},
		HealthCheck: &schema.ContainerHealthCheck{Test: []string{"CMD-SHELL", "curl -s http://localhost:4566/ || exit 1"}, Interval: "10s"},
		Restart:     &schema.ContainerRestart{Policy: "unless-stopped"},
	}
}

func (healthDriver) Profile(ep *Endpoint) Profile { return Profile{} }

func init() {
	RegisterDriver(healthDriver{})
}

func TestSpec_EffectiveHealthCheck(t *testing.T) {
	t.Run("falls back to the driver default", func(t *testing.T) {
		spec := &Spec{Driver: "test/health"}
		hc, err := spec.EffectiveHealthCheck()
		require.NoError(t, err)
		require.NotNil(t, hc)
		assert.Contains(t, hc.Cmd, "curl")
		assert.False(t, hc.Disable)
	})

	t.Run("component config overrides the driver default", func(t *testing.T) {
		spec := &Spec{Driver: "test/health", Container: &schema.ContainerRunStep{
			HealthCheck: &schema.ContainerHealthCheck{Test: []string{"CMD-SHELL", "my-probe"}},
		}}
		hc, err := spec.EffectiveHealthCheck()
		require.NoError(t, err)
		require.NotNil(t, hc)
		assert.Equal(t, "my-probe", hc.Cmd)
	})

	t.Run("component can disable the driver default", func(t *testing.T) {
		spec := &Spec{Driver: "test/health", Container: &schema.ContainerRunStep{
			HealthCheck: &schema.ContainerHealthCheck{Test: []string{"NONE"}},
		}}
		hc, err := spec.EffectiveHealthCheck()
		require.NoError(t, err)
		require.NotNil(t, hc)
		assert.True(t, hc.Disable)
	})

	t.Run("nil when the driver has no default", func(t *testing.T) {
		spec := &Spec{Driver: testDriverName}
		hc, err := spec.EffectiveHealthCheck()
		require.NoError(t, err)
		assert.Nil(t, hc)
	})
}

func TestSpec_EffectiveRestart(t *testing.T) {
	t.Run("falls back to the driver default", func(t *testing.T) {
		spec := &Spec{Driver: "test/health"}
		r, err := spec.EffectiveRestart()
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, "unless-stopped", r.Policy)
	})

	t.Run("component config overrides the driver default", func(t *testing.T) {
		spec := &Spec{Driver: "test/health", Container: &schema.ContainerRunStep{
			Restart: &schema.ContainerRestart{Policy: "always"},
		}}
		r, err := spec.EffectiveRestart()
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, "always", r.Policy)
	})

	t.Run("nil when the driver has no default", func(t *testing.T) {
		spec := &Spec{Driver: testDriverName}
		r, err := spec.EffectiveRestart()
		require.NoError(t, err)
		assert.Nil(t, r)
	})
}

func TestSpec_Validate_RejectsBadRestart(t *testing.T) {
	spec := &Spec{Driver: "test/health", Container: &schema.ContainerRunStep{
		Restart: &schema.ContainerRestart{Policy: "occasionally"},
	}}
	err := spec.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidContainerRestartPolicy)
}

func TestManager_namedConfig_WiresHealthAndRestart(t *testing.T) {
	m := newManagerWithRuntime(nil)

	t.Run("driver defaults flow into NamedConfig", func(t *testing.T) {
		cfg, err := m.namedConfig(&Spec{Driver: "test/health"}, "dev", "aws", nil, false)
		require.NoError(t, err)
		require.NotNil(t, cfg.HealthCheck)
		assert.True(t, strings.Contains(cfg.HealthCheck.Cmd, "curl"))
		require.NotNil(t, cfg.Restart)
		assert.Equal(t, "unless-stopped", cfg.Restart.Policy)
	})

	t.Run("no health/restart when the driver has none", func(t *testing.T) {
		cfg, err := m.namedConfig(&Spec{Driver: testDriverName}, "dev", "aws", nil, false)
		require.NoError(t, err)
		assert.Nil(t, cfg.HealthCheck)
		assert.Nil(t, cfg.Restart)
	})
}
