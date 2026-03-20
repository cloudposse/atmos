package exec

import (
	"errors"
	"fmt"
	osexec "os/exec"
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
