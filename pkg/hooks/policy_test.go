package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPolicyHooksLifecycle(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	definition, err := json.Marshal(map[string]any{"kind": "step", "type": "http", "with": map[string]any{"url": server.URL, "method": "POST"}})
	require.NoError(t, err)
	for _, event := range []HookEvent{AfterTerraformPlan, AfterHelmApply, AfterHelmfileApply, AfterKubernetesApply} {
		t.Run(string(event), func(t *testing.T) {
			h := policyTestHooks(event, definition)
			cmd := &cobra.Command{}
			cmd.Flags().String("skip-hooks", "", "")
			require.NoError(t, cmd.Flags().Set("skip-hooks", "*"))
			require.True(t, NewSkipPredicate(ResolveSkipHooks(cmd))("test/callback"))
			info := &schema.ConfigAndStacksInfo{Stack: "prod", ComponentFromArg: "app"}
			config := &schema.AtmosConfiguration{}
			before := calls.Load()
			// Required hooks execute even when every workspace hook is skipped.
			require.NoError(t, h.RunAll(event, config, info, cmd, nil))
			require.Equal(t, before+1, calls.Load())
			h.policyBundle.Rules[0].Effects.Hooks[0].Required = false
			require.NoError(t, h.RunAll(event, config, info, cmd, nil))
			require.Equal(t, before+1, calls.Load(), "optional hook honors the skip flag")
			h.policyBundle.Rules[0].Effects.Hooks[0].Required = true
			// A different lifecycle point, including apply's plan recheck, does not replay it.
			require.NoError(t, h.RunAll(BeforeTerraformInit, config, info, cmd, nil))
			require.Equal(t, before+1, calls.Load())
			info.DryRun = true
			require.NoError(t, h.RunAll(event, config, info, cmd, nil))
			require.Equal(t, before+1, calls.Load())
			info.DryRun = false
			h.policyBundle.Rules = append(h.policyBundle.Rules, policy.Rule{ID: "deny", On: policy.Operation, Engine: policy.CELEngine, Expression: "true", Effects: policy.Effects{Deny: true}})
			require.ErrorIs(t, h.RunAll(event, config, info, cmd, nil), errUtils.ErrPolicyDenied)
			require.Equal(t, before+1, calls.Load())
		})
	}
}

func TestPolicyHookFailuresBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer server.Close()
	failedHTTP, err := json.Marshal(map[string]any{"kind": "step", "type": "http", "on_failure": "ignore", "with": map[string]any{"url": server.URL}})
	require.NoError(t, err)
	for name, definition := range map[string]json.RawMessage{
		"failed post apply":  failedHTTP,
		"unknown kind":       json.RawMessage(`{"kind":"not-registered"}`),
		"skip condition":     json.RawMessage(`{"kind":"command","command":"true","when":false}`),
		"nested skip":        json.RawMessage(`{"kind":"step","type":"http","with":{"url":"https://example.invalid","when":false}}`),
		"background":         json.RawMessage(`{"kind":"step","type":"http","with":{"background":true}}`),
		"invalid definition": json.RawMessage(`{`),
	} {
		t.Run(name, func(t *testing.T) {
			h := policyTestHooks(AfterHelmApply, definition)
			require.ErrorIs(t, h.RunAll(AfterHelmApply, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, nil), errUtils.ErrPolicyRequirements)
		})
	}
	h := policyTestHooks(AfterHelmApply, failedHTTP)
	h.policyBundle.Rules[0].Effects.Hooks[0].RequireSuccess = false
	// Must execute does not imply must succeed; the governing ignore mode is respected.
	require.NoError(t, h.RunAll(AfterHelmApply, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, nil))
	delete(h.policyBundle.Hooks, "test/callback")
	require.ErrorIs(t, h.RunAll(AfterHelmApply, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, nil), errUtils.ErrPolicyRequirements)
}

func TestPolicyHookEvaluationFailureBlocksWithoutLeakingInput(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	definition, err := json.Marshal(map[string]any{"kind": "step", "type": "http", "with": map[string]any{"url": server.URL}})
	require.NoError(t, err)
	run := func(h *Hooks) error {
		return h.RunAll(AfterHelmApply, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, nil)
	}

	broken := policyTestHooks(AfterHelmApply, definition)
	broken.policyBundle.Rules[0].Expression = "input.missing.field == 'x'"
	err = run(broken)
	require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
	require.ErrorContains(t, err, "input details withheld")
	require.NotContains(t, err.Error(), "missing.field", "evaluation errors must not echo expression or input details")
	require.Zero(t, calls.Load(), "evaluation failure must block hook execution")

	// A rule that evaluates cleanly never reports an evaluation failure.
	healthy := policyTestHooks(AfterHelmApply, definition)
	require.NoError(t, run(healthy))
	require.EqualValues(t, 1, calls.Load(), "a healthy policy must execute its required hook")
}

func TestPolicyHookApprovalRequiresVerifyingProvider(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	h := policyTestHooks(AfterHelmApply, json.RawMessage(`{"kind":"command","command":"true"}`))
	spec := json.RawMessage(`{"reviewers":["alice"],"minimum":1}`)
	h.policyBundle.Rules[0].Effects.Approvals = []policy.ApprovalRequirement{{ID: "platform", Provider: "github-actions", Spec: spec}}
	run := func() error {
		return h.RunAll(AfterHelmApply, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, nil)
	}

	// Without a CI provider that can verify reviews, a pending approval fails closed.
	err := run()
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)

	// A rule that does not match never needs a provider.
	h.policyBundle.Rules[0].Expression = "false"
	require.NoError(t, run())
}

func policyTestHooks(event HookEvent, definition json.RawMessage) *Hooks {
	return &Hooks{policyBundle: &policy.Bundle{
		Hooks: map[string]json.RawMessage{"test/callback": definition},
		Rules: []policy.Rule{{
			ID: "callback", On: policy.Operation, Engine: policy.CELEngine,
			Expression: "input.event == '" + string(event) + "'", Effects: policy.Effects{Hooks: []policy.HookRequirement{
				{Ref: "test/callback", Event: string(event), Required: true, RequireSuccess: true},
			}},
		}},
	}}
}

func TestPolicyHookComponentPaths(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath:  filepath.Join(base, "terraform"),
		HelmDirAbsolutePath:       filepath.Join(base, "helm"),
		HelmfileDirAbsolutePath:   filepath.Join(base, "helmfile"),
		KubernetesDirAbsolutePath: filepath.Join(base, "kubernetes"),
	}
	for _, componentType := range []string{"terraform", "helm", "helmfile", "kubernetes"} {
		ctx := &ExecContext{AtmosConfig: config, Info: &schema.ConfigAndStacksInfo{ComponentType: componentType, ComponentFromArg: "app"}}
		require.Equal(t, filepath.Join(base, componentType, "app"), ComponentPath(ctx))
	}
}

func TestPolicyHookAttachments(t *testing.T) {
	for _, event := range []HookEvent{BeforeTerraformPlan, BeforeHelmApply, BeforeHelmfileApply, BeforeKubernetesApply} {
		t.Run(string(event), func(t *testing.T) {
			h := &Hooks{policyBundle: &policy.Bundle{Rules: []policy.Rule{{
				ID: "platform/deny", On: policy.Operation, Engine: policy.CELEngine, Expression: "true",
				Match: &policy.Selector{Labels: map[string]string{"stage": "prod"}}, Effects: policy.Effects{Deny: true},
			}}}, sections: map[string]any{"metadata": map[string]any{"labels": map[string]any{"stage": "prod"}}}}
			info := &schema.ConfigAndStacksInfo{Stack: "test", ComponentFromArg: "app"}
			config := &schema.AtmosConfiguration{}
			require.ErrorIs(t, h.RunAll(event, config, info, nil, nil), errUtils.ErrPolicyDenied)
			h.sections["metadata"] = map[string]any{"labels": map[string]any{"stage": "dev"}}
			require.NoError(t, h.RunAll(event, config, info, nil, nil))
			h.sections["metadata"] = map[string]any{"labels": map[string]any{"stage": 42}}
			require.ErrorIs(t, h.RunAll(event, config, info, nil, nil), errUtils.ErrPolicyInputInvalid)
			h.sections = nil
			require.ErrorIs(t, h.RunAll(event, config, info, nil, nil), errUtils.ErrPolicyEvaluation)
		})
	}
}

func TestPolicyHooksGoverningSourceAndInputs(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	config, info := &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}
	h, err := GetHooks(config, info)
	require.NoError(t, err)
	require.False(t, h.HasHooks())
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	_, err = GetHooks(config, info)
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	data, err := os.ReadFile("../../examples/policies/lifecycle/policy.yaml")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), data, 0o600))
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", "")
	h, err = GetHooks(config, info)
	require.NoError(t, err)
	require.True(t, h.HasHooks(), "governing policies run even without workspace hooks")
	require.Equal(t, "github-actions", h.policyCI.Provider)
	// Later component env cannot replace the captured governing bundle/snapshot.
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	require.NoError(t, h.RunAll(BeforeTerraformInit, config, info, nil, nil))
	_, err = operationPolicyRequest(&hookRunContext{}, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	_, err = operationPolicyRequest(&hookRunContext{info: info, event: "unknown"}, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	h.policyBundle.Rules[0].RequiredCI = []policy.CIRequirement{{Provider: "github-actions", Schema: "github-actions/v1", Dataset: "event"}}
	require.ErrorIs(t, h.RunAll(BeforeTerraformInit, config, info, nil, nil), errUtils.ErrPolicyEvaluation)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	require.ErrorIs(t, h.RunAll(BeforeTerraformInit, config, info, cmd, nil), errUtils.ErrPolicyEvaluation)
}

func TestPolicyHookSkipControls(t *testing.T) {
	for _, value := range []any{
		[]any{map[string]any{"when": false}},
		map[string]any{"nested": map[string]any{"on_failure": "ignore"}},
	} {
		require.True(t, hasPolicySkipControl(value))
	}
	require.False(t, hasPolicySkipControl([]any{map[string]any{"url": "https://example.invalid"}}))
	h := policyTestHooks(AfterTerraformPlan, json.RawMessage(`{"kind":"command","command":"true"}`))
	h.policyBundle.Rules[0].Effects.Hooks[0].Event = string(AfterTerraformApply)
	require.ErrorIs(t, h.RunAll(AfterTerraformPlan, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil, nil), errUtils.ErrPolicyRequirements)
}

func TestPolicyPlanDetailedExitCode(t *testing.T) {
	run := &hookRunContext{event: AfterTerraformPlan, outcome: Outcome{Status: RunFailure, ExitCode: 2}}
	require.Equal(t, RunSuccess, policyOperationStatus(run))
	run.outcome.Err = errors.Join(errUtils.ErrPolicyRequirements, errUtils.ErrPolicyPlan)
	require.Equal(t, RunFailure, policyOperationStatus(run))
	run.outcome.Err = nil
	run.event = AfterTerraformApply
	require.Equal(t, RunFailure, policyOperationStatus(run))
}

func TestStarlarkPolicyHookExecution(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	definition, err := json.Marshal(map[string]any{"kind": "step", "type": "http", "with": map[string]any{"url": server.URL}})
	require.NoError(t, err)
	h := policyTestHooks(AfterHelmApply, definition)
	h.policyBundle.Rules[0].Engine = policy.StarlarkEngine
	h.policyBundle.Rules[0].Expression = `return ctx.input["event"] == "after.helm.apply"`
	h.policyBundle.Rules[0].Match = &policy.Selector{Labels: map[string]string{"stage": "prod"}}
	h.sections = map[string]any{"metadata": map[string]any{"labels": map[string]any{"stage": "prod"}}}
	config := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{Stack: "prod", ComponentFromArg: "app"}
	require.NoError(t, h.RunAll(AfterHelmApply, config, info, nil, nil))
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, h.RunAll(BeforeHelmApply, config, info, nil, nil))
	require.EqualValues(t, 1, calls.Load(), "a different event does not replay the hook")
	h.policyBundle.Rules = append(h.policyBundle.Rules, policy.Rule{
		ID: "deny", On: policy.Operation, Engine: policy.CELEngine, Expression: "true", Effects: policy.Effects{Deny: true},
	})
	require.ErrorIs(t, h.RunAll(AfterHelmApply, config, info, nil, nil), errUtils.ErrPolicyDenied)
	require.EqualValues(t, 1, calls.Load(), "a CEL denial blocks Starlark-selected hooks")
}

func TestValidatePolicyBundleOffline(t *testing.T) {
	for _, tc := range []struct {
		name, definition string
		invalid          bool
	}{
		{"missing executable is still valid offline", `{"kind":"command","command":"not-installed-policy-test-binary"}`, false},
		{"missing command", `{"kind":"command"}`, true},
		{"unknown kind", `{"kind":"unknown"}`, true},
		{"HTTP without URL", `{"kind":"step","type":"http","with":{}}`, true},
		{"steps without URL", `{"kind":"steps","with":[{"type":"http"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := &policy.Bundle{Hooks: map[string]json.RawMessage{"test/hook": json.RawMessage(tc.definition)}}
			err := ValidatePolicyBundle(bundle)
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
