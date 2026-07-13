package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
)

// TestJITSource_OCIScheme is a regression test for
// https://github.com/cloudposse/atmos/issues/2716.
//
// JIT auto-provisioning (triggered by the before.terraform.init hook) failed
// for oci:// sources with go-getter's "download not supported for scheme
// 'oci'" error, even though the same source works with `atmos vendor pull`.
// This test reproduces the exact provision.workdir.enabled + source.uri:
// oci://... configuration from the issue, against an in-process fake
// registry (no real network or registry dependency, so it's safe and fast
// in CI).
func TestJITSource_OCIScheme(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	imageRef := ocitest.NewRegistry(t, "test/component:v1", map[string]string{
		"context.tf": "# OCI_JIT_MARKER\n",
	})

	atmosYAML := `base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
    init_run_reconfigure: true
    auto_generate_backend_file: false
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_pattern: "{stage}"
logs:
  file: "/dev/stderr"
  level: Info
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(atmosYAML), 0o644))

	stackYAML := `vars:
  stage: dev
components:
  terraform:
    oci-component:
      source:
        uri: "oci://` + imageRef + `"
      provision:
        workdir:
          enabled: true
      vars:
        enabled: true
`
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks", "deploy"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "deploy", "dev.yaml"), []byte(stackYAML), 0o644))

	cmd.RootCmd.SetArgs([]string{
		"terraform", "plan", "oci-component",
		"--stack", "dev",
		"--dry-run",
	})
	// Execute may error since terraform isn't configured here, but JIT source
	// provisioning should have already run before that point is reached.
	_ = cmd.Execute()

	workdirPath := filepath.Join(root, ".workdir", "terraform", "dev-oci-component")
	info, err := os.Stat(workdirPath)
	require.NoError(t, err, "workdir should exist at %s", workdirPath)
	require.True(t, info.IsDir(), "workdir path should be a directory")

	content, err := os.ReadFile(filepath.Join(workdirPath, "context.tf"))
	require.NoError(t, err, "context.tf should exist in workdir (provisioned from the OCI source)")
	require.Contains(t, string(content), "OCI_JIT_MARKER")
}
