package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/degradation"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ──────────────────────────────────────────────────────────────────────────────
// resolveTerraformCommand
// ──────────────────────────────────────────────────────────────────────────────

func TestResolveTerraformCommand_DefaultsToTerraform(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	resolveTerraformCommand(&atmosConfig, &info)

	assert.Equal(t, cfg.TerraformComponentType, info.Command)
}

func TestResolveTerraformCommand_UsesAtmosConfigCommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Command = "tofu"
	info := schema.ConfigAndStacksInfo{}

	resolveTerraformCommand(&atmosConfig, &info)

	assert.Equal(t, "tofu", info.Command)
}

func TestResolveTerraformCommand_DoesNotOverrideExistingCommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Command = "tofu"
	info := schema.ConfigAndStacksInfo{Command: "my-terraform"}

	resolveTerraformCommand(&atmosConfig, &info)

	// Should not overwrite an already-set command.
	assert.Equal(t, "my-terraform", info.Command)
}

// ──────────────────────────────────────────────────────────────────────────────
// checkComponentRestrictions
// ──────────────────────────────────────────────────────────────────────────────

func TestCheckComponentRestrictions_NoRestrictions(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	err := checkComponentRestrictions(&info)
	assert.NoError(t, err)
}

func TestCheckComponentRestrictions_AbstractComponentPlan(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:          "plan",
		ComponentIsAbstract: true,
		Component:           "my-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAbstractComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_AbstractComponentApply(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:          "apply",
		ComponentIsAbstract: true,
		Component:           "my-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAbstractComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_AbstractComponentDeploy(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:          "deploy",
		ComponentIsAbstract: true,
		Component:           "my-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAbstractComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_AbstractComponentWorkspace(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:          "workspace",
		ComponentIsAbstract: true,
		Component:           "my-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAbstractComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_AbstractComponentOutput_AllowedCommands(t *testing.T) {
	// Abstract components should be allowed for read-only commands.
	for _, subCmd := range []string{"output", "show", "validate", "state"} {
		t.Run(subCmd, func(t *testing.T) {
			info := schema.ConfigAndStacksInfo{
				SubCommand:          subCmd,
				ComponentIsAbstract: true,
				Component:           "my-component",
			}
			err := checkComponentRestrictions(&info)
			assert.NoError(t, err, "abstract component should be allowed for %s", subCmd)
		})
	}
}

func TestCheckComponentRestrictions_LockedComponentApply(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:        "apply",
		ComponentIsLocked: true,
		Component:         "my-locked-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockedComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_LockedComponentDestroy(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:        "destroy",
		ComponentIsLocked: true,
		Component:         "my-locked-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockedComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_LockedComponentPlan_Allowed(t *testing.T) {
	// Read-only commands should be allowed even for locked components.
	info := schema.ConfigAndStacksInfo{
		SubCommand:        "plan",
		ComponentIsLocked: true,
		Component:         "my-locked-component",
	}
	err := checkComponentRestrictions(&info)
	assert.NoError(t, err)
}

func TestCheckComponentRestrictions_LockedComponentAllMutatingSubcommands(t *testing.T) {
	mutatingCmds := []string{"apply", "deploy", "destroy", "import", "state", "taint", "untaint"}
	for _, subCmd := range mutatingCmds {
		t.Run(subCmd, func(t *testing.T) {
			info := schema.ConfigAndStacksInfo{
				SubCommand:        subCmd,
				ComponentIsLocked: true,
				Component:         "my-locked-component",
			}
			err := checkComponentRestrictions(&info)
			require.Error(t, err, "locked component should not allow %s", subCmd)
			assert.ErrorIs(t, err, errUtils.ErrLockedComponentCantBeProvisioned)
		})
	}
}

func TestCheckComponentRestrictions_WorkspaceWithHTTPBackend(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "workspace",
		ComponentBackendType: "http",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrHTTPBackendWorkspaces)
}

func TestCheckComponentRestrictions_WorkspaceWithS3Backend_Allowed(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "workspace",
		ComponentBackendType: "s3",
	}
	err := checkComponentRestrictions(&info)
	assert.NoError(t, err)
}

func TestCheckComponentRestrictions_FolderPrefixInErrorMessage(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:            "plan",
		ComponentIsAbstract:   true,
		ComponentFolderPrefix: "infra/networking",
		Component:             "vpc",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	expectedPath := filepath.Join("infra", "networking", "vpc")
	assert.Contains(t, err.Error(), expectedPath)
}

// ──────────────────────────────────────────────────────────────────────────────
// shouldRunTerraformInit
// ──────────────────────────────────────────────────────────────────────────────

func TestShouldRunTerraformInit_TrueForPlan(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	assert.True(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_TrueForApply(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "apply"}
	assert.True(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_FalseForInitSubCommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "init"}
	assert.False(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_FalseForDeployWithoutDeployRunInit(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.DeployRunInit = false
	info := schema.ConfigAndStacksInfo{SubCommand: "deploy"}
	assert.False(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_TrueForDeployWithDeployRunInit(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.DeployRunInit = true
	info := schema.ConfigAndStacksInfo{SubCommand: "deploy"}
	assert.True(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_FalseWhenSkipInitSet(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan", SkipInit: true}
	assert.False(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_FalseWhenSkipInitOverridesDeployRunInit(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.DeployRunInit = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan", SkipInit: true}
	assert.False(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_FalseWhenModeNever(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.Mode = schema.TerraformInitModeNever
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	assert.False(t, shouldRunTerraformInit(&atmosConfig, &info))
}

func TestShouldRunTerraformInit_TrueWhenModeAuto(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.Mode = schema.TerraformInitModeAuto
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	assert.True(t, shouldRunTerraformInit(&atmosConfig, &info))
}

// ──────────────────────────────────────────────────────────────────────────────
// buildInitArgs
// ──────────────────────────────────────────────────────────────────────────────
//
// buildInitArgs itself is a thin, decision-driven arg assembler: init, then -reconfigure when
// decision.Reconfigure, then -upgrade when decision.Upgrade, then -var-file when Init.PassVars.
// These tests exercise that assembly directly against hand-built autoinit.Decision values,
// rather than re-deriving the decision (that policy is pkg/terraform/autoinit's own contract,
// covered by its own tests). The former workdir-specific reconfigure carve-out is exercised at
// the decideAutoInit level instead (TestDecideAutoInit_* below), since decision.Reconfigure now
// carries that rule for every component via the backend fingerprint comparison.

func TestBuildInitArgs_BasicInit(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json", autoinit.Decision{})
	assert.Equal(t, []string{"init"}, args)
}

func TestBuildInitArgs_Reconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json", autoinit.Decision{Reconfigure: true})
	assert.Equal(t, []string{"init", "-reconfigure"}, args)
}

func TestBuildInitArgs_Upgrade(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json", autoinit.Decision{Upgrade: true})
	assert.Equal(t, []string{"init", "-upgrade"}, args)
}

func TestBuildInitArgs_ReconfigureAndUpgrade(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json", autoinit.Decision{Reconfigure: true, Upgrade: true})
	assert.Equal(t, []string{"init", "-reconfigure", "-upgrade"}, args)
}

func TestBuildInitArgs_PassVarsWithoutReconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json", autoinit.Decision{})
	assert.Equal(t, []string{"init", varFileFlag, "my-component.tfvars.json"}, args)
}

func TestBuildInitArgs_PassVarsWithReconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json", autoinit.Decision{Reconfigure: true})
	assert.Equal(t, []string{"init", "-reconfigure", varFileFlag, "my-component.tfvars.json"}, args)
}

func TestBuildInitArgs_PassVarsWithReconfigureAndUpgrade(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json", autoinit.Decision{Reconfigure: true, Upgrade: true})
	assert.Equal(t, []string{"init", "-reconfigure", "-upgrade", varFileFlag, "my-component.tfvars.json"}, args)
}

// ──────────────────────────────────────────────────────────────────────────────
// decideAutoInit
// ──────────────────────────────────────────────────────────────────────────────

// TestDecideAutoInit_ForcedForWorkspaceSubcommand verifies that the workspace subcommand
// always forces RunInit=true (ReasonForced) regardless of the fingerprint -- workspace
// operations need a clean state on each run. (decision.Reconfigure's own policy nuances are
// pkg/terraform/autoinit's contract, covered by its own tests -- this only checks that
// decideAutoInit feeds the workspace-subcommand force signal into the Request at all.)
func TestDecideAutoInit_ForcedForWorkspaceSubcommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace"}
	decision := decideAutoInit(&atmosConfig, &info, nil, false)
	assert.True(t, decision.RunInit)
	assert.Equal(t, autoinit.ReasonForced, decision.Reason)
}

// TestDecideAutoInit_ForcedForReprovisionedWorkdir verifies that a re-provisioned workdir
// (WorkdirReprovisionedKey set by the source/workdir provisioner) forces RunInit=true
// (ReasonForced), since any prior init marker belonged to the wiped directory. This is the
// decision-level replacement for the former buildInitArgs workdir carve-out: it feeds exactly
// the same signal the old code checked (WorkdirReprovisionedKey) into autoinit.Decide instead.
func TestDecideAutoInit_ForcedForReprovisionedWorkdir(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey:          "/tmp/.workdir/terraform/demo-consumer",
			provWorkdir.WorkdirReprovisionedKey: struct{}{},
		},
	}
	decision := decideAutoInit(&atmosConfig, &info, nil, false)
	assert.True(t, decision.RunInit)
	assert.Equal(t, autoinit.ReasonForced, decision.Reason)
}

// TestDecideAutoInit_NotForcedForPreservedWorkdir verifies that a preserved (not
// re-provisioned) workdir does NOT force init on its own -- only the explicit force
// parameter, the workspace subcommand, or WorkdirReprovisionedKey do.
func TestDecideAutoInit_NotForcedForPreservedWorkdir(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "/tmp/.workdir/terraform/demo-consumer",
			// WorkdirReprovisionedKey intentionally absent.
		},
	}
	decision := decideAutoInit(&atmosConfig, &info, nil, false)
	// nil Inputs (dry run / no fingerprint target) always runs init (ReasonNoInputs) --
	// that's autoinit's own contract, not a force signal from decideAutoInit.
	assert.Equal(t, autoinit.ReasonNoInputs, decision.Reason)
}

// TestDecideAutoInit_ExplicitForceParameter verifies that the force parameter (used by
// buildInitSubcommandArgs for an explicit `atmos terraform init`) forces RunInit regardless of
// SubCommand or ComponentSection.
func TestDecideAutoInit_ExplicitForceParameter(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "init"}
	decision := decideAutoInit(&atmosConfig, &info, nil, true)
	assert.True(t, decision.RunInit)
	assert.Equal(t, autoinit.ReasonForced, decision.Reason)
}

// ──────────────────────────────────────────────────────────────────────────────
// prepareInitExecution — workspace file cleanup behaviour
// ──────────────────────────────────────────────────────────────────────────────

// TestPrepareInitExecution_SkipsCleanWorkspaceForWorkdir verifies that
// .terraform/environment is NOT deleted for workdir-enabled components.
// Deleting the file before init -reconfigure causes OpenTofu to prompt
// "Do you want to migrate all workspaces?" because it sees workspace state
// directories (terraform.tfstate.d/) but no active workspace recorded.
// For workdir components the backend is always consistent so cleanup is wrong.
func TestPrepareInitExecution_SkipsCleanWorkspaceForWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	tfDir := filepath.Join(tmpDir, ".terraform")
	require.NoError(t, os.MkdirAll(tfDir, 0o755))
	envFile := filepath.Join(tfDir, "environment")
	require.NoError(t, os.WriteFile(envFile, []byte("myworkspace"), 0o644))

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: tmpDir,
		},
	}

	_, err := prepareInitExecution(t.Context(), provisioner.OutputWriters{}, &atmosConfig, &info, tmpDir)
	require.NoError(t, err)

	_, statErr := os.Stat(envFile)
	assert.NoError(t, statErr, ".terraform/environment must not be deleted for workdir components")
}

// TestPrepareInitExecution_CleansWorkspaceForNonWorkdir verifies that the standard
// .terraform/environment cleanup still runs for non-workdir components.
func TestPrepareInitExecution_CleansWorkspaceForNonWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	tfDir := filepath.Join(tmpDir, ".terraform")
	require.NoError(t, os.MkdirAll(tfDir, 0o755))
	envFile := filepath.Join(tfDir, "environment")
	require.NoError(t, os.WriteFile(envFile, []byte("myworkspace"), 0o644))

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{}, // no WorkdirPathKey
	}

	_, err := prepareInitExecution(t.Context(), provisioner.OutputWriters{}, &atmosConfig, &info, tmpDir)
	require.NoError(t, err)

	_, statErr := os.Stat(envFile)
	assert.True(t, os.IsNotExist(statErr), ".terraform/environment must be deleted for non-workdir components")
}

func TestPrepareInitExecutionPropagatesOutputSuppression(t *testing.T) {
	originalExecuteProvisioners := executeBeforeInitProvisioners
	t.Cleanup(func() { executeBeforeInitProvisioners = originalExecuteProvisioners })
	var observedSuppression atomic.Bool
	executeBeforeInitProvisioners = func(ctx context.Context, _ provisioner.HookEvent, _ *schema.AtmosConfiguration, _ map[string]any, _ *schema.AuthContext, _ provisioner.OutputWriters, _ ...*provisioner.TerraformExecContext) error {
		observedSuppression.Store(provWorkdir.OutputSuppressed(ctx))
		return nil
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}

	_, err := prepareInitExecution(provWorkdir.WithOutputSuppressed(t.Context()), provisioner.OutputWriters{}, &atmosConfig, &info, t.TempDir())
	require.NoError(t, err)
	require.True(t, observedSuppression.Load())
}

func TestResolveAndProvisionComponentPathPropagatesOutputSuppression(t *testing.T) {
	originalProvisioner := provisionAndResolveTerraformComponentPath
	t.Cleanup(func() { provisionAndResolveTerraformComponentPath = originalProvisioner })

	var observedSuppression atomic.Bool
	provisionAndResolveTerraformComponentPath = func(ctx context.Context, _ provisioner.OutputWriters, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ string, componentPath string) (string, bool, error) {
		observedSuppression.Store(provWorkdir.OutputSuppressed(ctx))
		return componentPath, true, nil
	}

	atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir()}
	info := schema.ConfigAndStacksInfo{FinalComponent: "component"}

	_, err := resolveAndProvisionComponentPath(provWorkdir.WithOutputSuppressed(t.Context()), provisioner.OutputWriters{}, &atmosConfig, &info)
	require.NoError(t, err)
	require.True(t, observedSuppression.Load())
}

// ──────────────────────────────────────────────────────────────────────────────
// handleDeploySubcommand
// ──────────────────────────────────────────────────────────────────────────────

func TestHandleDeploySubcommand_ConvertsDeploy(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "deploy"}
	handleDeploySubcommand(&atmosConfig, &info)
	assert.Equal(t, "apply", info.SubCommand)
}

func TestHandleDeploySubcommand_AddsAutoApproveForDeploy(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "deploy"}
	handleDeploySubcommand(&atmosConfig, &info)
	assert.Contains(t, info.AdditionalArgsAndFlags, autoApproveFlag)
}

func TestHandleDeploySubcommand_NoAutoApproveWhenPlanFileSet(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "deploy",
		UseTerraformPlan: true,
	}
	handleDeploySubcommand(&atmosConfig, &info)
	assert.NotContains(t, info.AdditionalArgsAndFlags, autoApproveFlag)
}

func TestHandleDeploySubcommand_NoAutoApproveWhenAlreadySet(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:             "deploy",
		AdditionalArgsAndFlags: []string{autoApproveFlag},
	}
	handleDeploySubcommand(&atmosConfig, &info)
	// Should not add a duplicate.
	count := 0
	for _, f := range info.AdditionalArgsAndFlags {
		if f == autoApproveFlag {
			count++
		}
	}
	assert.Equal(t, 1, count, "should not add duplicate -auto-approve flag")
}

func TestHandleDeploySubcommand_ApplyAutoApproveFromConfig(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.ApplyAutoApprove = true
	info := schema.ConfigAndStacksInfo{SubCommand: "apply"}
	handleDeploySubcommand(&atmosConfig, &info)
	assert.Contains(t, info.AdditionalArgsAndFlags, autoApproveFlag)
}

func TestHandleDeploySubcommand_ApplyAutoApproveNotAddedWhenPlanFile(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.ApplyAutoApprove = true
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		UseTerraformPlan: true,
	}
	handleDeploySubcommand(&atmosConfig, &info)
	assert.NotContains(t, info.AdditionalArgsAndFlags, autoApproveFlag)
}

func TestHandleDeploySubcommand_NonDeploySubcommandUnchanged(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	handleDeploySubcommand(&atmosConfig, &info)
	assert.Equal(t, "plan", info.SubCommand)
	assert.Empty(t, info.AdditionalArgsAndFlags)
}

// ──────────────────────────────────────────────────────────────────────────────
// resolveExitCode
// ──────────────────────────────────────────────────────────────────────────────

func TestResolveExitCode_NilErrReturnsZero(t *testing.T) {
	assert.Equal(t, 0, resolveExitCode(nil))
}

func TestResolveExitCode_ExitCodeErrorReturnsCode(t *testing.T) {
	err := errUtils.ExitCodeError{Code: 42}
	assert.Equal(t, 42, resolveExitCode(err))
}

func TestResolveExitCode_WrappedExitCodeErrorReturnsCode(t *testing.T) {
	inner := errUtils.ExitCodeError{Code: 5}
	err := fmt.Errorf("wrapper: %w", inner)
	assert.Equal(t, 5, resolveExitCode(err))
}

func TestResolveExitCode_OsExecExitError(t *testing.T) {
	// Create a real *exec.ExitError using a cross-platform approach:
	// "go" binary exists on all platforms where tests run, and "go run nonexistent.go"
	// exits with code 1.
	cmd := osexec.Command("go", "run", "nonexistent_file_that_does_not_exist.go")
	runErr := cmd.Run()
	require.Error(t, runErr)

	code := resolveExitCode(runErr)
	assert.NotEqual(t, 0, code, "exit code should be non-zero for a failed command")
}

func TestResolveExitCode_GenericErrorReturnsOne(t *testing.T) {
	err := errors.New("some generic error")
	assert.Equal(t, 1, resolveExitCode(err))
}

// ──────────────────────────────────────────────────────────────────────────────
// checkTTYRequirement
// ──────────────────────────────────────────────────────────────────────────────

func TestCheckTTYRequirement_NonApplySubCommandNoError(t *testing.T) {
	// stdin is always non-nil in test processes, so only test the SubCommand branch.
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	err := checkTTYRequirement(&info)
	assert.NoError(t, err)
}

func TestCheckTTYRequirement_ApplyWithAutoApproveNoError(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:             "apply",
		AdditionalArgsAndFlags: []string{autoApproveFlag},
	}
	err := checkTTYRequirement(&info)
	assert.NoError(t, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// addRegionEnvVarForImport
// ──────────────────────────────────────────────────────────────────────────────

func TestAddRegionEnvVarForImport_AddsRegionForImport(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand: "import",
		ComponentVarsSection: map[string]any{
			"region": "us-east-1",
		},
	}
	addRegionEnvVarForImport(&info)
	assert.Contains(t, info.ComponentEnvList, "AWS_REGION=us-east-1")
}

func TestAddRegionEnvVarForImport_NoRegionVarNoChange(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "import",
		ComponentVarsSection: map[string]any{},
	}
	addRegionEnvVarForImport(&info)
	assert.Empty(t, info.ComponentEnvList)
}

func TestAddRegionEnvVarForImport_SkipsForNonImportSubcommands(t *testing.T) {
	for _, subCmd := range []string{"plan", "apply", "destroy", "workspace"} {
		t.Run(subCmd, func(t *testing.T) {
			info := schema.ConfigAndStacksInfo{
				SubCommand: subCmd,
				ComponentVarsSection: map[string]any{
					"region": "eu-west-1",
				},
			}
			addRegionEnvVarForImport(&info)
			assert.Empty(t, info.ComponentEnvList, "should not add AWS_REGION for %s", subCmd)
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Secret variables never hit disk (computeTerraformSecretVarKeys / diskSafeVars / secretVarEnv)
// ──────────────────────────────────────────────────────────────────────────────

func TestTerraformSecretVars_NeverHitDisk(t *testing.T) {
	const secret = "tf-disk-guard-SECRET-abc123xyz"
	atmosio.RegisterSecret(secret)

	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{
			"plain_password": secret,
			"db_url":         "postgres://user:" + secret + "@db.example.com/app",
			"nested":         map[string]any{"token": secret},
			"region":         "us-east-1-tfdiskguard",
		},
	}

	computeTerraformSecretVarKeys(info)

	// Secret-bearing keys (direct, composed substring, nested) are flagged; the plain
	// non-secret one is not.
	require.NotNil(t, info.TerraformSecretVarKeys)
	assert.True(t, info.TerraformSecretVarKeys["plain_password"], "direct secret must be flagged")
	assert.True(t, info.TerraformSecretVarKeys["db_url"], "secret composed into a string must be flagged")
	assert.True(t, info.TerraformSecretVarKeys["nested"], "secret nested in a map must be flagged")
	assert.False(t, info.TerraformSecretVarKeys["region"], "non-secret var must not be flagged")

	// Write the disk-safe vars exactly as logAndWriteComponentVars does, then assert the
	// bytes on disk contain no representation of the secret.
	dir := t.TempDir()
	varFile := filepath.Join(dir, "test.terraform.tfvars.json")
	require.NoError(t, u.WriteToFileAsJSON(varFile, diskSafeVars(info), 0o600))

	onDisk, err := os.ReadFile(varFile)
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), secret, "secret must never be written to the varfile on disk")
	assert.Contains(t, string(onDisk), "us-east-1-tfdiskguard", "non-secret vars must still be written")

	// The secret-bearing vars are instead injected as TF_VAR_* env entries.
	env, err := secretVarEnv(info)
	require.NoError(t, err)
	joined := strings.Join(env, "\n")
	assert.Contains(t, joined, "TF_VAR_plain_password=")
	assert.Contains(t, joined, "TF_VAR_db_url=")
	assert.Contains(t, joined, secret, "secret value is carried in the TF_VAR_ env entry")
}

func TestTerraformSecretVars_NoSecrets(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{"region": "us-west-2-nosecret-xyz", "count": 1},
	}
	computeTerraformSecretVarKeys(info)
	assert.Nil(t, info.TerraformSecretVarKeys, "no secret keys expected when no secrets present")

	env, err := secretVarEnv(info)
	require.NoError(t, err)
	assert.Empty(t, env, "no TF_VAR_ entries expected when no secrets present")
}

func TestTerraformSensitiveDeclaredVars_NeverHitDisk(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	require.NoError(t, atmosio.Initialize())

	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{
			"sensitive_value": "ordinary-resolved-value",
			"region":          "us-east-1-sensitive-declaration",
		},
		ComponentSection: map[string]any{
			componentInfoKey: map[string]any{
				terraformConfigKey: &tfconfig.Module{Variables: map[string]*tfconfig.Variable{
					"sensitive_value": {Name: "sensitive_value", Sensitive: true},
					"not_supplied":    {Name: "not_supplied", Sensitive: true},
					"region":          {Name: "region", Sensitive: false},
				}},
			},
		},
	}

	computeTerraformSecretVarKeys(info)
	require.True(t, info.TerraformSecretVarKeys["sensitive_value"])
	assert.False(t, info.TerraformSecretVarKeys["region"])
	assert.NotContains(t, diskSafeVars(info), "sensitive_value")
	assert.Equal(t, "us-east-1-sensitive-declaration", diskSafeVars(info)["region"])
	assert.NotContains(t, varfileVarsToWrite(info, true, "test.tfvars.json"), "sensitive_value")
	assert.Equal(t, "us-east-1-sensitive-declaration", varfileVarsToWrite(info, true, "test.tfvars.json")["region"])

	env, err := secretVarEnv(info)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(env, "\n"), "TF_VAR_sensitive_value=ordinary-resolved-value")
}

func TestTerraformSensitiveDeclaredVars_MaskingDisabledPreservesJSONCompatibility(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	require.NoError(t, atmosio.Initialize())
	atmosio.GetContext().Masker().SetEnabled(false)

	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{"sensitive_value": map[string]any{"id": "typed-value"}},
		ComponentSection: map[string]any{
			componentInfoKey: map[string]any{
				terraformConfigKey: &tfconfig.Module{Variables: map[string]*tfconfig.Variable{
					"sensitive_value": {Name: "sensitive_value", Sensitive: true},
				}},
			},
		},
	}

	computeTerraformSecretVarKeys(info)
	assert.Nil(t, info.TerraformSecretVarKeys)
	assert.Equal(t, info.ComponentVarsSection, diskSafeVars(info))
}

func TestRejectComputedTerraformVars(t *testing.T) {
	assert.NoError(t, rejectComputedTerraformVars(map[string]any{"region": "us-east-1"}))
	err := rejectComputedTerraformVars(map[string]any{"value": degradation.AtmosComputedValue{}})
	assert.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
	err = rejectComputedTerraformVars(map[string]any{
		"nested": []any{map[string]any{"value": degradation.AtmosComputedValue{}}},
	})
	assert.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
}

func TestHandleDeploySubcommand(t *testing.T) {
	tests := []struct {
		name             string
		subCommand       string
		useTerraformPlan bool
		applyAutoApprove bool
		initialArgs      []string
		wantSubCommand   string
		wantArgs         []string
	}{
		{
			name:           "deploy rewrites to apply and adds auto-approve",
			subCommand:     subcommandDeploy,
			initialArgs:    []string{},
			wantSubCommand: subcommandApply,
			wantArgs:       []string{autoApproveFlag},
		},
		{
			name:             "deploy with UseTerraformPlan does not add auto-approve",
			subCommand:       subcommandDeploy,
			useTerraformPlan: true,
			initialArgs:      []string{},
			wantSubCommand:   subcommandApply,
			wantArgs:         []string{},
		},
		{
			name:           "deploy does not duplicate an existing auto-approve flag",
			subCommand:     subcommandDeploy,
			initialArgs:    []string{autoApproveFlag},
			wantSubCommand: subcommandApply,
			wantArgs:       []string{autoApproveFlag},
		},
		{
			name:             "apply with ApplyAutoApprove adds auto-approve",
			subCommand:       subcommandApply,
			applyAutoApprove: true,
			initialArgs:      []string{},
			wantSubCommand:   subcommandApply,
			wantArgs:         []string{autoApproveFlag},
		},
		{
			name:             "apply with ApplyAutoApprove does not duplicate auto-approve",
			subCommand:       subcommandApply,
			applyAutoApprove: true,
			initialArgs:      []string{autoApproveFlag},
			wantSubCommand:   subcommandApply,
			wantArgs:         []string{autoApproveFlag},
		},
		{
			name:             "apply with ApplyAutoApprove and UseTerraformPlan does not add auto-approve",
			subCommand:       subcommandApply,
			applyAutoApprove: true,
			useTerraformPlan: true,
			initialArgs:      []string{},
			wantSubCommand:   subcommandApply,
			wantArgs:         []string{},
		},
		{
			name:           "plan is left untouched",
			subCommand:     "plan",
			initialArgs:    []string{},
			wantSubCommand: "plan",
			wantArgs:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.Components.Terraform.ApplyAutoApprove = tt.applyAutoApprove
			info := &schema.ConfigAndStacksInfo{
				SubCommand:             tt.subCommand,
				UseTerraformPlan:       tt.useTerraformPlan,
				AdditionalArgsAndFlags: tt.initialArgs,
			}

			handleDeploySubcommand(atmosConfig, info)

			assert.Equal(t, tt.wantSubCommand, info.SubCommand)
			assert.Equal(t, tt.wantArgs, info.AdditionalArgsAndFlags)
		})
	}
}
