package exec

import (
	"bytes"
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

const testTerraformPolicyManifest = `apiVersion: atmos/v1
kind: Policy
metadata: { name: platform }
spec:
  policies:
    no-delete:
      on: terraform.plan
      action: deny
      rule: "has(input.resource_changes) && input.resource_changes.exists(r, 'delete' in r.change.actions)"
`

func TestTerraformPolicyLoading(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	gate, err := loadTerraformPlanPolicy(&info)
	require.NoError(t, err)
	require.Nil(t, gate)
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	_, err = loadTerraformPlanPolicy(&info)
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	// Loading happens before Atmos config resolution, even without a usable stack.
	require.ErrorIs(t, ExecuteTerraform(info), errUtils.ErrPolicyInvalid)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(testTerraformPolicyManifest), 0o600))
	gate, err = loadTerraformPlanPolicy(&info)
	require.NoError(t, err)
	require.Len(t, gate.Rules, 1)
	for _, command := range []string{"plan", "apply", "deploy", "destroy"} {
		require.True(t, governedTerraformCommand(command))
	}
	for _, command := range []string{"show", "output", "init", "state list"} {
		info.SubCommand = command
		gate, err = loadTerraformPlanPolicy(&info)
		require.NoError(t, err)
		require.Nil(t, gate)
	}
	info = schema.ConfigAndStacksInfo{SubCommand: "plan", NeedHelp: true}
	gate, err = loadTerraformPlanPolicy(&info)
	require.NoError(t, err)
	require.Nil(t, gate)
}

func TestTerraformPolicyRequiresSavedPlan(t *testing.T) {
	gate := &planfile.Guard{}
	for _, command := range []string{"apply", "deploy", "destroy"} {
		info := schema.ConfigAndStacksInfo{SubCommand: command}
		require.ErrorIs(t, validateTerraformPolicyOperation(&info, gate), errUtils.ErrPolicyPlan)
		require.NoError(t, validateTerraformPolicyOperation(&info, nil))
		info.UseTerraformPlan = true
		if command != "destroy" {
			require.NoError(t, validateTerraformPolicyOperation(&info, gate))
		}
	}
	require.Error(t, validateTerraformPolicyCommand(&schema.ConfigAndStacksInfo{SubCommand: "apply -auto-approve"}))
	require.NoError(t, validateTerraformPolicyCommand(&schema.ConfigAndStacksInfo{SubCommand: "state list"}))
}

func TestTerraformPolicyAttachments(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand: "apply", ComponentFromArg: "app", Stack: "dev",
		ComponentSection: schema.AtmosSectionMapType{"metadata": map[string]any{"labels": map[string]any{"stage": "dev"}}},
	}
	newGate := func() *planfile.Guard {
		return &planfile.Guard{Rules: []policy.Rule{{ID: "platform/protect", Match: &policy.Selector{Labels: map[string]string{"stage": "prod"}}}}}
	}
	gate, err := bindTerraformPolicy(newGate(), &info)
	require.NoError(t, err)
	require.Nil(t, gate, "nonmatching instances must not inherit saved-plan restrictions")
	info.ComponentSection["metadata"] = map[string]any{"labels": map[string]any{"stage": "prod"}}
	_, err = bindTerraformPolicy(newGate(), &info)
	require.ErrorIs(t, err, errUtils.ErrPolicyPlan, "matching instances require an explicit saved plan")
	info.UseTerraformPlan = true
	gate, err = bindTerraformPolicy(newGate(), &info)
	require.NoError(t, err)
	require.Equal(t, "platform/protect", gate.Rules[0].ID)
	require.Equal(t, "app", gate.Subject.Name)
	require.Equal(t, "prod", gate.Subject.Metadata.Labels["stage"])
	info.ComponentSection["metadata"] = map[string]any{"labels": map[string]any{"stage": 42}}
	_, err = bindTerraformPolicy(newGate(), &info)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	info.ComponentSection["metadata"] = map[string]any{"labels": map[string]any{"stage": "{{ .vars.stage }}"}}
	_, err = bindTerraformPolicy(newGate(), &info)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestTerraformPolicyInvocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	config := schema.AtmosConfiguration{Env: map[string]string{"GLOBAL": "set"}}
	info := schema.ConfigAndStacksInfo{Command: "/trusted/terraform", ComponentEnvList: []string{"COMPONENT=set"}}
	actual, invocation := terraformPolicyInvocation(&config, &info, []string{"plan"}, "component", []ShellCommandOption{
		WithEnvironment([]string{"SANITIZED=set"}), WithProcessContext(ctx),
	})
	require.Same(t, ctx, actual)
	require.Contains(t, invocation.Env, "GLOBAL=set")
	require.Contains(t, invocation.Env, "COMPONENT=set")
	require.Contains(t, invocation.Env, "SANITIZED=set")
	require.Contains(t, invocation.Env, "TF_LOG_PATH=")
	require.Equal(t, "/trusted/terraform", invocation.Command)
	actual, invocation = terraformPolicyInvocation(&config, &info, nil, "", nil)
	require.NotNil(t, actual)
	require.NotEmpty(t, invocation.Env)
}

func TestTerraformPolicyDryRunAndNoGate(t *testing.T) {
	config := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan", DryRun: true}
	require.NoError(t, executeGovernedTerraformCommand(&config, &info, []string{"plan", "-out=p"}, t.TempDir(), false, nil))
	// An incomplete Guard would error if dry-run accidentally called extraction/evaluation.
	require.NoError(t, executeGovernedTerraformCommand(&config, &info, []string{"plan", "-out=p"}, t.TempDir(), false, &planfile.Guard{}))
	info.SubCommand = "apply"
	require.Error(t, executeGovernedTerraformCommand(&config, &info, []string{"apply"}, t.TempDir(), false, &planfile.Guard{}))
}

func TestTerraformPolicyRetryRequiresGate(t *testing.T) {
	config := schema.AtmosConfiguration{}
	config.CI.Enabled = true
	config.Components.Terraform.CI.ExitCodes = map[int]bool{0: true, 1: true, 2: true}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
	// An unconfigured guard must fail before launching a subprocess, including
	// the smart-init recovery retry. CI success mappings must not swallow it.
	err := retryMainCommand(&config, &info, []string{"plan", "-out=approved.plan"}, t.TempDir(), false, &planfile.Guard{})
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
}

// Exercise actual Terraform binary plans without cloud credentials or providers.
// Skip only when Terraform is not installed; unit tests cover the same failure contracts.
func TestTerraformPolicyRealPlanAndApply(t *testing.T) {
	terraform, err := osexec.LookPath("terraform")
	if err != nil {
		t.Skip("Terraform executable not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.tf"), []byte(`resource "terraform_data" "test" {
  input = sensitive("policy-test-secret")
}`), 0o600))
	result := process.DefaultRunner{}.Run(ctx, process.TaskSpec{Command: terraform, Args: []string{"init", "-input=false", "-no-color"}, Dir: directory})
	require.NoError(t, result.Err)
	var engine policy.Engine
	require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
	gate := &planfile.Guard{Engine: &engine, Show: process.DefaultRunner{}, Rules: []policy.Rule{{
		ID: "no-delete", On: policy.TerraformPlan, Engine: policy.CELEngine,
		Expression: `has(input.resource_changes) && input.resource_changes.exists(r, 'delete' in r.change.actions)`,
		Effects:    policy.Effects{Deny: true},
	}}}
	config := schema.AtmosConfiguration{}
	config.CI.Enabled = true
	// Even a permissive remapping must not swallow a policy denial.
	config.Components.Terraform.CI.ExitCodes = map[int]bool{0: true, 1: true, 2: true}
	info := schema.ConfigAndStacksInfo{SubCommand: "plan", Command: terraform}
	var captured bytes.Buffer
	opts := []ShellCommandOption{WithStdoutOverride(&captured), WithStdoutCapture(&captured), WithProcessContext(ctx)}
	args := []string{"plan", "-input=false", "-no-color", "-detailed-exitcode", "-out=approved.plan"}
	require.NoError(t, executeGovernedTerraformCommand(&config, &info, args, directory, false, gate, opts...))
	require.FileExists(t, filepath.Join(directory, "approved.plan"))
	require.NotContains(t, captured.String(), "policy-test-secret")
	require.NotContains(t, captured.String(), `"format_version"`)
	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	require.NoError(t, executeGovernedTerraformCommand(&config, &info, []string{"apply", "-no-color", "approved.plan"}, directory, false, gate, opts...))
	require.FileExists(t, filepath.Join(directory, "terraform.tfstate"))
	// A fresh no-change plan must still be evaluated and pass.
	info.SubCommand = "plan"
	require.NoError(t, executeGovernedTerraformCommand(&config, &info, args, directory, false, gate, opts...))
	// A destroy plan is denied even though Terraform itself exits successfully (2).
	args = append(args, "-destroy")
	require.ErrorIs(t, executeGovernedTerraformCommand(&config, &info, args, directory, false, gate, opts...), errUtils.ErrPolicyDenied)
	// Changed governing policy is re-evaluated before consuming a previously accepted plan.
	gate.Rules[0].Expression = "true"
	info.SubCommand = "apply"
	require.ErrorIs(t, executeGovernedTerraformCommand(&config, &info, []string{"apply", "approved.plan"}, directory, false, gate, opts...), errUtils.ErrPolicyDenied)
	// Smart-init retries must retain the same governing gate and reject a saved
	// artifact under the changed policy rather than taking the ungoverned path.
	require.ErrorIs(t, retryMainCommand(&config, &info, []string{"apply", "approved.plan"}, directory, false, gate, opts...), errUtils.ErrPolicyDenied)
	files, err := filepath.Glob(filepath.Join(directory, ".atmos-*"))
	require.NoError(t, err)
	require.Empty(t, files)
}
