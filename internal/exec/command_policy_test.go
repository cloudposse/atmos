package exec

import (
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestComponentAdmissionAttachments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(`apiVersion: atmos/v1
kind: Policy
metadata: {name: admission}
spec:
  policies:
    production:
      on: component.command
      action: deny
      match: {labels: {stage: prod}}
      rule: input.command == 'deploy'
`), 0o600))
	for _, kind := range []string{"terraform", "helmfile", "helm", "kubernetes", "packer", "container", "ansible", "emulator", "custom"} {
		t.Run(kind, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{ComponentType: kind, SubCommand: "deploy", ComponentFromArg: "app", Stack: "prod", ComponentSection: map[string]any{"metadata": map[string]any{"labels": map[string]any{"stage": "prod"}}}}
			require.ErrorIs(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info), errUtils.ErrPolicyDenied)
			info.SubCommand = "describe"
			require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
			info.SubCommand = "deploy"
			info.ComponentSection["metadata"] = map[string]any{"labels": map[string]any{"stage": "dev"}}
			require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
			info.ComponentSection["metadata"] = map[string]any{"labels": map[string]any{"stage": "{{ .vars.stage }}"}}
			require.Error(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info), "unresolved metadata cannot detach a rule")
		})
	}
}

func TestFunctionPolicyBlocksLazyExec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	_, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, schema.AtmosSectionMapType{"nested": map[string]any{"value": "!exec should-never-run"}}, "prod", nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	value, err := processCustomTags(&schema.AtmosConfiguration{}, "!exec should-never-run", "prod", []string{"exec"}, nil)
	require.NoError(t, err)
	require.Equal(t, "!exec should-never-run", value, "skipping execution is safe")
}
