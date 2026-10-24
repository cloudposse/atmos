package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestYamlFuncTerraformState(t *testing.T) {
	// Clear caches to ensure isolation from other tests that may have run first.
	ResetStateCache()
	tfoutput.ResetOutputsCache()
	t.Cleanup(func() {
		ResetStateCache()
		tfoutput.ResetOutputsCache()
	})

	if _, lookErr := exec.LookPath("tofu"); lookErr != nil {
		if _, lookErr2 := exec.LookPath("terraform"); lookErr2 != nil {
			t.Skip("skipping: neither 'tofu' nor 'terraform' binary found in PATH (required for !terraform.state integration test)")
		}
	}
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "nonprod"

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function"
	setupTerraformYamlFunctionSandbox(t, workDir)
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err := ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	d, err := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-a", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-b", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 nonprod baz", "", nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-c", d)

	res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-2",
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-c")

	info = schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-2",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 foo", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-a", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod bar", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-b", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod baz", "", nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-c", d)

	res, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-3",
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: default-value")
	assert.Contains(t, y, `test_list:
    - fallback1
    - fallback2`)
	assert.Contains(t, y, `test_map:
    key1: fallback1
    key2: fallback2`)

	// Test bracket notation with map keys containing slashes (user-reported issue)
	// https://atmos.tools/functions/yaml/terraform.state#handling-yq-expressions-with-bracket-notation-and-quotes
	t.Run("bracket notation with slashes in map keys", func(t *testing.T) {
		// Test with single quotes around the YQ expression (recommended syntax)
		d, err = processTagTerraformState(&atmosConfig, `!terraform.state component-1 '.secret_arns_map["auth0-event-stream/app/client-id"]'`, stack, nil)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123", d)

		// Test with bare brackets (also valid)
		d, err = processTagTerraformState(&atmosConfig, `!terraform.state component-1 .secret_arns_map["auth0-event-stream/app/client-id"]`, stack, nil)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123", d)

		// Test with stack parameter and single quotes
		d, err = processTagTerraformState(&atmosConfig, `!terraform.state component-1 nonprod '.secret_arns_map["auth0-event-stream/app/client-secret"]'`, stack, nil)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-secret-xyz789", d)
	})

	// Test the component-bracket-notation component resolution
	t.Run("component-bracket-notation describe", func(t *testing.T) {
		res, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "component-bracket-notation",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err = u.ConvertToYAML(res)
		assert.Nil(t, err)
		assert.Contains(t, y, "client_id_arn: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123")
		assert.Contains(t, y, "client_secret_arn: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-secret-xyz789")
		assert.Contains(t, y, "client_id_arn_bare: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123")
		assert.Contains(t, y, "client_id_arn_with_stack: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123")
	})
}

// TestYamlFuncTerraformState_RefreshesAfterDependencyDeploys is a regression test for a bug where
// terraformStateCache poisoned itself: reading a component's state BEFORE it was ever deployed
// cached a "not provisioned" result forever (a nil map[string]any boxed into the `any`-typed cache,
// which is non-nil as an interface even though the underlying map is nil - see GetTerraformState),
// so a LATER read of the same component after it deployed still incorrectly returned the stale
// "not provisioned" result instead of the real, now-available state. This is exactly the shape of a
// `deploy --all` DAG run: a downstream component's `!terraform.state` reference to an
// upstream/dependency component can be evaluated once before the dependency exists (e.g. while
// building the dependency graph), and must reflect the dependency's real state once it's deployed
// later in the same run - not the stale pre-deploy snapshot.
func TestYamlFuncTerraformState_RefreshesAfterDependencyDeploys(t *testing.T) {
	ResetStateCache()
	tfoutput.ResetOutputsCache()
	t.Cleanup(func() {
		ResetStateCache()
		tfoutput.ResetOutputsCache()
	})

	if _, lookErr := exec.LookPath("tofu"); lookErr != nil {
		if _, lookErr2 := exec.LookPath("terraform"); lookErr2 != nil {
			t.Skip("skipping: neither 'tofu' nor 'terraform' binary found in PATH (required for !terraform.state integration test)")
		}
	}
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "nonprod"
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function"
	setupTerraformYamlFunctionSandbox(t, workDir)
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Terraform can create an empty workspace state while running a cold plan. Cache
	// that empty state exactly as graph preflight does; the expression still errors
	// because foo is absent, but the empty map remains cached until component-1 deploys.
	emptyStatePath := filepath.Join(
		os.Getenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"),
		"component-1",
		"terraform.tfstate.d",
		stack,
		"terraform.tfstate",
	)
	require.NoError(t, os.MkdirAll(filepath.Dir(emptyStatePath), 0o755))
	require.NoError(t, os.WriteFile(emptyStatePath, []byte(`{"version":4,"terraform_version":"1.10.0","serial":0,"lineage":"empty-state","outputs":{},"resources":[]}`), 0o600))

	_, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	assert.Error(t, err, "the empty pre-deploy state has no foo output")

	// Now actually deploy component-1 in the same process. ExecuteTerraform must evict
	// the preflight snapshot so a downstream node does not keep its fallback value.
	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	// Re-read the same state. This must now return the real, freshly-deployed value - not
	// the stale "not provisioned" result from before the deploy.
	d, err := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	assert.NoError(t, err, "state read after deploy must succeed, not reuse the stale pre-deploy cache entry")
	assert.Equal(t, "component-1-a", d)
}
