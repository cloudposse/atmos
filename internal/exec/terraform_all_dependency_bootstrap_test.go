package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteTerraformAll_UnprovisionedDependencyDoesNotFailPreflight reproduces the reported
// regression: on a fresh environment, `atmos terraform apply --all` failed during the
// describe-stacks preflight because a dependent component's `!terraform.state` reference to a
// not-yet-applied component (e.g. `kms`) was resolved strictly, before the scheduler ever got a
// chance to apply that dependency. The dependency graph itself is built from the static
// `dependencies`/`settings.depends_on` sections (see TestBuildTerraformDependencyGraph), never
// from resolved vars, so the preflight only needs to tolerate — not correctly resolve — this
// kind of reference. DryRun keeps the test hermetic: no Terraform binary is invoked, and no
// state file exists for either component, matching a genuinely fresh environment.
func TestExecuteTerraformAll_UnprovisionedDependencyDoesNotFailPreflight(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "kms"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "audit-trail"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`
base_path: "."
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - "**/*"
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(`
components:
  terraform:
    kms:
      vars: {}
    audit-trail:
      dependencies:
        components:
          - name: kms
      vars:
        kms_key_arn: !terraform.state kms dev key_arn
`), 0o600))

	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	os.Unsetenv("ATMOS_BASE_PATH")
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	t.Chdir(root)

	err := ExecuteTerraformAll(&schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentType:    "terraform",
		SubCommand:       "apply",
		DryRun:           true,
		ProcessTemplates: true,
		ProcessFunctions: true,
	})
	assert.NoError(t, err)
}
