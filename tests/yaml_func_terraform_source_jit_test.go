package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// toFileURI builds a cross-platform file:// URI that go-getter understands.
// On Unix the absolute path already starts with "/", yielding "file:///tmp/...".
// On Windows the slashified path is "D:/...", so a leading "/" is prepended
// to produce "file:///D:/..." (RFC 8089 form that go-getter parses correctly).
func toFileURI(absPath string) string {
	p := filepath.ToSlash(absPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// seedStateFile writes the hermetic pre-seeded state file for producer-from-source.
// This ensures test isolation — if a prior test (e.g. TestTerraformOutputJITWorkdirFromSource)
// deleted or replaced the workdir, the state test still has its file.
func seedStateFile(t *testing.T, cwd string) {
	t.Helper()

	const stateContent = `{
  "version": 4,
  "terraform_version": "1.13.1",
  "serial": 1,
  "lineage": "hermetic-test-producer-from-source",
  "outputs": {
    "foo": {
      "value": "foo-from-source-module",
      "type": "string"
    },
    "source": {
      "value": "hydrated-from-local-source",
      "type": "string"
    }
  },
  "resources": []
}
`
	stateDir := filepath.Join(cwd, ".workdir", "terraform", "test-producer-from-source", "terraform.tfstate.d", "test")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte(stateContent), 0o644))
}

// TestTerraformStateJITWorkdirFromSource verifies that !terraform.state resolves
// correctly when the producer component is configured with both source.uri and
// provision.workdir.enabled: true.
//
// The state file lives in the JIT workdir (.workdir/terraform/test-producer-from-source/),
// not in components/terraform/. resolveLocalBackendComponentPath must derive the JIT path
// even though the component was hydrated from an external source URI rather than a local
// components/ directory.
//
// This is a hermetic test — no terraform/tofu binary required. The state file is
// written by seedStateFile at test start for isolation from other tests that may
// delete the workdir (e.g. TestTerraformOutputJITWorkdirFromSource).
//
// Regression test for https://github.com/cloudposse/atmos/issues/2167.
func TestTerraformStateJITWorkdirFromSource(t *testing.T) {
	t.Chdir("./fixtures/scenarios/terraform-output-jit-workdir")

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Seed the state file regardless of prior test runs that may have removed the workdir.
	seedStateFile(t, cwd)

	// Set ATMOS_SOURCE_DIR so the Go template in source.uri resolves.
	// The value does not affect !terraform.state resolution (which only needs
	// IsWorkdirEnabled + BuildPath), but an unresolvable template would cause
	// stack processing to fail before we reach the state lookup.
	srcDir := toFileURI(filepath.Join(cwd, "source-modules", "mock-alt"))
	t.Setenv("ATMOS_SOURCE_DIR", srcDir)

	e.ResetStateCache()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.True(t, atmosConfig.Initialized)

	// consumer-from-source-state has:
	//   vars.foo: !terraform.state producer-from-source test foo
	//
	// producer-from-source has provision.workdir.enabled: true + source.uri.
	// resolveLocalBackendComponentPath must derive the JIT workdir path
	// (.workdir/terraform/test-producer-from-source/) and find the pre-seeded
	// state file there.
	componentSection, err := e.ExecuteDescribeComponent(
		&e.ExecuteDescribeComponentParams{
			Component:            "consumer-from-source-state",
			Stack:                "test",
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		},
	)
	require.NoError(t, err, "!terraform.state should resolve for source-provisioned JIT workdir component")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]interface{})
	require.True(t, ok, "vars should be a map")

	assert.Equal(t, "foo-from-source-module", vars["foo"],
		"vars.foo should be resolved from pre-populated JIT workdir state file for source-provisioned component")
}

// TestTerraformOutputJITWorkdirFromSource verifies that !terraform.output automatically
// provisions a JIT workdir by hydrating it from a source URI before running terraform init.
//
// The key assertion is that the workdir is created AND populated from the source URI
// (source-modules/mock-alt/), not from components/terraform/. The extra output "source"
// declared only in mock-alt's main.tf proves the hydration came from the right place.
//
// Regression test for https://github.com/cloudposse/atmos/issues/2167.
func TestTerraformOutputJITWorkdirFromSource(t *testing.T) {
	RequireTerraformOrTofu(t)

	// go-getter's FileGetter creates a symlink at the destination when fetching
	// file:// URIs. Windows CI runners frequently lack the privilege required to
	// create symlinks (SeCreateSymbolicLinkPrivilege), causing the fetch to fail
	// without a visible error message. Real-world source URIs are typically
	// remote (github.com, s3://) and go-getter copies rather than symlinks for
	// those, so the JIT-workdir-from-source behavior this test verifies is still
	// covered by Linux and macOS CI. See pkg/downloader/git_getter_test.go for
	// the same Windows-symlink skip pattern used elsewhere.
	if runtime.GOOS == "windows" {
		t.Skip("skipping file:// source fetch on Windows (go-getter symlink requirement)")
	}

	t.Chdir("./fixtures/scenarios/terraform-output-jit-workdir")

	cwd, err := os.Getwd()
	require.NoError(t, err)

	srcDir := toFileURI(filepath.Join(cwd, "source-modules", "mock-alt"))
	t.Setenv("ATMOS_SOURCE_DIR", srcDir)

	// Reset caches for test isolation.
	tfoutput.ResetOutputsCache()
	tfoutput.ResetWorkdirProvisionCache()
	e.ResetStateCache()

	workdirPath := filepath.Join(cwd, ".workdir", "terraform", "test-producer-from-source")

	// Remove any prior workdir so we verify it is freshly created.
	require.NoError(t, os.RemoveAll(workdirPath))

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.True(t, atmosConfig.Initialized)

	// Describing consumer-from-source triggers !terraform.output producer-from-source test foo.
	// This should auto-provision producer-from-source's workdir from the source URI before init.
	componentSection, describeErr := e.ExecuteDescribeComponent(
		&e.ExecuteDescribeComponentParams{
			Component:            "consumer-from-source",
			Stack:                "test",
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		},
	)

	// Primary assertion: workdir was created by source-aware auto-provision.
	_, statErr := os.Stat(workdirPath)
	require.NoError(t, statErr, "workdir should have been auto-provisioned from source URI before terraform init")

	// Secondary assertion: workdir contains main.tf hydrated from source-modules/mock-alt,
	// not from components/terraform/mock. The extra "source" output is the distinguishing marker.
	mainTF, readErr := os.ReadFile(filepath.Join(workdirPath, "main.tf"))
	require.NoError(t, readErr, "main.tf should exist in workdir after source hydration")
	assert.True(t, strings.Contains(string(mainTF), `output "source"`),
		fmt.Sprintf("workdir main.tf should contain the extra 'source' output from mock-alt, got:\n%s", string(mainTF)))

	// Tertiary: if describe succeeded, the section should be non-nil.
	if describeErr == nil {
		assert.NotNil(t, componentSection)
	}

	t.Cleanup(func() { os.RemoveAll(workdirPath) })
}
