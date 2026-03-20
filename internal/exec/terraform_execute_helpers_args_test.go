package exec

// terraform_execute_helpers_args_test.go tests argument-building helpers extracted from
// ExecuteTerraform — specifically the subcommand arg constructors and env-var assembly:
//   - buildTerraformCommandArgs (all subcommands)
//   - buildPlanSubcommandArgs
//   - buildApplySubcommandArgs
//   - appendApplyPlanFileArg
//   - buildWorkspaceSubcommandArgs
//   - logTerraformContext
//   - warnOnConflictingEnvVars
//   - cleanupTerraformFiles
//   - assembleComponentEnvVars

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
		SubCommand:       "apply",
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
		SubCommand:       "apply",
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
		SubCommand:       "apply",
		UseTerraformPlan: true,
		PlanFile:         "custom.tfplan",
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
// buildPlanSubcommandArgs
// ──────────────────────────────────────────────────────────────────────────────

func TestBuildPlanSubcommandArgs_BasicPlan(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}

	args, uploadFlag := buildPlanSubcommandArgs(&atmosConfig, &info, []string{"plan"}, "vars.json", "plan.tfplan")

	assert.Contains(t, args, varFileFlag)
	assert.Contains(t, args, "vars.json")
	assert.Contains(t, args, outFlag)
	assert.Contains(t, args, "plan.tfplan")
	assert.False(t, uploadFlag)
}

func TestBuildPlanSubcommandArgs_SkipPlanfile(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Plan.SkipPlanfile = true
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}

	args, _ := buildPlanSubcommandArgs(&atmosConfig, &info, []string{"plan"}, "vars.json", "plan.tfplan")

	assert.NotContains(t, args, outFlag)
}

func TestBuildPlanSubcommandArgs_UploadStatusFlag(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:             "plan",
		AdditionalArgsAndFlags: []string{"--upload-status"},
	}

	args, uploadFlag := buildPlanSubcommandArgs(&atmosConfig, &info, []string{"plan"}, "vars.json", "plan.tfplan")

	assert.True(t, uploadFlag)
	assert.Contains(t, args, detailedExitCodeFlag)
	// Upload status flag should be removed from additional args.
	assert.NotContains(t, info.AdditionalArgsAndFlags, "--upload-status")
}

// ──────────────────────────────────────────────────────────────────────────────
// buildApplySubcommandArgs
// ──────────────────────────────────────────────────────────────────────────────

func TestBuildApplySubcommandArgs_WithoutPlan(t *testing.T) {
	info := schema.ConfigAndStacksInfo{UseTerraformPlan: false}

	args := buildApplySubcommandArgs(&info, []string{"apply"}, "vars.json")

	assert.Contains(t, args, varFileFlag)
	assert.Contains(t, args, "vars.json")
}

func TestBuildApplySubcommandArgs_WithPlan(t *testing.T) {
	info := schema.ConfigAndStacksInfo{UseTerraformPlan: true}

	args := buildApplySubcommandArgs(&info, []string{"apply"}, "vars.json")

	assert.NotContains(t, args, varFileFlag)
}

// ──────────────────────────────────────────────────────────────────────────────
// appendApplyPlanFileArg
// ──────────────────────────────────────────────────────────────────────────────

func TestAppendApplyPlanFileArg_Apply_WithDefaultPlanFile(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "apply", UseTerraformPlan: true}
	args := appendApplyPlanFileArg(&info, []string{"apply"}, "auto.tfplan")
	assert.Contains(t, args, "auto.tfplan")
}

func TestAppendApplyPlanFileArg_Apply_WithCustomPlanFile(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "apply", UseTerraformPlan: true, PlanFile: "custom.tfplan"}
	args := appendApplyPlanFileArg(&info, []string{"apply"}, "auto.tfplan")
	assert.Contains(t, args, "custom.tfplan")
	assert.NotContains(t, args, "auto.tfplan")
}

func TestAppendApplyPlanFileArg_NonApply_NoChange(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "plan", UseTerraformPlan: true}
	original := []string{"plan", varFileFlag, "vars.json"}
	args := appendApplyPlanFileArg(&info, original, "auto.tfplan")
	assert.Equal(t, original, args)
}

func TestAppendApplyPlanFileArg_ApplyWithoutPlanFile_NoChange(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "apply", UseTerraformPlan: false}
	original := []string{"apply", varFileFlag, "vars.json"}
	args := appendApplyPlanFileArg(&info, original, "auto.tfplan")
	assert.Equal(t, original, args)
}

// ──────────────────────────────────────────────────────────────────────────────
// buildWorkspaceSubcommandArgs
// ──────────────────────────────────────────────────────────────────────────────

func TestBuildWorkspaceSubcommandArgs_List(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace", SubCommand2: "list"}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	assert.Contains(t, args, "list")
}

func TestBuildWorkspaceSubcommandArgs_Show(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace", SubCommand2: "show"}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	assert.Contains(t, args, "show")
}

func TestBuildWorkspaceSubcommandArgs_New(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:         "workspace",
		SubCommand2:        "new",
		TerraformWorkspace: "my-ws",
	}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	assert.Contains(t, args, "new")
	assert.Contains(t, args, "my-ws")
}

func TestBuildWorkspaceSubcommandArgs_Bare(t *testing.T) {
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace", SubCommand2: ""}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	// No subcommand2 → no additional args appended.
	assert.Equal(t, []string{"workspace"}, args)
}

// ──────────────────────────────────────────────────────────────────────────────
// logTerraformContext
// ──────────────────────────────────────────────────────────────────────────────

// logTerraformContext is a logging-only function. These tests verify it handles
// all input shapes without panicking. The function formats a command string from
// SubCommand+SubCommand2 and constructs an inheritance string — both are fed to
// log.Debug which is not easily capturable in unit tests.

func TestLogTerraformContext_NoSubCommand2(t *testing.T) {
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
		SubCommand:                "plan",
		ComponentFromArg:          "my-component",
		ComponentInheritanceChain: []string{"base-component", "grandparent"},
	}
	assert.NotPanics(t, func() { logTerraformContext(&info, "/tmp/workdir") })
}

// ──────────────────────────────────────────────────────────────────────────────
// warnOnConflictingEnvVars
// ──────────────────────────────────────────────────────────────────────────────

// warnOnConflictingEnvVars no-conflict path is implicitly covered by the
// 5 t.Setenv tests in _coverage_test.go. Standalone no-op test removed.

// cleanupTerraformFiles tests with real file assertions live in _coverage_test.go:
//   - TestCleanupTerraformFiles_ApplyRemovesVarfileForReal
//   - TestCleanupTerraformFiles_NonPlanShow_RemovesPlanfile
//   - TestCleanupTerraformFiles_ShowSubcommand_KeepsPlanfile
//   - TestCleanupTerraformFiles_PlanSubcommand_KeepsPlanfile
//   - TestCleanupTerraformFiles_ApplyWithCustomPlanFile_SkipsPlanfileRemoval
//   - TestCleanupTerraformFiles_MissingFiles_NoError
// NotPanics-only duplicates removed.

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

// storeAutoDetectedIdentity tests (including preset identity guard) live in
// utils_auth_test.go — TestStoreAutoDetectedIdentity table-driven suite.

// ──────────────────────────────────────────────────────────────────────────────
// assembleComponentEnvVars with non-nil tenv
// ──────────────────────────────────────────────────────────────────────────────

// TestAssembleComponentEnvVars_NonNilTenv verifies that the tenv != nil branch
// runs without error. ToolchainEnvironment's fields are unexported, so we can only
// construct a zero-value instance (EnvVars returns nil). This test exercises the
// branch dispatch, not the PATH ordering (which requires a real toolchain install).
func TestAssembleComponentEnvVars_NonNilTenv(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir

	info := schema.ConfigAndStacksInfo{}

	// Zero-value ToolchainEnvironment: path is empty, so EnvVars() returns nil.
	// The tenv != nil branch still runs, exercising the code path.
	tenv := &dependencies.ToolchainEnvironment{}

	err := assembleComponentEnvVars(&atmosConfig, &info, tenv)
	require.NoError(t, err)
	// Standard vars should still be present.
	envMap := envListToMap(info.ComponentEnvList)
	assert.Equal(t, "true", envMap["TF_IN_AUTOMATION"])
}

// ──────────────────────────────────────────────────────────────────────────────
// buildTerraformCommandArgs — init branch
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildTerraformCommandArgs_Init verifies the case "init" dispatch through
// buildTerraformCommandArgs. Since buildInitSubcommandArgs calls prepareInitExecution
// which requires a component path, we use a temp dir as the component path.
func TestBuildTerraformCommandArgs_Init(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	atmosConfig.Components.Terraform.Init.PassVars = true

	info := schema.ConfigAndStacksInfo{
		SubCommand: "init",
	}
	cp := tmpDir

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.planfile", &cp)
	require.NoError(t, err)
	assert.Contains(t, args, "init")
	assert.Contains(t, args, "-reconfigure")
	assert.Contains(t, args, "-var-file")
	assert.Contains(t, args, "vars.json")
}

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
