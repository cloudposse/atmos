package exec

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
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

// TestBuildInitArgs_ReconfigureWhenWorkdirReprovisioned verifies that -reconfigure is
// added when the workdir was actually wiped and re-provisioned this invocation
// (WorkdirReprovisionedKey set by the source/workdir provisioner).
// This prevents "Do you want to migrate all workspaces?" on fresh workdirs.
func TestBuildInitArgs_ReconfigureWhenWorkdirReprovisioned(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey:          "/tmp/.workdir/terraform/demo-consumer",
			provWorkdir.WorkdirReprovisionedKey: struct{}{},
		},
	}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure"}, args)
}

// TestBuildInitArgs_ReconfigureWhenWorkdirReprovisioned_WithPassVars verifies that both
// -reconfigure and -var-file are added when workdir was re-provisioned and PassVars is enabled.
func TestBuildInitArgs_ReconfigureWhenWorkdirReprovisioned_WithPassVars(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey:          "/tmp/.workdir/terraform/demo-consumer",
			provWorkdir.WorkdirReprovisionedKey: struct{}{},
		},
	}
	args := buildInitArgs(&atmosConfig, &info, "my-component.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure", varFileFlag, "my-component.tfvars.json"}, args)
}

// TestBuildInitArgs_NoReconfigureWhenWorkdirPreserved verifies that -reconfigure is NOT
// added when the workdir exists but was not re-provisioned (TTL not expired).
// Adding -reconfigure causes OpenTofu to treat init as fresh and prompt
// "Do you want to migrate all workspaces?" even when the backend is unchanged.
func TestBuildInitArgs_NoReconfigureWhenWorkdirPreserved(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "/tmp/.workdir/terraform/demo-consumer",
			// WorkdirReprovisionedKey intentionally absent — TTL not expired
		},
	}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init"}, args)
}

// TestBuildInitArgs_NoReconfigureWhenWorkdirPreserved_InitRunReconfigureIgnored verifies
// that InitRunReconfigure: true is ignored for workdir components with a preserved workdir.
// -reconfigure + workspace state dirs causes the "migrate all workspaces?" prompt even
// when the backend is unchanged; the global flag must not override this protection.
func TestBuildInitArgs_NoReconfigureWhenWorkdirPreserved_InitRunReconfigureIgnored(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "/tmp/.workdir/terraform/demo-consumer",
			// WorkdirReprovisionedKey intentionally absent — workdir was NOT wiped
		},
	}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init"}, args)
}

// TestBuildInitArgs_ReconfigureForNonWorkdir_InitRunReconfigure verifies that
// InitRunReconfigure: true still works as expected for non-workdir components.
func TestBuildInitArgs_ReconfigureForNonWorkdir_InitRunReconfigure(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		ComponentSection: map[string]any{}, // no WorkdirPathKey
	}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init", "-reconfigure"}, args)
}

// TestBuildInitArgs_NoReconfigureWithoutWorkdir verifies that -reconfigure is NOT
// added for regular (non-workdir) components unless explicitly configured.
func TestBuildInitArgs_NoReconfigureWithoutWorkdir(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		ComponentSection: map[string]any{},
	}
	args := buildInitArgs(&atmosConfig, &info, "vars.tfvars.json")
	assert.Equal(t, []string{"init"}, args)
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

	_, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
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

	_, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)

	_, statErr := os.Stat(envFile)
	assert.True(t, os.IsNotExist(statErr), ".terraform/environment must be deleted for non-workdir components")
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
// applyMetadataComponentSubpath
// ──────────────────────────────────────────────────────────────────────────────

// TestApplyMetadataComponentSubpath_WithSubpath verifies that a non-empty baseComponentPath
// is joined onto the workdir path (the core fix for issue #2364).
func TestApplyMetadataComponentSubpath_WithSubpath(t *testing.T) {
	workdir := t.TempDir()
	result := applyMetadataComponentSubpath("modules/iam-policy", workdir)
	assert.Equal(t, filepath.Join(workdir, "modules", "iam-policy"), result)
}

// TestApplyMetadataComponentSubpath_EmptySubpath is the regression guard:
// when metadata.component is absent (baseComponentPath == ""), the workdir root
// must be returned unchanged.
func TestApplyMetadataComponentSubpath_EmptySubpath(t *testing.T) {
	workdir := t.TempDir()
	result := applyMetadataComponentSubpath("", workdir)
	assert.Equal(t, workdir, result)
}

// TestApplyMetadataComponentSubpath_DotDotEscapeHatch verifies that ".." in
// baseComponentPath escapes the workdir boundary — intentional escape hatch.
func TestApplyMetadataComponentSubpath_DotDotEscapeHatch(t *testing.T) {
	workdir := t.TempDir()
	result := applyMetadataComponentSubpath("../sibling-module", workdir)
	// The result must be the sibling directory, not inside workdir.
	expected := filepath.Join(filepath.Dir(workdir), "sibling-module")
	assert.Equal(t, expected, result)
	// Confirm the result is NOT inside workdir — the ".." actually escaped.
	assert.NotEqual(t, workdir, result)
}

// TestApplyMetadataComponentSubpath_SingleSegment verifies a single-segment subpath.
func TestApplyMetadataComponentSubpath_SingleSegment(t *testing.T) {
	workdir := t.TempDir()
	result := applyMetadataComponentSubpath("exports", workdir)
	assert.Equal(t, filepath.Join(workdir, "exports"), result)
}

// ──────────────────────────────────────────────────────────────────────────────
// applyWorkdirSubpathToSection
// ──────────────────────────────────────────────────────────────────────────────

// TestApplyWorkdirSubpathToSection_JoinsSubpath verifies that the helper joins
// metadata.component onto WorkdirPathKey, mutates the component section in
// place, and sets the sentinel. This is the load-bearing fix for issue #2364:
// downstream consumers of WorkdirPathKey see the corrected path.
func TestApplyWorkdirSubpathToSection_JoinsSubpath(t *testing.T) {
	workdirRoot := t.TempDir()
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "modules/iam-policy",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}
	path, ok := applyWorkdirSubpathToSection(info)
	require.True(t, ok, "helper should report success when WorkdirPathKey is set")

	expected := filepath.Join(workdirRoot, "modules", "iam-policy")
	assert.Equal(t, expected, path, "returned path should be the joined subpath")
	assert.Equal(t, expected, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"WorkdirPathKey should be mutated in place to the joined subpath")
	_, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey]
	assert.True(t, applied, "sentinel WorkdirSubpathAppliedKey should be set")
}

// TestApplyWorkdirSubpathToSection_DoubleCallAppliesOnce proves the sentinel
// prevents double-joining when applyWorkdirSubpathToSection is invoked twice
// for the same component section (the terraform-init-then-terraform-plan
// scenario). Without the sentinel, the second call would produce
// <workdir>/<subpath>/<subpath>/.
func TestApplyWorkdirSubpathToSection_DoubleCallAppliesOnce(t *testing.T) {
	workdirRoot := t.TempDir()
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}
	expected := filepath.Join(workdirRoot, "exports")

	first, _ := applyWorkdirSubpathToSection(info)
	assert.Equal(t, expected, first)

	second, _ := applyWorkdirSubpathToSection(info)
	assert.Equal(t, expected, second, "second call must not re-join the subpath")
	assert.Equal(t, expected, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"section should still hold the singly-joined path")
}

// ──────────────────────────────────────────────────────────────────────────────
// resolveWorkdirComponentPath
// ──────────────────────────────────────────────────────────────────────────────

// TestResolveWorkdirComponentPath_ExistingDir returns exists=true and the
// joined path when both the workdir root and the metadata.component subpath
// exist on disk.
func TestResolveWorkdirComponentPath_ExistingDir(t *testing.T) {
	basePath := t.TempDir()
	stack := "dev"
	componentName := "null-label-exports"
	subpath := "exports"

	expectedRoot := filepath.Join(basePath, ".workdir", cfg.TerraformComponentType, stack+"-"+componentName)
	expectedCandidate := filepath.Join(expectedRoot, subpath)
	require.NoError(t, os.MkdirAll(expectedCandidate, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    componentName,
		Stack:             stack,
		BaseComponentPath: subpath,
		ComponentSection:  map[string]any{},
	}

	candidate, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, expectedCandidate, candidate)
}

// TestResolveWorkdirComponentPath_NonExistentDir returns exists=false and no
// error when the candidate path does not yet exist (workdir not provisioned).
// This lets callers retain their fallback path without surfacing a misleading
// error.
func TestResolveWorkdirComponentPath_NonExistentDir(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    "missing-component",
		Stack:             "dev",
		BaseComponentPath: "exports",
		ComponentSection:  map[string]any{},
	}

	candidate, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.NotEmpty(t, candidate)
}

// TestResolveWorkdirComponentPath_StatErrorPropagates ensures non-ENOENT stat
// failures (e.g. EACCES) surface as wrapped ErrWorkdirProvision instead of a
// silent fallback that masks the real failure.
func TestResolveWorkdirComponentPath_StatErrorPropagates(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test relies on POSIX permission denial; root bypasses chmod")
	}
	basePath := t.TempDir()
	stack := "dev"
	componentName := "guarded"
	subpath := "exports"

	// Create the workdir root, then chmod the parent so the candidate stat
	// fails with EACCES rather than ENOENT.
	expectedRoot := filepath.Join(basePath, ".workdir", cfg.TerraformComponentType, stack+"-"+componentName)
	require.NoError(t, os.MkdirAll(expectedRoot, 0o755))
	require.NoError(t, os.Chmod(expectedRoot, 0o000))
	t.Cleanup(func() { _ = os.Chmod(expectedRoot, 0o755) })

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    componentName,
		Stack:             stack,
		BaseComponentPath: subpath,
		ComponentSection:  map[string]any{},
	}

	_, _, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-ENOENT stat failures must wrap ErrWorkdirProvision")
}
