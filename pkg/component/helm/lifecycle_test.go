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

func TestResolveReleaseLifecycleDefaults(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{})
	require.NoError(t, err)

	resolution, err := resolveReleaseLifecycle(input, releaseOperationInstall, true)
	require.NoError(t, err)
	assert.Equal(t, releaseOperationInstall, resolution.Policy.Operation)
	assert.Equal(t, kube.HookOnlyStrategy, resolution.Policy.WaitStrategy)
	assert.Equal(t, cfg.HelmDefaultMaxHistory, resolution.Policy.MaxHistory)
	assert.Zero(t, resolution.Policy.Timeout)
	assert.False(t, resolution.TimeoutExplicit)
	assert.Equal(t, "built-in default", resolution.TimeoutField)
	assert.Equal(t, failurePolicyKeep, resolution.Policy.OnFailure)
	assert.False(t, resolution.Policy.WaitForJobs)
	assert.True(t, resolution.Policy.ChartHooks)
	assert.Equal(t, crdPolicyCreate, resolution.Policy.CRDs)
	assert.True(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
}

func TestResolveReleaseLifecycleOperationOverrides(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{
		cfg.HelmReleaseSectionName: map[string]any{
			cfg.HelmTimeoutSectionName:    "5m",
			cfg.HelmChartHooksSectionName: true,
			cfg.HelmWaitSectionName: map[string]any{
				cfg.HelmWaitStrategySectionName: "watcher",
				cfg.HelmWaitJobsSectionName:     true,
			},
			cfg.HelmHistorySectionName: map[string]any{cfg.HelmHistoryMaxSectionName: float64(7)},
			cfg.HelmInstallSectionName: map[string]any{
				cfg.HelmTimeoutSectionName:   "60m",
				cfg.HelmCRDsSectionName:      "skip",
				cfg.HelmOnFailureSectionName: "uninstall",
			},
			cfg.HelmUpgradeSectionName: map[string]any{
				cfg.HelmTimeoutSectionName:          "10m",
				cfg.HelmOnFailureSectionName:        "rollback",
				cfg.HelmCleanupOnFailureSectionName: true,
			},
			cfg.HelmDeleteSectionName: map[string]any{
				cfg.HelmTimeoutSectionName:    "2m",
				cfg.HelmChartHooksSectionName: false,
			},
		},
	})
	require.NoError(t, err)

	install, err := resolveReleaseLifecycle(input, releaseOperationInstall, true)
	require.NoError(t, err)
	assert.Equal(t, time.Hour, install.Policy.Timeout)
	assert.Equal(t, "release.install.timeout", install.TimeoutField)
	assert.Equal(t, failurePolicyUninstall, install.Policy.OnFailure)
	assert.Equal(t, crdPolicySkip, install.Policy.CRDs)
	assert.True(t, install.Policy.WaitForJobs)
	assert.Equal(t, cfg.HelmDefaultMaxHistory, install.Policy.MaxHistory)

	upgrade, err := resolveReleaseLifecycle(input, releaseOperationUpgrade, true)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, upgrade.Policy.Timeout)
	assert.Equal(t, "release.upgrade.timeout", upgrade.TimeoutField)
	assert.Equal(t, failurePolicyRollback, upgrade.Policy.OnFailure)
	assert.True(t, upgrade.Policy.CleanupOnFailure)
	assert.Equal(t, 7, upgrade.Policy.MaxHistory)

	deleted, err := resolveReleaseLifecycle(input, releaseOperationDelete, true)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, deleted.Policy.Timeout)
	assert.Equal(t, "release.delete.timeout", deleted.TimeoutField)
	assert.False(t, deleted.Policy.ChartHooks)
	assert.False(t, deleted.Policy.WaitForJobs)
	assert.Equal(t, cfg.HelmDefaultMaxHistory, deleted.Policy.MaxHistory)
	assert.Empty(t, deleted.Warnings)
}

func TestResolveReleaseLifecycleDerivedWaitStrategy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		operation string
		section   string
		failure   string
	}{
		{name: "install uninstall", operation: releaseOperationInstall, section: cfg.HelmInstallSectionName, failure: "uninstall"},
		{name: "upgrade rollback", operation: releaseOperationUpgrade, section: cfg.HelmUpgradeSectionName, failure: "rollback"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input, err := decodeReleasePolicy(map[string]any{
				cfg.HelmReleaseSectionName: map[string]any{
					cfg.HelmTimeoutSectionName: "5m",
					tt.section:                 map[string]any{cfg.HelmOnFailureSectionName: tt.failure},
				},
			})
			require.NoError(t, err)
			resolution, err := resolveReleaseLifecycle(input, tt.operation, true)
			require.NoError(t, err)
			assert.Equal(t, kube.StatusWatcherStrategy, resolution.Policy.WaitStrategy)
			assert.True(t, hasLifecycleWarning(resolution.Warnings, warningWaitDerived))
		})
	}
}

func TestResolveReleaseLifecycleWithFlagsHighestPrecedence(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{
		cfg.HelmReleaseSectionName: map[string]any{
			cfg.HelmTimeoutSectionName: "5m",
			cfg.HelmWaitSectionName: map[string]any{
				cfg.HelmWaitStrategySectionName: "watcher",
			},
			cfg.HelmHistorySectionName: map[string]any{cfg.HelmHistoryMaxSectionName: 10},
			cfg.HelmUpgradeSectionName: map[string]any{
				cfg.HelmTimeoutSectionName:          "10m",
				cfg.HelmOnFailureSectionName:        "rollback",
				cfg.HelmCleanupOnFailureSectionName: true,
			},
		},
	})
	require.NoError(t, err)

	resolution, err := resolveReleaseLifecycleWithFlags(input, releaseOperationUpgrade, map[string]any{
		cfg.HelmOnFailureSectionName:        "keep",
		cfg.HelmCleanupOnFailureSectionName: false,
		cfg.HelmWaitStrategySectionName:     "legacy",
		cfg.HelmWaitJobsSectionName:         true,
		cfg.HelmTimeoutSectionName:          "30m",
		cfg.HelmHistoryMaxSectionName:       0,
		cfg.HelmChartHooksSectionName:       false,
	})
	require.NoError(t, err)
	assert.Equal(t, failurePolicyKeep, resolution.Policy.OnFailure)
	assert.False(t, resolution.Policy.CleanupOnFailure)
	assert.Equal(t, kube.LegacyStrategy, resolution.Policy.WaitStrategy)
	assert.True(t, resolution.Policy.WaitForJobs)
	assert.Equal(t, 30*time.Minute, resolution.Policy.Timeout)
	assert.Equal(t, "--timeout", resolution.TimeoutField)
	assert.Zero(t, resolution.Policy.MaxHistory)
	assert.False(t, resolution.Policy.ChartHooks)
	assert.True(t, resolution.TimeoutExplicit)
	assert.False(t, hasLifecycleWarning(resolution.Warnings, warningTimeoutMigration))
}

func TestResolveReleaseLifecycleWithFlagsWaitCompatibility(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{})
	require.NoError(t, err)
	for value, expected := range map[string]kube.WaitStrategy{
		"true":  kube.StatusWatcherStrategy,
		"false": kube.HookOnlyStrategy,
	} {
		t.Run(value, func(t *testing.T) {
			resolution, overrideErr := resolveReleaseLifecycleWithFlags(input, releaseOperationInstall, map[string]any{
				cfg.HelmWaitStrategySectionName: value,
			})
			require.NoError(t, overrideErr)
			assert.Equal(t, expected, resolution.Policy.WaitStrategy)
			assert.True(t, hasLifecycleWarning(resolution.Warnings, warningWaitBoolean))
		})
	}
}

func TestResolveReleaseLifecycleFlagApplicability(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{})
	require.NoError(t, err)
	tests := []struct {
		name      string
		operation string
		flags     map[string]any
	}{
		{name: "cleanup during install", operation: releaseOperationInstall, flags: map[string]any{cfg.HelmCleanupOnFailureSectionName: true}},
		{name: "history during install", operation: releaseOperationInstall, flags: map[string]any{cfg.HelmHistoryMaxSectionName: 10}},
		{name: "crds during upgrade", operation: releaseOperationUpgrade, flags: map[string]any{cfg.HelmCRDsSectionName: "skip"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveReleaseLifecycleWithFlags(input, tt.operation, tt.flags)
			require.ErrorIs(t, err, errUtils.ErrHelmLifecycleFlagInapplicable)
		})
	}
}

func TestDecodeReleasePolicyValidation(t *testing.T) {
	tests := []struct {
		name    string
		release any
		wantErr error
	}{
		{name: "release not object", release: "not-a-map", wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "unknown release field", release: map[string]any{"unknown": true}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "unknown operation field", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmCleanupOnFailureSectionName: true}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "wait strategy", release: map[string]any{cfg.HelmWaitSectionName: map[string]any{cfg.HelmWaitStrategySectionName: "unknown"}}, wantErr: errUtils.ErrHelmWaitStrategyInvalid},
		{name: "timeout type", release: map[string]any{cfg.HelmTimeoutSectionName: 5}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "timeout syntax", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmTimeoutSectionName: "later"}}, wantErr: errUtils.ErrHelmTimeoutInvalid},
		{name: "negative timeout", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmTimeoutSectionName: "-1s"}}, wantErr: errUtils.ErrHelmTimeoutInvalid},
		{name: "history type", release: map[string]any{cfg.HelmHistorySectionName: map[string]any{cfg.HelmHistoryMaxSectionName: 1.5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "negative history", release: map[string]any{cfg.HelmHistorySectionName: map[string]any{cfg.HelmHistoryMaxSectionName: -1}}, wantErr: errUtils.ErrHelmMaxHistoryInvalid},
		{name: "install failure enum", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmOnFailureSectionName: "rollback"}}, wantErr: errUtils.ErrHelmFailureActionInvalid},
		{name: "upgrade failure enum", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmOnFailureSectionName: "uninstall"}}, wantErr: errUtils.ErrHelmFailureActionInvalid},
		{name: "unsupported crd replace", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmCRDsSectionName: "replace"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete wait jobs", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmWaitSectionName: map[string]any{cfg.HelmWaitJobsSectionName: true}}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "jobs with hook only", release: map[string]any{cfg.HelmWaitSectionName: map[string]any{cfg.HelmWaitJobsSectionName: true}}, wantErr: errUtils.ErrHelmWaitForJobsRequiresWait},
		// Wrong-type inputs at every nested field position: each must surface a
		// decode error rather than being silently coerced.
		{name: "timeout not string", release: map[string]any{cfg.HelmTimeoutSectionName: 5}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "chart_hooks not bool", release: map[string]any{cfg.HelmChartHooksSectionName: "yes"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "wait not object", release: map[string]any{cfg.HelmWaitSectionName: "watcher"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "wait strategy not string", release: map[string]any{cfg.HelmWaitSectionName: map[string]any{cfg.HelmWaitStrategySectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "wait jobs not bool", release: map[string]any{cfg.HelmWaitSectionName: map[string]any{cfg.HelmWaitJobsSectionName: "true"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "history not object", release: map[string]any{cfg.HelmHistorySectionName: "10"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "history max not int", release: map[string]any{cfg.HelmHistorySectionName: map[string]any{cfg.HelmHistoryMaxSectionName: "ten"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install not object", release: map[string]any{cfg.HelmInstallSectionName: "yes"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install timeout not string", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmTimeoutSectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install chart_hooks not bool", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmChartHooksSectionName: "yes"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install wait not object", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmWaitSectionName: "watcher"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install crds not string", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmCRDsSectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install on_failure not string", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmOnFailureSectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "upgrade not object", release: map[string]any{cfg.HelmUpgradeSectionName: "yes"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "upgrade timeout not string", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmTimeoutSectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "upgrade chart_hooks not bool", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmChartHooksSectionName: "yes"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "upgrade wait not object", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmWaitSectionName: "watcher"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "upgrade on_failure not string", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmOnFailureSectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "upgrade cleanup_on_failure not bool", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmCleanupOnFailureSectionName: "yes"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete not object", release: map[string]any{cfg.HelmDeleteSectionName: "yes"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete timeout not string", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmTimeoutSectionName: 5}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete chart_hooks not bool", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmChartHooksSectionName: "yes"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete wait not object", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmWaitSectionName: "legacy"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeReleasePolicy(map[string]any{cfg.HelmReleaseSectionName: tt.release})
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
		{name: "integral float64", value: float64(7), want: 7, ok: true},
		{name: "unsigned overflow", value: uint64(math.MaxInt) + 1, ok: false},
		{name: "non integer", value: 1.5, ok: false},
		{name: "not finite", value: math.Inf(1), ok: false},
		{name: "float overflow", value: -float64(math.MinInt), ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := exactInt(tt.value)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRejectUnknownFieldsIsDeterministic(t *testing.T) {
	section := map[string]any{"zeta": true, "alpha": true}
	for range 20 {
		err := rejectUnknownFields(section, "release", "known")
		require.ErrorContains(t, err, "unknown field release.alpha")
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
