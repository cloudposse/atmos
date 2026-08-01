package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/kube"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestResolveReleaseLifecycle_Defaults(t *testing.T) {
	resolution, err := resolveReleaseLifecycle(map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, kube.HookOnlyStrategy, resolution.Policy.WaitStrategy)
	assert.Equal(t, cfg.HelmDefaultMaxHistory, resolution.Policy.MaxHistory)
	assert.Zero(t, resolution.Policy.Timeout)
	assert.False(t, resolution.TimeoutExplicit)
	assert.False(t, resolution.Policy.RollbackOnFailure)
	assert.False(t, resolution.Policy.WaitForJobs)
	assert.False(t, resolution.Policy.CleanupOnFail)
	assert.False(t, resolution.Policy.DisableChartHooks)
	assert.False(t, resolution.Policy.SkipCRDs)
	assert.True(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
}

func TestResolveReleaseLifecycle_CanonicalFields(t *testing.T) {
	resolution, err := resolveReleaseLifecycle(map[string]any{
		cfg.HelmRollbackOnFailureSectionName: true,
		cfg.HelmWaitStrategySectionName:      "legacy",
		cfg.HelmWaitForJobsSectionName:       true,
		cfg.HelmTimeoutSectionName:           "1h",
		cfg.HelmCleanupOnFailSectionName:     true,
		cfg.HelmMaxHistorySectionName:        0,
		cfg.HelmDisableChartHooksSectionName: true,
		cfg.HelmSkipCRDsSectionName:          true,
	})
	require.NoError(t, err)

	assert.True(t, resolution.Policy.RollbackOnFailure)
	assert.Equal(t, kube.LegacyStrategy, resolution.Policy.WaitStrategy)
	assert.True(t, resolution.Policy.WaitForJobs)
	assert.Equal(t, time.Hour, resolution.Policy.Timeout)
	assert.True(t, resolution.TimeoutExplicit)
	assert.True(t, resolution.Policy.CleanupOnFail)
	assert.Zero(t, resolution.Policy.MaxHistory)
	assert.True(t, resolution.Policy.DisableChartHooks)
	assert.True(t, resolution.Policy.SkipCRDs)
	assert.Empty(t, resolution.Warnings)
}

func TestResolveReleaseLifecycle_AliasesAndPrecedence(t *testing.T) {
	t.Run("aliases normalize", func(t *testing.T) {
		resolution, err := resolveReleaseLifecycle(map[string]any{
			cfg.HelmAtomicSectionName:  true,
			cfg.HelmWaitSectionName:    true,
			cfg.HelmTimeoutSectionName: "0s",
		})
		require.NoError(t, err)
		assert.True(t, resolution.Policy.RollbackOnFailure)
		assert.Equal(t, kube.StatusWatcherStrategy, resolution.Policy.WaitStrategy)
		assert.True(t, hasLifecycleWarning(resolution.Warnings, warningAtomicDeprecated))
		assert.False(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
	})

	t.Run("canonical rollback overrides inherited alias", func(t *testing.T) {
		resolution, err := resolveReleaseLifecycle(map[string]any{
			cfg.HelmAtomicSectionName:            true,
			cfg.HelmRollbackOnFailureSectionName: false,
			cfg.HelmTimeoutSectionName:           "0s",
		})
		require.NoError(t, err)
		assert.False(t, resolution.Policy.RollbackOnFailure)
		assert.True(t, hasLifecycleWarning(resolution.Warnings, warningAtomicDeprecated))
	})

	t.Run("canonical wait strategy overrides convenience alias", func(t *testing.T) {
		resolution, err := resolveReleaseLifecycle(map[string]any{
			cfg.HelmWaitSectionName:         false,
			cfg.HelmWaitStrategySectionName: "legacy",
			cfg.HelmTimeoutSectionName:      "0s",
		})
		require.NoError(t, err)
		assert.Equal(t, kube.LegacyStrategy, resolution.Policy.WaitStrategy)
		assert.True(t, hasLifecycleWarning(resolution.Warnings, warningWaitIgnored))
	})
}

func TestResolveReleaseLifecycle_DerivedWaitStrategy(t *testing.T) {
	resolution, err := resolveReleaseLifecycle(map[string]any{
		cfg.HelmRollbackOnFailureSectionName: true,
		cfg.HelmWaitForJobsSectionName:       true,
		cfg.HelmTimeoutSectionName:           "5m",
	})
	require.NoError(t, err)
	assert.Equal(t, kube.StatusWatcherStrategy, resolution.Policy.WaitStrategy)
	require.True(t, hasLifecycleWarning(resolution.Warnings, warningWaitDerived))
	assert.Contains(t, resolution.Warnings, lifecycleWarning{
		Code:    warningWaitDerived,
		Field:   cfg.HelmWaitStrategySectionName,
		Message: "helm wait strategy was derived as 'watcher' because 'rollback_on_failure' is enabled",
	})
}

func TestApplyLifecycleFlagOverrides(t *testing.T) {
	section := map[string]any{
		cfg.HelmRollbackOnFailureSectionName: true,
		cfg.HelmWaitStrategySectionName:      "watcher",
	}

	resolution, err := resolveReleaseLifecycleWithFlags(section, map[string]any{
		cfg.HelmRollbackOnFailureSectionName: false,
		cfg.HelmWaitStrategySectionName:      "legacy",
		cfg.HelmWaitForJobsSectionName:       true,
		cfg.HelmTimeoutSectionName:           "30m",
		cfg.HelmCleanupOnFailSectionName:     true,
		cfg.HelmMaxHistorySectionName:        0,
		cfg.HelmDisableChartHooksSectionName: true,
		cfg.HelmSkipCRDsSectionName:          true,
	})
	require.NoError(t, err)
	assert.False(t, resolution.Policy.RollbackOnFailure)
	assert.Equal(t, kube.LegacyStrategy, resolution.Policy.WaitStrategy)
	assert.True(t, resolution.Policy.WaitForJobs)
	assert.Equal(t, 30*time.Minute, resolution.Policy.Timeout)
	assert.True(t, resolution.TimeoutExplicit)
	assert.False(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
	assert.True(t, resolution.Policy.CleanupOnFail)
	assert.Zero(t, resolution.Policy.MaxHistory)
	assert.True(t, resolution.Policy.DisableChartHooks)
	assert.True(t, resolution.Policy.SkipCRDs)
}

func TestApplyLifecycleFlagOverridesWaitCompatibility(t *testing.T) {
	section := map[string]any{cfg.HelmTimeoutSectionName: "0s"}

	for value, expected := range map[string]kube.WaitStrategy{
		"true":  kube.StatusWatcherStrategy,
		"false": kube.HookOnlyStrategy,
	} {
		t.Run(value, func(t *testing.T) {
			resolution, overrideErr := resolveReleaseLifecycleWithFlags(section, map[string]any{cfg.HelmWaitStrategySectionName: value})
			require.NoError(t, overrideErr)
			assert.Equal(t, expected, resolution.Policy.WaitStrategy)
			assert.True(t, hasLifecycleWarning(resolution.Warnings, warningWaitBoolean))
		})
	}
}

func TestResolveReleaseLifecycleWithFlagsRecomputesDerivedWait(t *testing.T) {
	resolution, err := resolveReleaseLifecycleWithFlags(map[string]any{
		cfg.HelmRollbackOnFailureSectionName: true,
		cfg.HelmTimeoutSectionName:           "0s",
	}, map[string]any{
		cfg.HelmRollbackOnFailureSectionName: false,
	})
	require.NoError(t, err)
	assert.False(t, resolution.Policy.RollbackOnFailure)
	assert.Equal(t, kube.HookOnlyStrategy, resolution.Policy.WaitStrategy)
}

func TestResolveReleaseLifecycleWithFlagsAliasPrecedence(t *testing.T) {
	section := map[string]any{
		cfg.HelmRollbackOnFailureSectionName: false,
		cfg.HelmTimeoutSectionName:           "0s",
	}

	resolution, err := resolveReleaseLifecycleWithFlags(section, map[string]any{
		cfg.HelmAtomicSectionName: true,
	})
	require.NoError(t, err)
	assert.True(t, resolution.Policy.RollbackOnFailure)
	assert.True(t, hasLifecycleWarning(resolution.Warnings, warningAtomicDeprecated))

	resolution, err = resolveReleaseLifecycleWithFlags(section, map[string]any{
		cfg.HelmAtomicSectionName:            true,
		cfg.HelmRollbackOnFailureSectionName: false,
	})
	require.NoError(t, err)
	assert.False(t, resolution.Policy.RollbackOnFailure)
}

func TestResolveReleaseLifecycle_Validation(t *testing.T) {
	tests := []struct {
		name    string
		section map[string]any
		wantErr error
	}{
		{
			name:    "boolean type",
			section: map[string]any{cfg.HelmRollbackOnFailureSectionName: "true"},
			wantErr: errUtils.ErrHelmLifecycleDecode,
		},
		{
			name:    "wait strategy",
			section: map[string]any{cfg.HelmWaitStrategySectionName: "unknown"},
			wantErr: errUtils.ErrHelmWaitStrategyInvalid,
		},
		{
			name:    "timeout type",
			section: map[string]any{cfg.HelmTimeoutSectionName: 5},
			wantErr: errUtils.ErrHelmLifecycleDecode,
		},
		{
			name:    "timeout syntax",
			section: map[string]any{cfg.HelmTimeoutSectionName: "later"},
			wantErr: errUtils.ErrHelmTimeoutInvalid,
		},
		{
			name:    "negative timeout",
			section: map[string]any{cfg.HelmTimeoutSectionName: "-1s"},
			wantErr: errUtils.ErrHelmTimeoutInvalid,
		},
		{
			name:    "history type",
			section: map[string]any{cfg.HelmMaxHistorySectionName: 1.5},
			wantErr: errUtils.ErrHelmLifecycleDecode,
		},
		{
			name:    "negative history",
			section: map[string]any{cfg.HelmMaxHistorySectionName: -1},
			wantErr: errUtils.ErrHelmMaxHistoryInvalid,
		},
		{
			name: "wait for jobs with hook only",
			section: map[string]any{
				cfg.HelmWaitForJobsSectionName: true,
				cfg.HelmTimeoutSectionName:     "5m",
			},
			wantErr: errUtils.ErrHelmWaitForJobsRequiresWait,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveReleaseLifecycle(tt.section)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func hasLifecycleWarning(warnings []lifecycleWarning, code lifecycleWarningCode) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
