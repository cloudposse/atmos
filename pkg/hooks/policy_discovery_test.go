package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPolicyAttachmentsWithoutWorkspaceHooks(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv("ATMOS_POLICY_DIR", filepath.Join(root, "policy.d"))
	for _, directory := range []string{"stacks", "components/terraform/app", "policy.d"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, directory), 0o700))
	}
	for path, contents := range map[string]string{
		"atmos.yaml": `base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["**/*"]
  name_pattern: "{stage}"
`,
		"stacks/prod.yaml": `vars: {stage: prod}
components:
  terraform:
    app:
      metadata:
        labels: {owner: platform}
`,
		"policy.d/guard.yaml": `apiVersion: atmos/v1
kind: Policy
metadata: {name: guard}
spec:
  policies:
    protected:
      on: operation
      match:
        labels: {owner: platform}
      rule: "true"
      action: deny
      reason: Platform approval required.
`,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(contents), 0o600))
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "prod", ComponentType: "terraform"}
	config, err := cfg.InitCliConfig(*info, true)
	require.NoError(t, err)
	h, err := GetHooks(&config, info)
	require.NoError(t, err)
	require.Empty(t, h.items)
	require.True(t, h.HasHooks(), "governing policies apply without user-defined hooks")
	require.ErrorIs(t, h.RunAll(BeforeTerraformPlan, &config, info, nil, nil), errUtils.ErrPolicyDenied)
}
