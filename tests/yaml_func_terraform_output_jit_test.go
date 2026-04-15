package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// TestTerraformOutputJITWorkdir verifies that !terraform.output automatically
// provisions the JIT working directory for a component with
// provision.workdir.enabled: true before running terraform init.
//
// The test asserts that the workdir is created by the auto-provision step.
// terraform output may return empty results if no state exists — that is
// expected and acceptable; the key assertion is that auto-provision fired.
//
// Regression test for https://github.com/cloudposse/atmos/issues/2167.
func TestTerraformOutputJITWorkdir(t *testing.T) {
	// Skip if neither terraform nor tofu is available.
	RequireTerraformOrTofu(t)

	t.Chdir("./fixtures/scenarios/terraform-output-jit-workdir")

	// Reset caches for test isolation.
	tfoutput.ResetOutputsCache()
	tfoutput.ResetWorkdirProvisionCache()
	e.ResetStateCache()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.True(t, atmosConfig.Initialized)

	// The workdir should NOT exist before describe runs.
	// Use filepath.Abs after t.Chdir so the path is CWD-independent.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	workdirPath := filepath.Join(cwd, ".workdir", "terraform", "test-producer")
	_, statErr := os.Stat(workdirPath)
	require.True(t, os.IsNotExist(statErr), "workdir should not exist before test runs")

	// Describing consumer triggers !terraform.output producer test foo.
	// This should auto-provision producer's workdir before running terraform init.
	// If terraform output fails (no state), we still assert the workdir was created.
	componentSection, describeErr := e.ExecuteDescribeComponent(
		&e.ExecuteDescribeComponentParams{
			Component:            "consumer",
			Stack:                "test",
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		},
	)

	// Primary assertion: workdir was created by auto-provision.
	_, statErr = os.Stat(workdirPath)
	require.NoError(t, statErr, "workdir should have been auto-provisioned before terraform init (issue #2167)")

	// Secondary: if describe succeeded, verify the section is non-nil.
	if describeErr == nil {
		assert.NotNil(t, componentSection)
	}

	// Cleanup: remove workdir so re-runs are clean.
	t.Cleanup(func() { os.RemoveAll(filepath.Join(cwd, ".workdir")) })
}
