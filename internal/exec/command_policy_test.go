package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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

const componentAdmissionDenyDestroyManifest = `apiVersion: atmos/v1
kind: Policy
metadata: {name: no-destroy}
spec:
  policies:
    guard:
      on: component.command
      action: deny
      rule: input.destroy
`

// TestComponentAdmissionRunsBeforeAuthentication verifies that a denied command
// fails before Atmos resolves or logs in to any identity. Before the fix,
// `terraform destroy` authenticated first, so an expired SSO session masked the
// denial and a valid one logged in to production before the policy ran.
func TestComponentAdmissionRunsBeforeAuthentication(t *testing.T) {
	newInfo := func(subCommand string, args ...string) schema.ConfigAndStacksInfo {
		return schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc", Stack: "my-explicit-stack", ComponentType: cfg.TerraformComponentType,
			SubCommand: subCommand, AdditionalArgsAndFlags: args,
		}
	}
	countAuth := func(t *testing.T) *int {
		t.Helper()
		calls := 0
		orig := defaultAuthManagerCreator
		t.Cleanup(func() { defaultAuthManagerCreator = orig })
		defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, _ auth.ReExecContext) (auth.AuthManager, error) {
			calls++
			return nil, nil
		}
		return &calls
	}
	setup := func(t *testing.T) schema.AtmosConfiguration {
		t.Helper()
		atmosConfig := setupSharedCacheFixture(t)
		dir := t.TempDir()
		t.Setenv("ATMOS_POLICY_DIR", dir)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(componentAdmissionDenyDestroyManifest), 0o600))
		return atmosConfig
	}

	for _, tc := range []struct {
		name string
		info schema.ConfigAndStacksInfo
	}{
		{name: "destroy", info: newInfo("destroy")},
		{name: "plan -destroy", info: newInfo("plan", "-destroy")},
	} {
		t.Run("setupTerraformAuth denies "+tc.name+" without logging in", func(t *testing.T) {
			atmosConfig := setup(t)
			calls := countAuth(t)
			info := tc.info
			_, err := setupTerraformAuth(&atmosConfig, &info)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Zero(t, *calls, "auth must not run for a denied command")
		})
		t.Run("createAndAuthenticateAuthManager denies "+tc.name+" without logging in", func(t *testing.T) {
			atmosConfig := setup(t)
			calls := countAuth(t)
			info := tc.info
			_, err := createAndAuthenticateAuthManager(&atmosConfig, &info)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Zero(t, *calls, "auth must not run for a denied command")
		})
	}

	t.Run("admitted command still authenticates", func(t *testing.T) {
		atmosConfig := setup(t)
		calls := countAuth(t)
		info := newInfo("plan")
		_, err := setupTerraformAuth(&atmosConfig, &info)
		require.NoError(t, err)
		require.Equal(t, 1, *calls)
	})
}

func TestAdmitComponentLeavesNonPolicyErrorsToExecutor(t *testing.T) {
	atmosConfig := setupSharedCacheFixture(t)
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(componentAdmissionDenyDestroyManifest), 0o600))

	// Without a stack there is no subject to admit yet.
	require.NoError(t, AdmitComponent(&atmosConfig, &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc", ComponentType: cfg.TerraformComponentType, SubCommand: "destroy",
	}, nil))
	// A component that does not resolve is reported by the executor, not here.
	require.NoError(t, AdmitComponent(&atmosConfig, &schema.ConfigAndStacksInfo{
		ComponentFromArg: "does-not-exist", Stack: "my-explicit-stack", ComponentType: cfg.TerraformComponentType, SubCommand: "destroy",
	}, nil))
}

// writeAdmissionWorkspaceFixture writes a project whose `vpc` component renders
// metadata.terraform_workspace_template with a template that always fails, standing
// in for a template that reads an identity-backed store before any login.
func writeAdmissionWorkspaceFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform", "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`base_path: "."
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths: ["**/*"]
  name_pattern: "{stage}"
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "prod.yaml"), []byte(`vars:
  stage: prod
components:
  terraform:
    vpc:
      metadata:
        terraform_workspace_template: '{{ fail "store requires login" }}'
`), 0o600))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestComponentAdmissionRunsBeforeWorkspaceTemplate verifies that a workspace
// template failure cannot mask a denial: AdmitComponent ignores non-policy errors,
// so a template evaluated before admission would let the caller log in first.
func TestComponentAdmissionRunsBeforeWorkspaceTemplate(t *testing.T) {
	atmosConfig := writeAdmissionWorkspaceFixture(t)
	policyDir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", policyDir)
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc", Stack: "prod", ComponentType: cfg.TerraformComponentType, SubCommand: "shell",
	}

	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "guard.yaml"), []byte(componentAdmissionDenyManifest), 0o600))
	_, err := ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.ErrorIs(t, AdmitComponent(&atmosConfig, &info, nil), errUtils.ErrPolicyDenied)

	// An admitted invocation still reaches the workspace template and reports its failure.
	other := strings.ReplaceAll(componentAdmissionDenyManifest, "'vpc'", "'not-vpc'")
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "guard.yaml"), []byte(other), 0o600))
	_, err = ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Contains(t, err.Error(), "store requires login")
}

// TestComponentAdmissionCommandPath verifies that component admission carries the
// CLI command path: executors fall back to the component type and subcommands, and
// describe callers such as `secret shell` name their own command.
func TestComponentAdmissionCommandPath(t *testing.T) {
	t.Run("fallback and explicit paths", func(t *testing.T) {
		require.Equal(t, []string{"terraform", "shell"}, componentCommandPath(&schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "shell"}))
		require.Equal(t, []string{"terraform", "providers", "lock"}, componentCommandPath(&schema.ConfigAndStacksInfo{
			ComponentType: "terraform", SubCommand: "providers", SubCommand2: "lock",
		}))
		require.Equal(t, []string{"terraform", "apply"}, componentCommandPath(&schema.ConfigAndStacksInfo{
			ComponentType: "terraform", SubCommand: "apply", CliArgs: []string{"terraform", "apply", ""},
		}))
		require.Equal(t, []string{"secret", "shell"}, componentCommandPath(&schema.ConfigAndStacksInfo{
			ComponentType: "terraform", SubCommand: "shell", CliArgs: []string{"secret", "shell"},
		}))
	})

	t.Run("describe callers name their command", func(t *testing.T) {
		atmosConfig := setupSharedCacheFixture(t)
		dir := t.TempDir()
		t.Setenv("ATMOS_POLICY_DIR", dir)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte(`apiVersion: atmos/v1
kind: Policy
metadata: {name: no-secret-shell}
spec:
  policies:
    guard:
      on: component.command
      action: deny
      rule: input.command == 'shell' && input.command_path == ['secret', 'shell']
`), 0o600))
		describe := func(path []string) error {
			_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
				AtmosConfig: &atmosConfig, Component: "vpc", Stack: "my-explicit-stack", CommandPath: path,
			})
			return err
		}

		require.ErrorIs(t, describe([]string{"secret", "shell"}), errUtils.ErrPolicyDenied)
		require.NoError(t, describe([]string{"secret", "get"}))
		require.NoError(t, describe(nil), "plain describe component is not a shell")
	})
}
