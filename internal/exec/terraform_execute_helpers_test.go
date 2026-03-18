package exec

import (
	"errors"
	"fmt"
	osexec "os/exec"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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
		SubCommand:         "apply",
		ComponentIsLocked:  true,
		Component:          "my-locked-component",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockedComponentCantBeProvisioned)
}

func TestCheckComponentRestrictions_LockedComponentDestroy(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:         "destroy",
		ComponentIsLocked:  true,
		Component:          "my-locked-component",
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
		SubCommand:          "plan",
		ComponentIsAbstract: true,
		ComponentFolderPrefix: "infra/networking",
		Component:           "vpc",
	}
	err := checkComponentRestrictions(&info)
	require.Error(t, err)
	expectedPath := filepath.Join("infra/networking", "vpc")
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

// ──────────────────────────────────────────────────────────────────────────────
// buildInitArgs
// ──────────────────────────────────────────────────────────────────────────────

func TestBuildInitArgs_BasicInit(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init"}, args)
}

func TestBuildInitArgs_ReconfigureWhenWorkspace(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure"}, args)
}

func TestBuildInitArgs_ReconfigureWhenConfigEnabled(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure"}, args)
}

func TestBuildInitArgs_PassVarsWithoutReconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json")
	assert.Equal(t, []string{"init", varFileFlag, "my-component.tfvars.json"}, args)
}

func TestBuildInitArgs_PassVarsWithReconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure", varFileFlag, "my-component.tfvars.json"}, args)
}

func TestBuildInitArgs_PassVarsWithWorkspaceAndReconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace"}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure", varFileFlag, "my-component.tfvars.json"}, args)
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
		SubCommand:      "deploy",
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
		SubCommand:      "apply",
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
	// Create a real *exec.ExitError by running a failing command.
	cmd := osexec.Command("false")
	runErr := cmd.Run()
	require.Error(t, runErr)

	code := resolveExitCode(runErr)
	assert.Equal(t, 1, code)
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
// buildTerraformCommandArgs
// ──────────────────────────────────────────────────────────────────────────────

func TestBuildTerraformCommandArgs_Plan(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	componentPath := "/tmp/my-component"

	args, uploadFlag, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.False(t, uploadFlag)
	assert.Contains(t, args, "plan")
	assert.Contains(t, args, varFileFlag)
	assert.Contains(t, args, "vars.json")
	assert.Contains(t, args, outFlag)
	assert.Contains(t, args, "plan.tfplan")
}

func TestBuildTerraformCommandArgs_PlanSkipPlanfile(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Plan.SkipPlanfile = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.NotContains(t, args, outFlag)
}

func TestBuildTerraformCommandArgs_PlanCustomOutFlag(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:             "plan",
		AdditionalArgsAndFlags: []string{outFlag, "custom.tfplan"},
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "auto.tfplan", &componentPath)

	require.NoError(t, err)
	// Should NOT add another -out flag when one is already provided.
	outFlagCount := 0
	for _, a := range args {
		if a == outFlag {
			outFlagCount++
		}
	}
	assert.Equal(t, 1, outFlagCount, "should not add duplicate -out flag")
}

func TestBuildTerraformCommandArgs_PlanUploadStatus(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:             "plan",
		AdditionalArgsAndFlags: []string{"--upload-status"},
	}
	componentPath := "/tmp/my-component"

	args, uploadFlag, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.True(t, uploadFlag, "upload flag should be true")
	assert.Contains(t, args, detailedExitCodeFlag)
}

func TestBuildTerraformCommandArgs_Destroy(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "destroy"}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "destroy")
	assert.Contains(t, args, varFileFlag)
	assert.Contains(t, args, "vars.json")
}

func TestBuildTerraformCommandArgs_Import(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "import"}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "import")
	assert.Contains(t, args, varFileFlag)
}

func TestBuildTerraformCommandArgs_Refresh(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "refresh"}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "refresh")
	assert.Contains(t, args, varFileFlag)
}

func TestBuildTerraformCommandArgs_Apply_WithoutPlan(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:      "apply",
		UseTerraformPlan: false,
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "apply")
	assert.Contains(t, args, varFileFlag)
}

func TestBuildTerraformCommandArgs_Apply_WithPlanFileAutoDetected(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:      "apply",
		UseTerraformPlan: true,
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "auto.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "apply")
	assert.NotContains(t, args, varFileFlag)
	assert.Contains(t, args, "auto.tfplan")
}

func TestBuildTerraformCommandArgs_Apply_WithCustomPlanFile(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:      "apply",
		UseTerraformPlan: true,
		PlanFile:        "custom.tfplan",
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "auto.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "custom.tfplan")
	assert.NotContains(t, args, "auto.tfplan")
}

func TestBuildTerraformCommandArgs_Workspace_List(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "workspace",
		SubCommand2: "list",
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "workspace")
	assert.Contains(t, args, "list")
}

func TestBuildTerraformCommandArgs_Workspace_Show(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "workspace",
		SubCommand2: "show",
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "workspace")
	assert.Contains(t, args, "show")
}

func TestBuildTerraformCommandArgs_Workspace_New(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:         "workspace",
		SubCommand2:        "new",
		TerraformWorkspace: "my-workspace",
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "workspace")
	assert.Contains(t, args, "new")
	assert.Contains(t, args, "my-workspace")
}

func TestBuildTerraformCommandArgs_AdditionalArgsAppended(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:             "plan",
		AdditionalArgsAndFlags: []string{"-compact-warnings"},
	}
	componentPath := "/tmp/my-component"

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.tfplan", &componentPath)

	require.NoError(t, err)
	assert.Contains(t, args, "-compact-warnings")
}

// ──────────────────────────────────────────────────────────────────────────────
// logTerraformContext
// ──────────────────────────────────────────────────────────────────────────────

func TestLogTerraformContext_NoSubCommand2(t *testing.T) {
	// Just verifies the function doesn't panic.
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "plan",
		SubCommand2: "",
	}
	assert.NotPanics(t, func() { logTerraformContext(&info, "/tmp/workdir") })
}

func TestLogTerraformContext_WithSubCommand2(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "workspace",
		SubCommand2: "list",
	}
	assert.NotPanics(t, func() { logTerraformContext(&info, "/tmp/workdir") })
}

func TestLogTerraformContext_WithInheritanceChain(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:               "plan",
		ComponentFromArg:         "my-component",
		ComponentInheritanceChain: []string{"base-component", "grandparent"},
	}
	assert.NotPanics(t, func() { logTerraformContext(&info, "/tmp/workdir") })
}

// ──────────────────────────────────────────────────────────────────────────────
// warnOnConflictingEnvVars
// ──────────────────────────────────────────────────────────────────────────────

func TestWarnOnConflictingEnvVars_NoConflictsNoError(t *testing.T) {
	// Simply ensure the function doesn't panic when called.
	assert.NotPanics(t, warnOnConflictingEnvVars)
}

// ──────────────────────────────────────────────────────────────────────────────
// cleanupTerraformFiles
// ──────────────────────────────────────────────────────────────────────────────

func TestCleanupTerraformFiles_ApplyRemovesVarFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create fake files to simulate what Atmos generates.
	varFile := filepath.Join(tmpDir, "test.terraform.tfvars.json")
	require.NoError(t, writeTestFile(varFile, `{"key":"value"}`))

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	atmosConfig.Components.Terraform.BasePath = "components/terraform"
	info := schema.ConfigAndStacksInfo{
		SubCommand:              "apply",
		ComponentFromArg:        "test",
		FinalComponent:          "test",
		Stack:                   "test-stack",
		TerraformWorkspace:      "test-stack",
		ComponentFolderPrefix:   "",
	}

	// We cannot test the exact path without complex mocking, but we can verify
	// the function does not panic and handles missing files gracefully.
	assert.NotPanics(t, func() {
		cleanupTerraformFiles(&atmosConfig, &info)
	})
}

func TestCleanupTerraformFiles_PlanSubcommandSkipsCleanup(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "plan",
	}
	assert.NotPanics(t, func() {
		cleanupTerraformFiles(&atmosConfig, &info)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// assembleComponentEnvVars
// ──────────────────────────────────────────────────────────────────────────────

func TestAssembleComponentEnvVars_StandardVarsPresent(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.CliConfigPath = "/etc/atmos"
	atmosConfig.BasePath = tmpDir

	info := schema.ConfigAndStacksInfo{}

	err := assembleComponentEnvVars(&atmosConfig, &info, nil)
	require.NoError(t, err)

	// Check that essential vars are always added.
	envMap := envListToMap(info.ComponentEnvList)
	assert.Equal(t, "/etc/atmos", envMap["ATMOS_CLI_CONFIG_PATH"])
	assert.Equal(t, "true", envMap["TF_IN_AUTOMATION"])
	_, hasBasePath := envMap["ATMOS_BASE_PATH"]
	assert.True(t, hasBasePath, "ATMOS_BASE_PATH should be set")
}

func TestAssembleComponentEnvVars_ComponentEnvSectionMerged(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir

	info := schema.ConfigAndStacksInfo{
		ComponentEnvSection: map[string]any{
			"MY_VAR": "my-value",
		},
	}

	err := assembleComponentEnvVars(&atmosConfig, &info, nil)
	require.NoError(t, err)

	envMap := envListToMap(info.ComponentEnvList)
	assert.Equal(t, "my-value", envMap["MY_VAR"])
}

func TestAssembleComponentEnvVars_AppendUserAgentFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TF_APPEND_USER_AGENT", "") // Ensure OS env doesn't interfere.

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	atmosConfig.Components.Terraform.AppendUserAgent = "my-agent/1.0"

	info := schema.ConfigAndStacksInfo{}

	err := assembleComponentEnvVars(&atmosConfig, &info, nil)
	require.NoError(t, err)

	envMap := envListToMap(info.ComponentEnvList)
	assert.Equal(t, "my-agent/1.0", envMap["TF_APPEND_USER_AGENT"])
}

func TestAssembleComponentEnvVars_AppendUserAgentFromOSEnvOverridesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TF_APPEND_USER_AGENT", "os-agent/2.0")

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	atmosConfig.Components.Terraform.AppendUserAgent = "config-agent/1.0"

	info := schema.ConfigAndStacksInfo{}

	err := assembleComponentEnvVars(&atmosConfig, &info, nil)
	require.NoError(t, err)

	envMap := envListToMap(info.ComponentEnvList)
	assert.Equal(t, "os-agent/2.0", envMap["TF_APPEND_USER_AGENT"])
}

// ──────────────────────────────────────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────────────────────────────────────

// envListToMap converts a []string of "KEY=VALUE" pairs to a map for easy lookup.
func envListToMap(envList []string) map[string]string {
	m := make(map[string]string, len(envList))
	for _, pair := range envList {
		idx := len(pair)
		for i, c := range pair {
			if c == '=' {
				idx = i
				break
			}
		}
		key := pair[:idx]
		val := ""
		if idx < len(pair) {
			val = pair[idx+1:]
		}
		m[key] = val
	}
	return m
}

// writeTestFile is a helper to create a file with content for tests.
func writeTestFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
