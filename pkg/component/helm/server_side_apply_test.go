package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestParseServerSideApply(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    serverSideApplyPolicy
		wantErr bool
	}{
		{name: "auto", value: "auto", want: serverSideApplyAuto},
		{name: "true", value: "true", want: serverSideApplyTrue},
		{name: "false", value: "false", want: serverSideApplyFalse},
		{name: "invalid", value: "maybe", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServerSideApply(tt.value)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrHelmServerSideApplyInvalid)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveServerSideApplyDefaults confirms that an omitted setting leaves the
// policy unset (so the Helm default is preserved) and force_conflicts off.
func TestResolveServerSideApplyDefaults(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{})
	require.NoError(t, err)

	for _, operation := range []string{releaseOperationInstall, releaseOperationUpgrade, releaseOperationDelete} {
		resolution, resolveErr := resolveReleaseLifecycle(input, operation, false)
		require.NoError(t, resolveErr)
		assert.Equal(t, serverSideApplyUnset, resolution.Policy.ServerSideApply, operation)
		assert.False(t, resolution.Policy.ForceConflicts, operation)
	}
}

// TestResolveServerSideApplyReleaseWide confirms release-wide settings reach the
// install and upgrade actions but never the delete action.
func TestResolveServerSideApplyReleaseWide(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{
		cfg.HelmReleaseSectionName: map[string]any{
			cfg.HelmServerSideApplySectionName: "auto",
			cfg.HelmForceConflictsSectionName:  true,
		},
	})
	require.NoError(t, err)

	install, err := resolveReleaseLifecycle(input, releaseOperationInstall, false)
	require.NoError(t, err)
	assert.Equal(t, serverSideApplyAuto, install.Policy.ServerSideApply)
	assert.True(t, install.Policy.ForceConflicts)

	upgrade, err := resolveReleaseLifecycle(input, releaseOperationUpgrade, false)
	require.NoError(t, err)
	assert.Equal(t, serverSideApplyAuto, upgrade.Policy.ServerSideApply)
	assert.True(t, upgrade.Policy.ForceConflicts)

	deleted, err := resolveReleaseLifecycle(input, releaseOperationDelete, false)
	require.NoError(t, err)
	assert.Equal(t, serverSideApplyUnset, deleted.Policy.ServerSideApply)
	assert.False(t, deleted.Policy.ForceConflicts)
}

// TestResolveServerSideApplyPerPhaseOverride confirms a per-phase block overrides
// the release-wide default for just that operation.
func TestResolveServerSideApplyPerPhaseOverride(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{
		cfg.HelmReleaseSectionName: map[string]any{
			cfg.HelmServerSideApplySectionName: "auto",
			cfg.HelmInstallSectionName: map[string]any{
				cfg.HelmServerSideApplySectionName: false,
			},
			cfg.HelmUpgradeSectionName: map[string]any{
				cfg.HelmServerSideApplySectionName: true,
				cfg.HelmForceConflictsSectionName:  true,
			},
		},
	})
	require.NoError(t, err)

	install, err := resolveReleaseLifecycle(input, releaseOperationInstall, false)
	require.NoError(t, err)
	assert.Equal(t, serverSideApplyFalse, install.Policy.ServerSideApply)
	assert.False(t, install.Policy.ForceConflicts)

	upgrade, err := resolveReleaseLifecycle(input, releaseOperationUpgrade, false)
	require.NoError(t, err)
	assert.Equal(t, serverSideApplyTrue, upgrade.Policy.ServerSideApply)
	assert.True(t, upgrade.Policy.ForceConflicts)
}

// TestResolveServerSideApplyFlagPrecedence confirms CLI flags beat configuration.
func TestResolveServerSideApplyFlagPrecedence(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{
		cfg.HelmReleaseSectionName: map[string]any{
			cfg.HelmServerSideApplySectionName: "auto",
			cfg.HelmForceConflictsSectionName:  false,
		},
	})
	require.NoError(t, err)

	resolution, err := resolveReleaseLifecycleWithFlags(input, releaseOperationInstall, map[string]any{
		cfg.HelmServerSideApplySectionName: "false",
		cfg.HelmForceConflictsSectionName:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, serverSideApplyFalse, resolution.Policy.ServerSideApply)
	assert.True(t, resolution.Policy.ForceConflicts)
}

func TestResolveServerSideApplyInvalidFlag(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{})
	require.NoError(t, err)

	_, err = resolveReleaseLifecycleWithFlags(input, releaseOperationUpgrade, map[string]any{
		cfg.HelmServerSideApplySectionName: "nope",
	})
	require.ErrorIs(t, err, errUtils.ErrHelmServerSideApplyInvalid)
}

// TestServerSideApplyFlagsInapplicableOnDelete confirms the apply-method flags are
// rejected for the delete operation, which has no server-side apply surface.
func TestServerSideApplyFlagsInapplicableOnDelete(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{})
	require.NoError(t, err)

	for _, flags := range []map[string]any{
		{cfg.HelmServerSideApplySectionName: "true"},
		{cfg.HelmForceConflictsSectionName: true},
	} {
		_, err := resolveReleaseLifecycleWithFlags(input, releaseOperationDelete, flags)
		require.ErrorIs(t, err, errUtils.ErrHelmLifecycleFlagInapplicable)
	}
}

// TestDecodeServerSideApplyShapes confirms a YAML boolean and the string "auto"
// both decode, normalizing the boolean to its string form.
func TestDecodeServerSideApplyShapes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "yaml true", value: true, want: "true"},
		{name: "yaml false", value: false, want: "false"},
		{name: "string auto", value: "auto", want: "auto"},
		{name: "string true", value: "true", want: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := decodeReleasePolicy(map[string]any{
				cfg.HelmReleaseSectionName: map[string]any{cfg.HelmServerSideApplySectionName: tt.value},
			})
			require.NoError(t, err)
			require.NotNil(t, input.ServerSideApply)
			assert.Equal(t, tt.want, *input.ServerSideApply)
		})
	}
}

func TestDecodeForceConflicts(t *testing.T) {
	input, err := decodeReleasePolicy(map[string]any{
		cfg.HelmReleaseSectionName: map[string]any{cfg.HelmForceConflictsSectionName: true},
	})
	require.NoError(t, err)
	require.NotNil(t, input.ForceConflicts)
	assert.True(t, *input.ForceConflicts)
}

func TestDecodeServerSideApplyValidation(t *testing.T) {
	tests := []struct {
		name    string
		release any
		wantErr error
	}{
		{name: "release invalid enum", release: map[string]any{cfg.HelmServerSideApplySectionName: "yes"}, wantErr: errUtils.ErrHelmServerSideApplyInvalid},
		{name: "release wrong type", release: map[string]any{cfg.HelmServerSideApplySectionName: 5}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "release force_conflicts wrong type", release: map[string]any{cfg.HelmForceConflictsSectionName: "yes"}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "install invalid enum", release: map[string]any{cfg.HelmInstallSectionName: map[string]any{cfg.HelmServerSideApplySectionName: "nope"}}, wantErr: errUtils.ErrHelmServerSideApplyInvalid},
		{name: "upgrade force_conflicts wrong type", release: map[string]any{cfg.HelmUpgradeSectionName: map[string]any{cfg.HelmForceConflictsSectionName: 1}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete rejects server_side_apply", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmServerSideApplySectionName: "true"}}, wantErr: errUtils.ErrHelmLifecycleDecode},
		{name: "delete rejects force_conflicts", release: map[string]any{cfg.HelmDeleteSectionName: map[string]any{cfg.HelmForceConflictsSectionName: true}}, wantErr: errUtils.ErrHelmLifecycleDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeReleasePolicy(map[string]any{cfg.HelmReleaseSectionName: tt.release})
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestConfigureInstallLifecycleServerSideApply confirms the install action gets a
// boolean apply method, and an unset policy leaves the Helm default untouched.
func TestConfigureInstallLifecycleServerSideApply(t *testing.T) {
	actx := memoryActionContext(t)

	t.Run("unset leaves default", func(t *testing.T) {
		client := action.NewInstall(actx.cfg)
		before := client.ServerSideApply
		configureInstallLifecycle(client, effectiveReleasePolicy{ServerSideApply: serverSideApplyUnset})
		assert.Equal(t, before, client.ServerSideApply)
		assert.False(t, client.ForceConflicts)
	})

	t.Run("false disables", func(t *testing.T) {
		client := action.NewInstall(actx.cfg)
		configureInstallLifecycle(client, effectiveReleasePolicy{ServerSideApply: serverSideApplyFalse})
		assert.False(t, client.ServerSideApply)
	})

	for _, policy := range []serverSideApplyPolicy{serverSideApplyAuto, serverSideApplyTrue} {
		t.Run("enables for "+string(policy), func(t *testing.T) {
			client := action.NewInstall(actx.cfg)
			configureInstallLifecycle(client, effectiveReleasePolicy{ServerSideApply: policy})
			assert.True(t, client.ServerSideApply)
		})
	}

	t.Run("force conflicts", func(t *testing.T) {
		client := action.NewInstall(actx.cfg)
		configureInstallLifecycle(client, effectiveReleasePolicy{ForceConflicts: true})
		assert.True(t, client.ForceConflicts)
	})
}

// TestConfigureUpgradeLifecycleServerSideApply confirms the upgrade action gets a
// string apply method (passed through), and an unset policy leaves the Helm
// default ("auto") untouched.
func TestConfigureUpgradeLifecycleServerSideApply(t *testing.T) {
	actx := memoryActionContext(t)

	t.Run("unset leaves default", func(t *testing.T) {
		client := action.NewUpgrade(actx.cfg)
		before := client.ServerSideApply
		configureUpgradeLifecycle(client, effectiveReleasePolicy{ServerSideApply: serverSideApplyUnset})
		assert.Equal(t, before, client.ServerSideApply)
		assert.False(t, client.ForceConflicts)
	})

	for _, policy := range []serverSideApplyPolicy{serverSideApplyAuto, serverSideApplyTrue, serverSideApplyFalse} {
		t.Run("passes through "+string(policy), func(t *testing.T) {
			client := action.NewUpgrade(actx.cfg)
			configureUpgradeLifecycle(client, effectiveReleasePolicy{ServerSideApply: policy})
			assert.Equal(t, string(policy), client.ServerSideApply)
		})
	}

	t.Run("force conflicts", func(t *testing.T) {
		client := action.NewUpgrade(actx.cfg)
		configureUpgradeLifecycle(client, effectiveReleasePolicy{ForceConflicts: true})
		assert.True(t, client.ForceConflicts)
	})
}
