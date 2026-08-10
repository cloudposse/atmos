package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRestartPolicyFromStep(t *testing.T) {
	t.Run("nil step", func(t *testing.T) {
		assert.Nil(t, RestartPolicyFromStep(nil))
	})
	t.Run("no restart configured", func(t *testing.T) {
		assert.Nil(t, RestartPolicyFromStep(&schema.ContainerRunStep{}))
	})
	t.Run("empty policy is treated as unset", func(t *testing.T) {
		assert.Nil(t, RestartPolicyFromStep(&schema.ContainerRunStep{Restart: &schema.ContainerRestart{}}))
	})
	t.Run("maps policy and max_retries", func(t *testing.T) {
		got := RestartPolicyFromStep(&schema.ContainerRunStep{
			Restart: &schema.ContainerRestart{Policy: "on-failure", MaxRetries: 5},
		})
		require.NotNil(t, got)
		assert.Equal(t, "on-failure", got.Policy)
		assert.Equal(t, 5, got.MaxRetries)
	})
}

func TestHealthCheckFromStep(t *testing.T) {
	t.Run("nil step", func(t *testing.T) {
		assert.Nil(t, HealthCheckFromStep(nil))
	})
	t.Run("no healthcheck configured", func(t *testing.T) {
		assert.Nil(t, HealthCheckFromStep(&schema.ContainerRunStep{}))
	})
	t.Run("CMD-SHELL strips the prefix", func(t *testing.T) {
		got := HealthCheckFromStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{
			Test:     []string{"CMD-SHELL", "curl -sf http://localhost/ || exit 1"},
			Interval: "10s", Timeout: "5s", Retries: 3, StartPeriod: "2s",
		}})
		require.NotNil(t, got)
		assert.Equal(t, "curl -sf http://localhost/ || exit 1", got.Cmd)
		assert.Equal(t, "10s", got.Interval)
		assert.Equal(t, 3, got.Retries)
		assert.False(t, got.Disable)
	})
	t.Run("CMD joins exec-form args", func(t *testing.T) {
		got := HealthCheckFromStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{
			Test: []string{"CMD", "pg_isready", "-U", "postgres"},
		}})
		require.NotNil(t, got)
		assert.Equal(t, "pg_isready -U postgres", got.Cmd)
	})
	t.Run("bare string is treated as CMD-SHELL", func(t *testing.T) {
		got := HealthCheckFromStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{
			Test: []string{"echo ok"},
		}})
		require.NotNil(t, got)
		assert.Equal(t, "echo ok", got.Cmd)
	})
	t.Run("NONE test disables", func(t *testing.T) {
		got := HealthCheckFromStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{
			Test: []string{"NONE"},
		}})
		require.NotNil(t, got)
		assert.True(t, got.Disable)
		assert.Empty(t, got.Cmd)
	})
	t.Run("explicit disable wins over a test", func(t *testing.T) {
		got := HealthCheckFromStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{
			Test: []string{"CMD-SHELL", "true"}, Disable: true,
		}})
		require.NotNil(t, got)
		assert.True(t, got.Disable)
	})
}

func TestValidateRunStep(t *testing.T) {
	t.Run("nil step", func(t *testing.T) {
		assert.NoError(t, ValidateRunStep(nil))
	})
	t.Run("valid restart + healthcheck", func(t *testing.T) {
		err := ValidateRunStep(&schema.ContainerRunStep{
			Restart:     &schema.ContainerRestart{Policy: "unless-stopped"},
			HealthCheck: &schema.ContainerHealthCheck{Interval: "30s", Timeout: "5s", Retries: 3},
		})
		assert.NoError(t, err)
	})
	t.Run("invalid restart policy", func(t *testing.T) {
		err := ValidateRunStep(&schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "sometimes"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidContainerRestartPolicy)
	})
	t.Run("negative max_retries", func(t *testing.T) {
		err := ValidateRunStep(&schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "on-failure", MaxRetries: -1}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidContainerRestartPolicy)
	})
	t.Run("negative retries", func(t *testing.T) {
		err := ValidateRunStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{Retries: -2}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidContainerHealthCheck)
	})
	t.Run("invalid duration", func(t *testing.T) {
		err := ValidateRunStep(&schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{Interval: "soon"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidContainerHealthCheck)
	})
}
