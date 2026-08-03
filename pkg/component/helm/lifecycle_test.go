package helm

import (
	"math"
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
	assert.Empty(t, resolution.Policy.OnFailure)
	assert.False(t, resolution.Policy.WaitForJobs)
	assert.False(t, resolution.Policy.DisableChartHooks)
	assert.False(t, resolution.Policy.SkipCRDs)
	assert.True(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
}

func TestResolveReleaseLifecycle_CanonicalFields(t *testing.T) {
	resolution, err := resolveReleaseLifecycle(map[string]any{
		cfg.HelmOnFailureSectionName:         []any{"cleanup", "rollback", "cleanup"},
		cfg.HelmWaitStrategySectionName:      "legacy",
		cfg.HelmWaitForJobsSectionName:       true,
		cfg.HelmTimeoutSectionName:           "1h",
		cfg.HelmMaxHistorySectionName:        0,
		cfg.HelmDisableChartHooksSectionName: true,
		cfg.HelmSkipCRDsSectionName:          true,
	})
	require.NoError(t, err)

	assert.Equal(t, []failureAction{failureActionRollback, failureActionCleanup}, resolution.Policy.OnFailure)
	assert.Equal(t, kube.LegacyStrategy, resolution.Policy.WaitStrategy)
	assert.True(t, resolution.Policy.WaitForJobs)
	assert.Equal(t, time.Hour, resolution.Policy.Timeout)
	assert.True(t, resolution.TimeoutExplicit)
	assert.Zero(t, resolution.Policy.MaxHistory)
	assert.True(t, resolution.Policy.DisableChartHooks)
	assert.True(t, resolution.Policy.SkipCRDs)
	assert.Empty(t, resolution.Warnings)
}

func TestResolveReleaseLifecycle_AliasPrecedence(t *testing.T) {
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
		cfg.HelmOnFailureSectionName:   []any{"rollback"},
		cfg.HelmWaitForJobsSectionName: true,
		cfg.HelmTimeoutSectionName:     "5m",
	})
	require.NoError(t, err)
	assert.Equal(t, kube.StatusWatcherStrategy, resolution.Policy.WaitStrategy)
	require.True(t, hasLifecycleWarning(resolution.Warnings, warningWaitDerived))
	assert.Contains(t, resolution.Warnings, lifecycleWarning{
		Code:    warningWaitDerived,
		Field:   cfg.HelmWaitStrategySectionName,
		Message: "helm wait strategy was derived as 'watcher' because on_failure includes 'rollback'",
	})
}

func TestApplyLifecycleFlagOverrides(t *testing.T) {
	section := map[string]any{
		cfg.HelmOnFailureSectionName:    []any{"rollback"},
		cfg.HelmWaitStrategySectionName: "watcher",
	}

	resolution, err := resolveReleaseLifecycleWithFlags(section, map[string]any{
		cfg.HelmOnFailureSectionName:         []string{"cleanup"},
		cfg.HelmWaitStrategySectionName:      "legacy",
		cfg.HelmWaitForJobsSectionName:       true,
		cfg.HelmTimeoutSectionName:           "30m",
		cfg.HelmMaxHistorySectionName:        0,
		cfg.HelmDisableChartHooksSectionName: true,
		cfg.HelmSkipCRDsSectionName:          true,
	})
	require.NoError(t, err)
	assert.Equal(t, []failureAction{failureActionCleanup}, resolution.Policy.OnFailure)
	assert.Equal(t, kube.LegacyStrategy, resolution.Policy.WaitStrategy)
	assert.True(t, resolution.Policy.WaitForJobs)
	assert.Equal(t, 30*time.Minute, resolution.Policy.Timeout)
	assert.True(t, resolution.TimeoutExplicit)
	assert.False(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
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
		cfg.HelmOnFailureSectionName: []any{"rollback"},
		cfg.HelmTimeoutSectionName:   "0s",
	}, map[string]any{
		cfg.HelmOnFailureSectionName: []string{},
	})
	require.NoError(t, err)
	assert.Empty(t, resolution.Policy.OnFailure)
	assert.Equal(t, kube.HookOnlyStrategy, resolution.Policy.WaitStrategy)
}

func TestResolveReleaseLifecycle_Validation(t *testing.T) {
	tests := []struct {
		name    string
		section map[string]any
		wantErr error
	}{
		{
			name:    "failure actions type",
			section: map[string]any{cfg.HelmOnFailureSectionName: "rollback"},
			wantErr: errUtils.ErrHelmLifecycleDecode,
		},
		{
			name:    "failure action item type",
			section: map[string]any{cfg.HelmOnFailureSectionName: []any{"rollback", true}},
			wantErr: errUtils.ErrHelmLifecycleDecode,
		},
		{
			name:    "unknown failure action",
			section: map[string]any{cfg.HelmOnFailureSectionName: []any{"notify"}},
			wantErr: errUtils.ErrHelmFailureActionInvalid,
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

func TestExactInt(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "nil", value: nil, ok: false},
		{name: "signed", value: int8(-7), want: -7, ok: true},
		{name: "unsigned", value: uint16(9), want: 9, ok: true},
		{name: "unsigned overflow", value: uint64(math.MaxInt) + 1, ok: false},
		{name: "non integer", value: 1.5, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := exactInt(tt.value)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
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
