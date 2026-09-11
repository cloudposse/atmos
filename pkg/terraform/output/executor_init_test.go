package output

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"
)

// autoInitComponentConfig returns a ComponentConfig pointed at a fresh, real
// temp directory (required so autoinit.Compute/Record can fingerprint and
// write a marker against real files) with the "auto" policy on every init
// knob, matching atmosConfig defaults when components.terraform.init is unset.
func autoInitComponentConfig(t *testing.T) *ComponentConfig {
	t.Helper()

	return &ComponentConfig{
		ComponentPath:   t.TempDir(),
		Executable:      "/usr/local/bin/terraform",
		Workspace:       "test-workspace",
		BackendType:     "s3",
		InitMode:        schema.TerraformInitModeAuto,
		InitReconfigure: schema.TerraformInitReconfigureAuto,
		InitUpgrade:     schema.TerraformInitUpgradeAuto,
	}
}

// --- ensureInitialized tests ---

func TestEnsureInitialized_SkipsInitWhenMarkerMatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}
	ctx := context.Background()

	// First call: no marker on disk yet, so init must run (and, since no
	// marker was ever recorded, Decide's conservative default reconfigures).
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil).Times(2)

	err := executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)

	// Second call with identical inputs: the marker autoinit.Record wrote after
	// the first call must match the freshly computed fingerprint, so Init must
	// NOT be called again. No EXPECT().Init is registered for this call — if
	// the executor called it anyway, gomock's strict mode would fail the test
	// immediately with "unexpected call", which is the proof this test wants.
	err = executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)
}

func TestEnsureInitialized_ForcesInitWhenWorkdirReprovisioned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}
	ctx := context.Background()

	// Init must run on both calls: the second call has an up-to-date marker
	// (same as the skip test above) but config.WorkdirReprovisioned forces
	// autoinit.Decide via Request.Force regardless.
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil).Times(2)

	err := executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)

	config.WorkdirReprovisioned = true
	err = executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)
}

func TestEnsureInitialized_InitModeAlwaysAlwaysInits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	config.InitMode = schema.TerraformInitModeAlways
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}
	ctx := context.Background()

	// mode: always means every call runs init, even with a matching marker.
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil).Times(2)

	err := executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)
	err = executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)
}

func TestEnsureInitialized_InitModeNeverSkipsInitButSelectsWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	config.InitMode = schema.TerraformInitModeNever
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}
	ctx := context.Background()

	// No EXPECT().Init: mode "never" must never call Init. Workspace selection
	// still must happen -- unlike SkipInit (the GetOutputSkipInit path), mode:
	// never still reaches the "else" branch of ensureInitialized, which always
	// calls EnsureWorkspace afterward.
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil).Times(1)

	err := executor.ensureInitialized(ctx, atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false)
	require.NoError(t, err)
}

func TestEnsureInitialized_SkipInitIsNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// skipInit=true must return immediately without calling Init OR
	// WorkspaceSelect/WorkspaceNew -- matching execute()'s pre-autoinit
	// behavior, where both were gated by the same `if !skipInit` block. No
	// mock expectations are registered at all, so any call fails the test.
	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	err := executor.ensureInitialized(context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, true)
	require.NoError(t, err)
}

// --- runOutputWithInitRecovery tests ---

func outputMetaFor(value string) map[string]tfexec.OutputMeta {
	return map[string]tfexec.OutputMeta{"result": {Value: []byte(value)}}
}

func TestRunOutput_RecoversFromBackendInitRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	first := mockRunner.EXPECT().Output(gomock.Any()).
		Return(nil, errors.New(`Backend initialization required, please run "terraform init"`))
	second := mockRunner.EXPECT().Output(gomock.Any()).Return(outputMetaFor(`"recovered"`), nil)
	gomock.InOrder(first, second)

	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)

	outputs, err := executor.runOutputWithInitRecovery(
		context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false,
	)
	require.NoError(t, err)
	assert.Equal(t, `"recovered"`, string(outputs["result"].Value))
}

func TestRunOutput_RecoversWithUpgradeWhenDiagnosticAsks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	first := mockRunner.EXPECT().Output(gomock.Any()).
		Return(nil, errors.New("Error: Inconsistent dependency lock file\n\nmust use terraform init -upgrade"))
	second := mockRunner.EXPECT().Output(gomock.Any()).Return(outputMetaFor(`"recovered"`), nil)
	gomock.InOrder(first, second)

	var initOpts []tfexec.InitOption
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts ...tfexec.InitOption) error {
			initOpts = opts
			return nil
		},
	)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)

	_, err := executor.runOutputWithInitRecovery(
		context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false,
	)
	require.NoError(t, err)
	require.Len(t, initOpts, 1, "an upgrade-only recovery must not add -reconfigure")
	assert.Equal(t, "&{upgrade:true}", initOptionString(initOpts[0]))
}

func TestRunOutput_RecoversWithReconfigureOnBackendChanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	first := mockRunner.EXPECT().Output(gomock.Any()).
		Return(nil, errors.New("Error: Backend configuration changed"))
	second := mockRunner.EXPECT().Output(gomock.Any()).Return(outputMetaFor(`"recovered"`), nil)
	gomock.InOrder(first, second)

	var initOpts []tfexec.InitOption
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts ...tfexec.InitOption) error {
			initOpts = opts
			return nil
		},
	)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)

	_, err := executor.runOutputWithInitRecovery(
		context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false,
	)
	require.NoError(t, err)
	require.Len(t, initOpts, 2, "a backend-changed recovery must add both -reconfigure and the base -upgrade=false option")
	assert.Contains(t, []string{initOptionString(initOpts[0]), initOptionString(initOpts[1])}, "&{reconfigure:true}")
}

func TestRunOutput_NoRecoveryForUnrelatedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	unrelatedErr := errors.New("dial tcp: connection refused")
	// Exactly one Output call: no diagnostic match means no recovery attempt,
	// so a second Output call (and any Init/WorkspaceSelect call) would be
	// unexpected -- no mock expectations are registered for them.
	mockRunner.EXPECT().Output(gomock.Any()).Return(nil, unrelatedErr).Times(1)

	_, err := executor.runOutputWithInitRecovery(
		context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, unrelatedErr)
}

func TestRunOutput_NoRecoveryWhenSkipInit_ErrorWrapsInitRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	initRequiredErr := errors.New(`Backend initialization required, please run "terraform init"`)
	mockRunner.EXPECT().Output(gomock.Any()).Return(nil, initRequiredErr).Times(1)

	// skipInit=true means the caller explicitly opted out of implicit init
	// (the GetOutputSkipInit path); ShouldRecover must refuse to silently
	// re-run init and instead return a policy error.
	_, err := executor.runOutputWithInitRecovery(
		context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, true,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTerraformInitRequired)
	assert.ErrorIs(t, err, initRequiredErr, "the original terraform diagnostic must remain reachable via errors.Is")
}

func TestRunOutput_NoRecoveryWhenUpgradeNever_ErrorWrapsUpgradeRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	config := autoInitComponentConfig(t)
	config.InitUpgrade = schema.TerraformInitUpgradeNever
	atmosConfig := &schema.AtmosConfiguration{}
	executor := &Executor{}

	upgradeErr := errors.New("Error: Inconsistent dependency lock file\n\nmust use terraform init -upgrade")
	mockRunner.EXPECT().Output(gomock.Any()).Return(nil, upgradeErr).Times(1)

	_, err := executor.runOutputWithInitRecovery(
		context.Background(), atmosConfig, mockRunner, config, "comp", "stack", nil, tfplugin.Cache{}, map[string]string{}, false,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTerraformInitUpgradeRequired)
	assert.ErrorIs(t, err, upgradeErr, "the original terraform diagnostic must remain reachable via errors.Is")
}
