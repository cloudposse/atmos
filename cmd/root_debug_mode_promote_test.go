package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ci"
	github "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

// registerFreshGHAProvider snapshots the CI provider registry, installs a
// fresh GitHub Actions provider that reads the current process env, and
// schedules registry restoration on test cleanup.
func registerFreshGHAProvider(t *testing.T) {
	t.Helper()
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	ci.Register(github.NewProvider())
}

func TestMaybePromoteLogLevelForDebugMode(t *testing.T) {
	t.Run("config not loaded -> no promote", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "true")
		t.Setenv("ACTIONS_STEP_DEBUG", "")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Info"

		got := maybePromoteLogLevelForDebugMode(cfg, false)
		assert.False(t, got.Promoted)
		assert.Equal(t, "Info", cfg.Logs.Level)
	})

	t.Run("ci.enabled=false -> no promote even with debug env set", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_STEP_DEBUG", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = false
		cfg.Logs.Level = "Info"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.False(t, got.Promoted)
		assert.Equal(t, "Info", cfg.Logs.Level)
	})

	t.Run("no provider detected -> no promote", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "true")
		t.Setenv("ACTIONS_STEP_DEBUG", "true")
		// Empty registry — no providers at all.
		restore := ci.SwapRegistryForTest()
		t.Cleanup(restore)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Info"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.False(t, got.Promoted)
	})

	t.Run("GHA detected but no debug env -> no promote", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "")
		t.Setenv("ACTIONS_STEP_DEBUG", "")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Info"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.False(t, got.Promoted)
	})

	t.Run("ACTIONS_RUNNER_DEBUG promotes Info -> Debug", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "true")
		t.Setenv("ACTIONS_STEP_DEBUG", "")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Info"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.True(t, got.Promoted)
		assert.Equal(t, "Info", got.From)
		assert.Equal(t, "github-actions", got.Provider)
		assert.Equal(t, "Debug", cfg.Logs.Level)
	})

	t.Run("ACTIONS_STEP_DEBUG promotes Warning -> Debug", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "")
		t.Setenv("ACTIONS_STEP_DEBUG", "true")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Warning"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.True(t, got.Promoted)
		assert.Equal(t, "Warning", got.From)
		assert.Equal(t, "Debug", cfg.Logs.Level)
	})

	t.Run("overrides explicit Trace (CI signal wins)", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "true")
		t.Setenv("ACTIONS_STEP_DEBUG", "")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Trace"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.True(t, got.Promoted)
		assert.Equal(t, "Trace", got.From)
		assert.Equal(t, "Debug", cfg.Logs.Level)
	})

	t.Run("overrides explicit Off (CI signal wins)", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_RUNNER_DEBUG", "")
		t.Setenv("ACTIONS_STEP_DEBUG", "true")
		registerFreshGHAProvider(t)

		cfg := &schema.AtmosConfiguration{}
		cfg.CI.Enabled = true
		cfg.Logs.Level = "Off"

		got := maybePromoteLogLevelForDebugMode(cfg, true)
		assert.True(t, got.Promoted)
		assert.Equal(t, "Off", got.From)
		assert.Equal(t, "Debug", cfg.Logs.Level)
	})
}

// TestMaybePromoteLogLevelForDebugMode_NilConfig verifies the helper does
// not panic on a nil config (defensive — never expected at the real call
// site).
func TestMaybePromoteLogLevelForDebugMode_NilConfig(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()

	got := maybePromoteLogLevelForDebugMode(nil, true)
	assert.False(t, got.Promoted)
}
