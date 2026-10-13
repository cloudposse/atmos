package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

// runPlanDiffTerraformForTest implements show/plan using JSON plan fixtures, without a provider or network.
func runPlanDiffTerraformForTest() int {
	args := os.Args[1:]
	if len(args) == 0 {
		return 1
	}
	switch args[0] {
	case "show":
		contents, err := os.ReadFile(args[len(args)-1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(string(contents))
	case "plan":
		for _, arg := range args[1:] {
			if path, found := strings.CutPrefix(arg, "-out="); found {
				if err := os.WriteFile(path, []byte(os.Getenv("_ATMOS_TEST_PLAN_DIFF_FRESH_JSON")), 0o600); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
				return 0
			}
		}
		return 1
	case "version":
		fmt.Println(`{"terraform_version":"1.11.0","platform":"test","provider_selections":{}}`)
	case "workspace":
		if len(args) > 1 && args[1] == "show" {
			fmt.Println("test")
		}
	}
	return 0
}

type planDiffCallerTestCase struct {
	name       string
	entries    []string
	realChange bool
	wantDiff   bool
	pin        string
	attribute  string
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
	attribute := tc.attribute
	if attribute == "" {
		attribute = "latest_restorable_time"
	}
	origResource := planDiffTestResource("aws_db_instance", attribute, "old", "resource_changes")
	freshResource := planDiffTestResource("aws_db_instance", attribute, "new", "resource_changes")
	if tc.realChange {
		freshResource["change"].(map[string]interface{})["after"].(map[string]interface{})["instance_class"] = "db.t3.small"
	}
	origJSON, err := json.Marshal(planDiffResourcePlan(t, origResource, "resource_changes"))
	require.NoError(t, err)
	freshJSON, err := json.Marshal(planDiffResourcePlan(t, freshResource, "resource_changes"))
	require.NoError(t, err)
	t.Setenv("_ATMOS_TEST_PLAN_DIFF_FRESH_JSON", string(freshJSON))
	origFile, freshFile := filepath.Join(componentDir, "stored.plan"), filepath.Join(componentDir, "fresh.plan")
	require.NoError(t, os.WriteFile(origFile, origJSON, 0o600))
	require.NoError(t, os.WriteFile(freshFile, freshJSON, 0o600))
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	return atmosConfig, info, origFile, freshFile
}

func TestPlanDiffPolicyCallers(t *testing.T) {
	for _, caller := range []string{"plan-diff", "verification"} {
		for _, tc := range []planDiffCallerTestCase{
			{name: "unconfigured", wantDiff: true},
			{name: "older edition configured", pin: "2026-10-12", entries: []string{"aws_db_instance.latest_restorable_time"}},
			{name: "old hash", pin: "2026-10-12", attribute: "content_md5", wantDiff: true},
			{name: "current hash", pin: "2026-10-13", attribute: "content_md5", wantDiff: true},
			{name: "old hash excluded", pin: "2026-10-12", attribute: "content_md5", entries: []string{"aws_db_instance.content_md5"}},
			{name: "current hash excluded", pin: "2026-10-13", attribute: "content_md5", entries: []string{"aws_db_instance.content_md5"}},
			{name: "configured", entries: []string{"aws_db_instance.latest_restorable_time"}},
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
				oldExit := errUtils.OsExit
				t.Cleanup(func() { errUtils.OsExit = oldExit })
				exitCode := 0
				errUtils.OsExit = func(code int) { exitCode = code }
				info.AdditionalArgsAndFlags = []string{"--orig=" + orig, "--new=" + fresh}
				require.NoError(t, TerraformPlanDiff(&config, &info))
				if tc.wantDiff {
					assert.Equal(t, 2, exitCode)
				} else {
					assert.Zero(t, exitCode)
				}
			})
		}
	}
}

func TestPlanDiffRejectsInvalidPolicyBeforeExecution(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.PlanDiff.IgnoreChanges = []string{"*.latest_restorable_time"}
	info := &schema.ConfigAndStacksInfo{}
	assert.ErrorIs(t, TerraformPlanDiff(config, info), schema.ErrInvalidPlanDiffIgnoreChange)
	assert.ErrorIs(t, comparePlansAndGenerateDiff(config, info, "", "", ""), schema.ErrInvalidPlanDiffIgnoreChange)
}

func TestPlanDiffRejectsInvalidEditionBeforeExecution(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{Edition: "not-a-date"}
	info := &schema.ConfigAndStacksInfo{}
	assert.ErrorIs(t, TerraformPlanDiff(config, info), edition.ErrInvalidEdition)
	assert.ErrorIs(t, comparePlansAndGenerateDiff(config, info, "", "", ""), edition.ErrInvalidEdition)
}
