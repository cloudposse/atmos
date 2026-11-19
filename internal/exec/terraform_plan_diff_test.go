package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/plandiff"
)

// TestTerraformPlanDiffProcessStacksError tests the error path when ProcessStacks fails.
func TestTerraformPlanDiffProcessStacksError(t *testing.T) {
	// Create a minimal atmosConfig
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: t.TempDir(),
	}

	// Test case 1: Missing stack triggers ProcessStacks error
	t.Run("missing_stack_returns_error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "", // Missing stack - will cause ProcessStacks to fail
			ComponentType:    "terraform",
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stack is required")
	})

	// Test case 2: Missing component triggers ProcessStacks error
	t.Run("missing_component_returns_error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "", // Missing component - will cause ProcessStacks to fail
			Stack:            "test-stack",
			ComponentType:    "terraform",
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "component")
	})

	// Test case 3: Invalid stack configuration triggers ProcessStacks error
	t.Run("invalid_stack_config_returns_error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "nonexistent-component",
			Stack:            "nonexistent-stack",
			ComponentType:    "terraform",
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		// ProcessStacks will fail because it can't find the stack configuration
	})
}

// TestTerraformPlanDiffWithVariousInputs tests the TerraformPlanDiff function with various inputs.
// These tests exercise the function entry point and error handling paths.
func TestTerraformPlanDiffWithVariousInputs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create atmosConfig
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: filepath.Join(tmpDir, "components", "terraform"),
	}

	// Test with empty AdditionalArgsAndFlags - ensures function entry is covered
	t.Run("empty_args_with_valid_stack_component", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg:       "test-component",
			Stack:                  "test-stack",
			ComponentType:          "terraform",
			AdditionalArgsAndFlags: []string{},
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		// Will fail at ProcessStacks because stack config doesn't exist
	})

	// Test with flags but invalid stack - ensures function processes input
	t.Run("with_flags_invalid_stack", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "nonexistent-stack",
			ComponentType:    "terraform",
			AdditionalArgsAndFlags: []string{
				"--orig=test.planfile",
				"--new=new.planfile",
			},
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		// Will fail at ProcessStacks
	})

	// Test with nil AdditionalArgsAndFlags
	t.Run("nil_args", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg:       "test-component",
			Stack:                  "test-stack",
			ComponentType:          "terraform",
			AdditionalArgsAndFlags: nil,
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
	})
}

// TestTerraformPlanDiffWithValidFixture tests the TerraformPlanDiff function with a valid fixture.
// This test exercises the code path AFTER ProcessStacks succeeds (covers the "happy path" branch).
func TestTerraformPlanDiffWithValidFixture(t *testing.T) {
	// Change to the test fixture directory
	testDir := "../../tests/fixtures/scenarios/atmos-terraform-plan-diff"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Test 1: ProcessStacks succeeds, but --orig flag is missing
	t.Run("process_stacks_succeeds_missing_orig_flag", func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:       "base-component",
			Stack:                  "test-stack",
			ComponentType:          cfg.TerraformComponentType,
			AdditionalArgsAndFlags: []string{}, // Missing --orig flag
		}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		require.NoError(t, err)

		err = TerraformPlanDiff(&atmosConfig, &configAndStacksInfo)
		assert.Error(t, err)
		// Should fail with "original plan file (--orig) is required" AFTER ProcessStacks succeeds
		assert.Contains(t, err.Error(), "original plan file (--orig) is required")
	})

	// Test 2: ProcessStacks succeeds, --orig flag provided, but file doesn't exist
	t.Run("process_stacks_succeeds_nonexistent_planfile", func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:       "base-component",
			Stack:                  "test-stack",
			ComponentType:          cfg.TerraformComponentType,
			AdditionalArgsAndFlags: []string{"--orig=nonexistent.planfile"},
		}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		require.NoError(t, err)

		err = TerraformPlanDiff(&atmosConfig, &configAndStacksInfo)
		assert.Error(t, err)
		// Should fail with "does not exist" after ProcessStacks and parsePlanDiffFlags succeed
		assert.Contains(t, err.Error(), "does not exist")
	})

	// Test 3: ProcessStacks succeeds, planfile exists, but new planfile doesn't exist
	// This exercises more of the code path including prepareNewPlanFile
	t.Run("process_stacks_succeeds_with_existing_orig_planfile", func(t *testing.T) {
		// Create a dummy planfile in the component directory
		planfilePath := filepath.Join("components", "terraform", "base-component", "test.planfile")
		err := os.WriteFile(planfilePath, []byte("dummy plan data"), 0o644)
		require.NoError(t, err)
		defer os.Remove(planfilePath)

		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg: "base-component",
			Stack:            "test-stack",
			ComponentType:    cfg.TerraformComponentType,
			AdditionalArgsAndFlags: []string{
				"--orig=test.planfile",
				"--new=nonexistent-new.planfile", // New file doesn't exist
			},
			SkipInit: true, // Skip terraform init for this test
		}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		require.NoError(t, err)

		err = TerraformPlanDiff(&atmosConfig, &configAndStacksInfo)
		assert.Error(t, err)
		// Should fail with "does not exist" for the new planfile
		// This exercises validateOriginalPlanFile success and prepareNewPlanFile
		assert.Contains(t, err.Error(), "does not exist")
	})
}

func TestRunTerraformShowCapturesWithInjectedStdout(t *testing.T) {
	origStdout := os.Stdout
	executor := terraformPlanDiffExecutorFunc(func(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
		assert.Equal(t, "show", info.SubCommand)
		assert.Equal(t, []string{"-json", "plan.tfplan"}, info.AdditionalArgsAndFlags)
		assert.Same(t, origStdout, os.Stdout)

		var cfg shellCommandConfig
		for _, opt := range opts {
			opt(&cfg)
		}
		require.NotNil(t, cfg.StdoutOverride)
		_, err := cfg.StdoutOverride.Write([]byte("terraform noise\n{\"format_version\":\"1.0\"}"))
		require.NoError(t, err)
		return nil
	})

	output, err := runTerraformShowWithExecutor(&schema.ConfigAndStacksInfo{}, "plan.tfplan", executor)

	require.NoError(t, err)
	assert.Equal(t, "terraform noise\n{\"format_version\":\"1.0\"}", output)
	assert.Same(t, origStdout, os.Stdout)
}

// TestMockTerraformPlanDiff tests the plandiff.Compare function directly.
func TestMockTerraformPlanDiff(t *testing.T) {
	// Create JSON content for the original plan
	origPlanJSON := map[string]interface{}{
		"format_version":    "1.2",
		"terraform_version": "1.5.7",
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/Stockholm",
				},
			},
		},
	}

	// Create JSON content for the new plan
	newPlanJSON := map[string]interface{}{
		"format_version":    "1.2",
		"terraform_version": "1.5.7",
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "New York",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/New+York",
				},
			},
		},
	}

	// Test the plandiff.Compare function directly
	diff, hasDiff := plandiff.Compare(origPlanJSON, newPlanJSON)
	assert.True(t, hasDiff, "Plans should be different")
	assert.Contains(t, diff, "~ location: Stockholm => New York")
	assert.Contains(t, diff, "~ url: https://example.com/Stockholm => https://example.com/New+York")

	// Test with identical plans
	diff, hasDiff = plandiff.Compare(origPlanJSON, origPlanJSON)
	assert.False(t, hasDiff, "Identical plans should not have differences")
	assert.Empty(t, diff, "Diff should be empty for identical plans")
}

// TestTerraformPlanDiffMetadataComponentResolution verifies that plan-diff uses FinalComponent
// (resolved from metadata.component) rather than ComponentFromArg when constructing
// the component path. This prevents searching the wrong directory (e.g.,
// components/terraform/foobar-atmos-pro/ instead of components/terraform/foobar/).
func TestTerraformPlanDiffMetadataComponentResolution(t *testing.T) {
	// This is a unit test for the component path construction logic.
	// It tests that the componentPath is correctly built from FinalComponent,
	// not from ComponentFromArg.

	tmpDir := t.TempDir()

	// Create the component directory structure:
	// - components/terraform/base-component/ (actual component code)
	// - components/terraform/derived-component/ should NOT exist
	baseComponentDir := filepath.Join(tmpDir, "components", "terraform", "base-component")
	err := os.MkdirAll(baseComponentDir, 0o755)
	require.NoError(t, err)

	// Create a dummy planfile in the CORRECT location (base-component)
	planfileName := "test.planfile"
	planfilePath := filepath.Join(baseComponentDir, planfileName)
	err = os.WriteFile(planfilePath, []byte("dummy plan data"), 0o644)
	require.NoError(t, err)

	terraformDirPath := filepath.Join(tmpDir, "components", "terraform")

	// Test that validateOriginalPlanFile finds the file in the correct component path
	// when FinalComponent is properly set (simulates what happens after ProcessStacks)
	t.Run("component_path_with_FinalComponent", func(t *testing.T) {
		// Construct component path as TerraformPlanDiff does after ProcessStacks
		componentFolderPrefix := ""
		finalComponent := "base-component" // Resolved from metadata.component
		componentPath := filepath.Join(terraformDirPath, componentFolderPrefix, finalComponent)

		// Validate that the planfile is found in the correct path
		result, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.NoError(t, err)
		assert.Equal(t, planfilePath, result)
	})

	// Test that the planfile would NOT be found if we used ComponentFromArg instead of FinalComponent
	// This demonstrates what the bug was before the fix
	t.Run("component_path_with_wrong_component_name", func(t *testing.T) {
		// Construct component path incorrectly (using ComponentFromArg instead of FinalComponent)
		componentFolderPrefix := ""
		componentFromArg := "derived-component" // Wrong! Should use FinalComponent
		wrongComponentPath := filepath.Join(terraformDirPath, componentFolderPrefix, componentFromArg)

		// Validate that the planfile is NOT found in the wrong path
		_, err := validateOriginalPlanFile(planfileName, wrongComponentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}

// TestCopyPlanFileIfNeeded tests the copyPlanFileIfNeeded function.
// TestTerraformPlanDiffWithWorkdir tests that plan-diff correctly handles workdir scenarios.
// When workdir is enabled, the component path should be the workdir path, not the base component path.
// This test verifies the path resolution logic works with workdir provisioning.
func TestTerraformPlanDiffWithWorkdir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the base component directory structure
	baseComponentDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	err := os.MkdirAll(baseComponentDir, 0o755)
	require.NoError(t, err)

	// Create the workdir directory (what workdir provisioner creates)
	workdirPath := filepath.Join(tmpDir, ".workdir", "terraform", "dev-vpc")
	err = os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	// Create planfile in the WORKDIR (where it would be after workdir provisioning)
	planfileName := "test.planfile"
	planfileInWorkdir := filepath.Join(workdirPath, planfileName)
	err = os.WriteFile(planfileInWorkdir, []byte("dummy plan data from workdir"), 0o644)
	require.NoError(t, err)

	terraformDirPath := filepath.Join(tmpDir, "components", "terraform")

	// Test 1: Planfile is found in workdir path (correct behavior)
	t.Run("planfile_found_in_workdir", func(t *testing.T) {
		// When workdir is enabled, the componentPath should be the workdir path
		componentPath := workdirPath

		result, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.NoError(t, err)
		assert.Equal(t, planfileInWorkdir, result)
	})

	// Test 2: Planfile NOT found in base component path (demonstrates the issue)
	t.Run("planfile_not_in_base_component_path", func(t *testing.T) {
		// If we incorrectly use the base component path instead of workdir,
		// the planfile won't be found
		componentPath := filepath.Join(terraformDirPath, "vpc")

		_, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	// Test 3: Verify WorkdirPathKey extraction from ComponentSection
	t.Run("workdir_path_key_extraction", func(t *testing.T) {
		// This tests the pattern used in terraform.go and other commands
		// to extract the workdir path from ComponentSection
		componentSection := map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		}

		// Extract workdir path as done in terraform.go
		if extractedWorkdir, ok := componentSection[provWorkdir.WorkdirPathKey].(string); ok {
			assert.Equal(t, workdirPath, extractedWorkdir)

			// Validate planfile is found at workdir path
			result, err := validateOriginalPlanFile(planfileName, extractedWorkdir)
			assert.NoError(t, err)
			assert.Equal(t, planfileInWorkdir, result)
		} else {
			t.Fatal("Failed to extract workdir path from ComponentSection")
		}
	})

	// Test 4: Empty workdir path should fall back to base component path
	t.Run("empty_workdir_falls_back_to_base", func(t *testing.T) {
		componentSection := map[string]any{
			provWorkdir.WorkdirPathKey: "",
		}

		// When workdir path is empty, should use base component path
		extractedWorkdir, ok := componentSection[provWorkdir.WorkdirPathKey].(string)
		if ok && extractedWorkdir != "" {
			t.Fatal("Expected empty workdir path")
		}

		// With empty workdir, would fall back to base component path
		// Create planfile in base component for this test
		planfileInBase := filepath.Join(baseComponentDir, planfileName)
		err := os.WriteFile(planfileInBase, []byte("dummy plan data from base"), 0o644)
		require.NoError(t, err)
		defer os.Remove(planfileInBase)

		result, err := validateOriginalPlanFile(planfileName, baseComponentDir)
		assert.NoError(t, err)
		assert.Equal(t, planfileInBase, result)
	})
}

// TestTerraformPlanDiffWithSourceVendoring tests that plan-diff correctly handles
// source-vendored components with workdir enabled.
// When source + workdir are both enabled, the planfile should be in the workdir path.
func TestTerraformPlanDiffWithSourceVendoring(t *testing.T) {
	tmpDir := t.TempDir()

	// Source + workdir scenario:
	// 1. Component is vendored from remote source
	// 2. Workdir is enabled, so files go to .workdir/terraform/<stack>-<component>/
	// 3. Planfile should be looked up in the workdir path

	// Create the workdir path where source provisioner would vendor files
	workdirPath := filepath.Join(tmpDir, ".workdir", "terraform", "plat-ue2-sandbox-foobar-atmos-pro")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	// Create a mock terraform file to simulate vendored component
	mainTf := filepath.Join(workdirPath, "main.tf")
	err = os.WriteFile(mainTf, []byte(`resource "null_resource" "example" {}`), 0o644)
	require.NoError(t, err)

	// Create the planfile in the workdir (where it would be stored after terraform plan)
	planfileName := "plat-ue2-sandbox-foobar-atmos-pro.planfile"
	planfilePath := filepath.Join(workdirPath, planfileName)
	err = os.WriteFile(planfilePath, []byte("dummy planfile for source+workdir test"), 0o644)
	require.NoError(t, err)

	// Simulate what would happen in TerraformPlanDiff with source + workdir
	t.Run("source_vendored_with_workdir", func(t *testing.T) {
		// After ProcessStacks, the component section would have WorkdirPathKey set
		// by the source provisioner when source + workdir are enabled
		componentSection := map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		}

		// Extract workdir path as done in terraform.go
		componentPath := ""
		if workdir, ok := componentSection[provWorkdir.WorkdirPathKey].(string); ok && workdir != "" {
			componentPath = workdir
		}

		assert.Equal(t, workdirPath, componentPath)

		// Validate planfile is found
		result, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.NoError(t, err)
		assert.Equal(t, planfilePath, result)
	})

	// Test the error case when workdir path is not used
	t.Run("source_vendored_without_workdir_path_fails", func(t *testing.T) {
		// If plan-diff doesn't check WorkdirPathKey and uses base component path,
		// it won't find the planfile
		baseComponentPath := filepath.Join(tmpDir, "components", "terraform", "foobar")

		_, err := validateOriginalPlanFile(planfileName, baseComponentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}

type planDiffCallerTestCase struct {
	name       string
	entries    []string
	realChange bool
	wantDiff   bool
	pin        string
}

func setupPlanDiffPolicyTest(t *testing.T, tc *planDiffCallerTestCase) (schema.AtmosConfiguration, schema.ConfigAndStacksInfo, string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_CI", "false")
	t.Setenv("ATMOS_EDITION", "")
	t.Setenv("_ATMOS_TEST_PLAN_DIFF_TERRAFORM", "1")
	executable, err := os.Executable()
	require.NoError(t, err)
	componentDir := filepath.Join(dir, "components", "terraform", "rds")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("terraform {}\n"), 0o600))
	stacksDir := filepath.Join(dir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte("vars:\n  stage: test\ncomponents:\n  terraform:\n    rds:\n      vars: {}\n"), 0o600))
	config := map[string]interface{}{
		"base_path": dir,
		"edition":   tc.pin,
		"stacks": map[string]interface{}{
			"base_path": "stacks", "included_paths": []string{"*"}, "name_template": "{{ .vars.stage }}",
		},
		"components": map[string]interface{}{"terraform": map[string]interface{}{
			"base_path": "components/terraform", "command": executable, "auto_generate_backend_file": false,
			"plan_diff": map[string]interface{}{"ignore_changes": tc.entries},
		}},
	}
	configJSON, err := yaml.Marshal(config)
	require.NoError(t, err)
	configPath := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(configPath, configJSON, 0o600))
	info := schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configPath}, ComponentFromArg: "rds",
		ComponentType: cfg.TerraformComponentType, Stack: "test", SkipInit: true,
	}
	attribute := "latest_restorable_time"
	origJSON, err := json.Marshal(planDiffPolicyPlan(attribute, "old", false))
	require.NoError(t, err)
	freshJSON, err := json.Marshal(planDiffPolicyPlan(attribute, "new", tc.realChange))
	require.NoError(t, err)
	t.Setenv("_ATMOS_TEST_PLAN_DIFF_FRESH_JSON", string(freshJSON))
	origFile, freshFile := filepath.Join(componentDir, "stored.plan"), filepath.Join(componentDir, "fresh.plan")
	require.NoError(t, os.WriteFile(origFile, origJSON, 0o600))
	require.NoError(t, os.WriteFile(freshFile, freshJSON, 0o600))
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	return atmosConfig, info, origFile, freshFile
}

// TestPlanDiffPolicyCallers verifies that plan-diff and deployment verification both honor the
// configured exclusions. It runs serially because it changes the working directory, sets
// environment variables. Policy and display details are covered
// in pkg/terraform/plandiff.
func TestPlanDiffPolicyCallers(t *testing.T) {
	for _, caller := range []string{"plan-diff", "verification"} {
		for _, tc := range []planDiffCallerTestCase{
			{name: "unconfigured", wantDiff: true},
			{name: "configured", entries: []string{"aws_db_instance.latest_restorable_time"}},
			{name: "older edition configured", pin: "2026-10-12", entries: []string{"aws_db_instance.latest_restorable_time"}},
			{name: "real change", entries: []string{"aws_db_instance.latest_restorable_time"}, realChange: true, wantDiff: true},
		} {
			t.Run(caller+"/"+tc.name, func(t *testing.T) {
				config, info, orig, fresh := setupPlanDiffPolicyTest(t, &tc)
				assert.Equal(t, tc.pin, config.Edition)
				if caller == "verification" {
					err := VerifyPlanfile(&info, orig, schema.PlanfileVerifyFail)
					if tc.wantDiff {
						assert.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
					} else {
						require.NoError(t, err)
						assert.True(t, info.UseTerraformPlan)
						assert.FileExists(t, info.PlanFile)
					}
					return
				}
				info.AdditionalArgsAndFlags = []string{"--orig=" + orig, "--new=" + fresh}
				err := TerraformPlanDiff(&config, &info)
				if tc.wantDiff {
					require.ErrorIs(t, err, errUtils.ErrPlanHasDiff)
					assert.Equal(t, 2, errUtils.GetExitCode(err))
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}
