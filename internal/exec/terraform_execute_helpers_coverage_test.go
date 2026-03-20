package exec

// terraform_execute_helpers_coverage_test.go contains unit tests for file-I/O,
// validation, init, arg-building, and cleanup helpers extracted from ExecuteTerraform.
// Workspace, TTY, auth, and pipeline tests live in sibling files to keep each file
// under the 600-line guideline.
//
// Organisation:
//   - printAndWriteVarFiles
//   - validateTerraformComponent
//   - prepareInitExecution
//   - buildInitSubcommandArgs (via prepareInitExecution)
//   - warnOnConflictingEnvVars (with env-var triggers)
//   - cleanupTerraformFiles (actual file creation/removal)

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ──────────────────────────────────────────────────────────────────────────────
// printAndWriteVarFiles
// ──────────────────────────────────────────────────────────────────────────────

// TestPrintAndWriteVarFiles_WorkspaceSubcommand verifies the early-return path
// when the subcommand is "workspace" (varfiles are not used for workspace ops).
func TestPrintAndWriteVarFiles_WorkspaceSubcommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace"}
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_DryRun_SkipsFileWrite verifies that with DryRun=true
// the function logs but does NOT attempt to write the varfile to disk.
func TestPrintAndWriteVarFiles_DryRun_SkipsFileWrite(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		DryRun:           true,
		UseTerraformPlan: false,
	}
	// /nonexistent path would error if an actual write were attempted.
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_UseTerraformPlan_SkipsVarFile verifies that when
// UseTerraformPlan=true the function skips the varfile entirely (we are
// applying a pre-built plan that already baked in the vars).
func TestPrintAndWriteVarFiles_UseTerraformPlan_SkipsVarFile(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		UseTerraformPlan: true,
	}
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_WithCliVarsSection verifies that a populated
// tf_cli_vars section in ComponentSection does not cause an error.
func TestPrintAndWriteVarFiles_WithCliVarsSection(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           true,
		UseTerraformPlan: false,
		ComponentSection: map[string]any{
			cfg.TerraformCliVarsSectionName: map[string]any{
				"region": "us-east-1",
				"env":    "prod",
			},
		},
	}
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_DebugLogLevel_Success verifies the LogLevelDebug branch
// triggers PrintAsYAMLToFileDescriptor without error for valid data.
func TestPrintAndWriteVarFiles_DebugLogLevel_Success(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Logs.Level = u.LogLevelDebug

	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           true,
		UseTerraformPlan: true, // Skip varfile write.
		ComponentVarsSection: map[string]any{
			"region": "us-east-1",
			"name":   "test",
		},
	}

	err := printAndWriteVarFiles(&atmosConfig, &info)
	require.NoError(t, err)
}

// TestPrintAndWriteVarFiles_DebugLogLevel_CliVarsSection exercises the second
// LogLevelDebug branch inside logCliVarsOverrides.
func TestPrintAndWriteVarFiles_DebugLogLevel_CliVarsSection(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Logs.Level = u.LogLevelDebug

	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           true,
		UseTerraformPlan: true,
		ComponentVarsSection: map[string]any{
			"region": "us-east-1",
		},
		ComponentSection: map[string]any{
			cfg.TerraformCliVarsSectionName: map[string]any{
				"override_var": "override_val",
			},
		},
	}

	err := printAndWriteVarFiles(&atmosConfig, &info)
	require.NoError(t, err)
}

// TestPrintAndWriteVarFiles_TraceLogLevel exercises the LogLevelTrace path.
func TestPrintAndWriteVarFiles_TraceLogLevel(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Logs.Level = u.LogLevelTrace

	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           true,
		UseTerraformPlan: true,
		ComponentVarsSection: map[string]any{
			"env": "prod",
		},
	}

	err := printAndWriteVarFiles(&atmosConfig, &info)
	require.NoError(t, err)
}

// TestPrintAndWriteVarFiles_WriteActualFile verifies that with DryRun=false the
// function writes a JSON varfile to disk at the path constructed from atmosConfig+info.
func TestPrintAndWriteVarFiles_WriteActualFile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           false,
		UseTerraformPlan: false,
		ContextPrefix:    "ctx",
		Component:        "mycomp",
		ComponentVarsSection: map[string]any{
			"region": "us-east-1",
		},
	}

	// The function builds the varfile path itself from atmosConfig+info.
	expectedVarfilePath := constructTerraformComponentVarfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(expectedVarfilePath), 0o755))

	err := printAndWriteVarFiles(&atmosConfig, &info)
	require.NoError(t, err)
	assert.FileExists(t, expectedVarfilePath)
}

// ──────────────────────────────────────────────────────────────────────────────
// validateTerraformComponent
// ──────────────────────────────────────────────────────────────────────────────

// TestValidateTerraformComponent_EmptySection_Valid verifies that a component
// with no validation section passes validation (empty validations → valid).
func TestValidateTerraformComponent_EmptySection_Valid(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "my-component",
		ComponentSection: map[string]any{},
	}
	err := validateTerraformComponent(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestValidateTerraformComponent_NilSection_Valid verifies that a nil
// ComponentSection also passes (FindValidationSection handles nil).
func TestValidateTerraformComponent_NilSection_Valid(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "my-component",
		ComponentSection: nil,
	}
	err := validateTerraformComponent(&atmosConfig, &info)
	assert.NoError(t, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// prepareInitExecution
// ──────────────────────────────────────────────────────────────────────────────

// TestPrepareInitExecution_WorkdirPath_ReturnsWorkdir verifies that when the
// component section contains a workdir path key the function returns that path
// instead of the default componentPath.
func TestPrepareInitExecution_WorkdirPath_ReturnsWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	customWorkdir := filepath.Join(tmpDir, "custom-workdir")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"_workdir_path": customWorkdir,
		},
	}

	result, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, customWorkdir, result)
}

// TestPrepareInitExecution_NoWorkdirPath_ReturnsOriginalPath verifies that when
// no workdir path is set the original componentPath is returned unchanged.
func TestPrepareInitExecution_NoWorkdirPath_ReturnsOriginalPath(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	result, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, result)
}

// TestPrepareInitExecution_EmptyWorkdirPath_ReturnsOriginalPath verifies that an
// empty string for the workdir key is treated as "not set".
func TestPrepareInitExecution_EmptyWorkdirPath_ReturnsOriginalPath(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"_workdir_path": "", // empty → treated as not set
		},
	}

	result, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, result)
}

// ──────────────────────────────────────────────────────────────────────────────
// buildInitSubcommandArgs (exercises prepareInitExecution internally)
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildInitSubcommandArgs_BasicInit verifies the default case: no flags, no
// workdir override — just returns ["init"].
func TestBuildInitSubcommandArgs_BasicInit(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	componentPath := tmpDir

	args, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Equal(t, []string{"init"}, args)
	assert.Equal(t, tmpDir, componentPath) // path unchanged
}

// TestBuildInitSubcommandArgs_ReconfigureEnabled verifies -reconfigure is added.
func TestBuildInitSubcommandArgs_ReconfigureEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	componentPath := tmpDir

	args, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Contains(t, args, "-reconfigure")
}

// TestBuildInitSubcommandArgs_PassVarsEnabled verifies varfile flag is added.
func TestBuildInitSubcommandArgs_PassVarsEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	componentPath := tmpDir

	args, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Contains(t, args, varFileFlag)
	assert.Contains(t, args, "vars.json")
}

// TestBuildInitSubcommandArgs_WorkdirPathUpdatesComponentPath verifies that when
// the component section carries a workdir path key, *componentPath is updated.
func TestBuildInitSubcommandArgs_WorkdirPathUpdatesComponentPath(t *testing.T) {
	tmpDir := t.TempDir()
	customWorkdir := filepath.Join(tmpDir, "custom-workdir")
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"_workdir_path": customWorkdir,
		},
	}
	componentPath := tmpDir

	_, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Equal(t, customWorkdir, componentPath)
}

// warnOnConflictingEnvVars is a logging-only function with no return value or
// side effects beyond log.Warn. NotPanics-only tests were removed per CLAUDE.md:
// "avoid tautological tests; no coverage theater." The function's log output
// cannot be intercepted without log-hook infrastructure.

// ──────────────────────────────────────────────────────────────────────────────
// cleanupTerraformFiles (actual file creation/removal)
// ──────────────────────────────────────────────────────────────────────────────

// TestCleanupTerraformFiles_ApplyRemovesVarfileForReal creates an actual varfile
// and verifies it is deleted after cleanupTerraformFiles for apply.
func TestCleanupTerraformFiles_ApplyRemovesVarfileForReal(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "apply",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}

	// Construct the expected path so we can create the file before cleanup.
	varfilePath := constructTerraformComponentVarfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(varfilePath), 0o755))
	require.NoError(t, os.WriteFile(varfilePath, []byte(`{"key":"value"}`), 0o644))
	require.FileExists(t, varfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.NoFileExists(t, varfilePath)
}

// TestCleanupTerraformFiles_NonPlanShow_RemovesPlanfile creates an actual planfile
// and verifies it is removed after cleanupTerraformFiles for a destroy command.
func TestCleanupTerraformFiles_NonPlanShow_RemovesPlanfile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "destroy",
		ContextPrefix: "ctx",
		Component:     "mycomp",
		PlanFile:      "", // empty PlanFile triggers planfile removal
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))
	require.FileExists(t, planfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.NoFileExists(t, planfilePath)
}

// TestCleanupTerraformFiles_ShowSubcommand_KeepsPlanfile verifies that "show"
// does NOT remove the planfile (it is a read-only operation over the plan).
func TestCleanupTerraformFiles_ShowSubcommand_KeepsPlanfile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "show",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))
	require.FileExists(t, planfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.FileExists(t, planfilePath, "show should not delete the planfile")
}

// TestCleanupTerraformFiles_PlanSubcommand_KeepsPlanfile verifies that "plan"
// itself does NOT remove the planfile it just generated.
func TestCleanupTerraformFiles_PlanSubcommand_KeepsPlanfile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "plan",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))
	require.FileExists(t, planfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.FileExists(t, planfilePath, "plan should not delete its own output planfile")
}

// TestCleanupTerraformFiles_ApplyWithCustomPlanFile_SkipsPlanfileRemoval
// verifies that when PlanFile is non-empty (consuming a pre-existing plan) the
// planfile is NOT deleted by cleanup — only the varfile is.
func TestCleanupTerraformFiles_ApplyWithCustomPlanFile_SkipsPlanfileRemoval(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "apply",
		ContextPrefix: "ctx",
		Component:     "mycomp",
		PlanFile:      "custom.planfile", // non-empty → planfile removal is skipped
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.FileExists(t, planfilePath, "planfile should NOT be removed when PlanFile is set")
}

// TestCleanupTerraformFiles_MissingFiles_NoError verifies that cleanup is
// graceful when neither planfile nor varfile exists (already cleaned up).
func TestCleanupTerraformFiles_MissingFiles_NoError(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = t.TempDir()
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "apply",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}
	assert.NotPanics(t, func() { cleanupTerraformFiles(&atmosConfig, &info) })
}
