package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
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

const componentAdmissionDenyManifest = `apiVersion: atmos/v1
kind: Policy
metadata: {name: admission}
spec:
  policies:
    block:
      on: component.command
      action: deny
      rule: input.component == 'vpc'
`

func TestComponentAdmissionWithoutRules(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "apply", ComponentFromArg: "vpc", Stack: "prod"}

	t.Run("no policy directory", func(t *testing.T) {
		t.Setenv("ATMOS_POLICY_DIR", "")
		t.Chdir(t.TempDir())
		require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
	})

	t.Run("directory without component rules", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("ATMOS_POLICY_DIR", dir)
		manifest := strings.ReplaceAll(componentAdmissionDenyManifest, "component.command", "function")
		manifest = strings.ReplaceAll(manifest, "input.component == 'vpc'", "input.name == 'exec'")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(manifest), 0o600))
		require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
	})

	t.Run("unreadable policy directory fails closed", func(t *testing.T) {
		t.Setenv("ATMOS_POLICY_DIR", filepath.Join(t.TempDir(), "missing"))
		require.ErrorIs(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info), errUtils.ErrPolicyInvalid)
	})
}

func TestComponentAdmissionRejectsMalformedMetadata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(componentAdmissionDenyManifest), 0o600))
	info := &schema.ConfigAndStacksInfo{
		ComponentType: "terraform", SubCommand: "apply", ComponentFromArg: "other", Stack: "prod",
		ComponentSection: map[string]any{"metadata": map[string]any{"labels": map[string]any{"stage": 5}}},
	}
	// Labels must be strings. Malformed metadata cannot silently detach a rule.
	require.ErrorIs(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info), errUtils.ErrPolicyInputInvalid)

	info.ComponentSection["metadata"] = map[string]any{"labels": map[string]any{"stage": "prod"}}
	require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
}

func TestProcessStacksEnforcesComponentAdmission(t *testing.T) {
	atmosConfig := setupSharedCacheFixture(t)
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "my-explicit-stack",
		ComponentType:    cfg.TerraformComponentType,
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(componentAdmissionDenyManifest), 0o600))
	_, err := ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)

	// The same invocation is admitted when the rule targets a different component.
	other := strings.ReplaceAll(componentAdmissionDenyManifest, "'vpc'", "'not-vpc'")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(other), 0o600))
	_, err = ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
	require.NoError(t, err)
}

func TestFunctionPolicyBlocksLazyExec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	_, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, schema.AtmosSectionMapType{"nested": map[string]any{"value": taggedFunctionForTest("!exec should-never-run")}}, "prod", nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	value, err := processCustomTags(&schema.AtmosConfiguration{}, "!exec should-never-run", "prod", []string{"exec"}, nil)
	require.NoError(t, err)
	require.Equal(t, "!exec should-never-run", value, "skipping execution is safe")
}

func TestComponentAdmissionDestroyInput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(`apiVersion: atmos/v1
kind: Policy
metadata: {name: no-destroy}
spec:
  policies:
    guard:
      on: component.command
      action: deny
      rule: input.destroy
`), 0o600))
	tests := []struct {
		name          string
		componentType string
		subCommand    string
		cliArgs       []string
		args          []string
		denied        bool
	}{
		{name: "terraform destroy", componentType: "terraform", subCommand: "destroy", denied: true},
		{name: "apply -destroy", componentType: "terraform", subCommand: "apply", args: []string{"-destroy", "-auto-approve"}, denied: true},
		{name: "apply --destroy=true", componentType: "terraform", subCommand: "apply", args: []string{"--destroy=true"}, denied: true},
		{name: "apply -destroy=1 in cli args", componentType: "terraform", subCommand: "apply", cliArgs: []string{"-destroy=1"}, denied: true},
		{name: "plan -destroy", componentType: "terraform", subCommand: "plan", args: []string{"-destroy"}, denied: true},
		{name: "deploy -destroy", componentType: "terraform", subCommand: "deploy", args: []string{"-destroy"}, denied: true},
		{name: "apply after separator", componentType: "terraform", subCommand: "apply", args: []string{"--", "-destroy"}, denied: true},
		{name: "apply -destroy=false", componentType: "terraform", subCommand: "apply", args: []string{"-destroy=false", "-auto-approve"}},
		{name: "plain apply", componentType: "terraform", subCommand: "apply", args: []string{"-auto-approve"}},
		{name: "plain plan", componentType: "terraform", subCommand: "plan"},
		{name: "helmfile destroy", componentType: "helmfile", subCommand: "destroy", denied: true},
		{name: "helmfile sync", componentType: "helmfile", subCommand: "sync", args: []string{"-destroy"}},
		{name: "packer ignores flag", componentType: "packer", subCommand: "build", args: []string{"-destroy"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{
				ComponentType: tc.componentType, SubCommand: tc.subCommand, ComponentFromArg: "app", Stack: "dev",
				CliArgs: tc.cliArgs, AdditionalArgsAndFlags: tc.args,
			}
			err := enforceComponentAdmission(&schema.AtmosConfiguration{}, info)
			if tc.denied {
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
