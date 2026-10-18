package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
)

func getGovernedHooks(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*Hooks, error) {
	// Resolve governing data before describing (potentially untrusted) component data.
	var bundle *policy.Bundle
	var snapshot *policy.CISnapshot
	directory, err := policy.DirectoryFromEnvironment()
	if err != nil {
		return nil, err
	}
	if directory != "" {
		bundle, err = policy.LoadDirectory(directory)
		if err != nil {
			return nil, err
		}
		snapshot, err = ci.CollectPolicySnapshot(context.Background())
		if err != nil {
			return nil, err
		}
	}
	h, err := getComponentHooks(config, info)
	if err != nil {
		return nil, err
	}
	h.policyBundle, h.policyCI = bundle, snapshot
	return h, nil
}

func (h *Hooks) runPolicyHooks(run *hookRunContext) error {
	if h.policyBundle == nil {
		return nil
	}
	ctx := context.Background()
	if run.cmd != nil && run.cmd.Context() != nil {
		ctx = run.cmd.Context()
	}
	request, err := operationPolicyRequest(run, h.policyCI)
	if err != nil {
		return err
	}
	if h.sections != nil {
		config := run.atmosConfig
		if config == nil {
			config = h.config
		}
		var delimiters []string
		if config != nil {
			delimiters = config.Templates.Settings.Delimiters
		}
		request.Subject, err = policy.NewSubject(policy.ObjectIdentity{
			Name: run.info.ComponentFromArg, Stack: run.info.Stack,
			ComponentType: strings.Split(string(run.event.Normalize()), ".")[1],
		}, h.sections["metadata"], delimiters...)
		if err != nil {
			return err
		}
	}
	engine := policy.NewEngine()
	decision, err := engine.Evaluate(ctx, request, h.policyBundle.Rules)
	if err != nil {
		return fmt.Errorf("%w: operation evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	if err := enforcePolicyAuthorization(decision); err != nil {
		return err
	}
	return h.dispatchPolicyHooks(ctx, decision, run)
}

func operationPolicyRequest(run *hookRunContext, snapshot *policy.CISnapshot) (*policy.Request, error) {
	if run.info == nil {
		return nil, fmt.Errorf("%w: missing lifecycle context", errUtils.ErrPolicyInputUnavailable)
	}
	parts := strings.Split(string(run.event.Normalize()), ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: unsupported lifecycle event", errUtils.ErrPolicyInputUnavailable)
	}
	input, err := json.Marshal(map[string]any{
		"event": string(run.event.Normalize()), "component_type": parts[1], "command": parts[2],
		"component": run.info.ComponentFromArg, "stack": run.info.Stack, "status": string(policyOperationStatus(run)),
		"exit_code": run.outcome.ExitCode, "dry_run": run.info.DryRun,
	})
	if err != nil {
		return nil, err
	}
	return &policy.Request{Kind: policy.Operation, Input: input, CI: snapshot}, nil
}

// Detailed-exitcode 2 is a successful Terraform plan with changes, even when the
// CLI preserves it as a nonzero exit. Never reinterpret a policy gate error.
func policyOperationStatus(run *hookRunContext) RunStatus {
	if run.event.Normalize() != AfterTerraformPlan || run.outcome.ExitCode != 2 {
		return run.outcome.Status
	}
	for _, sentinel := range []error{errUtils.ErrPolicyInvalid, errUtils.ErrPolicyPlan, errUtils.ErrPolicyDenied, errUtils.ErrPolicyEvaluation, errUtils.ErrPolicyRequirements} {
		if errors.Is(run.outcome.Err, sentinel) {
			return run.outcome.Status
		}
	}
	return RunSuccess
}

func enforcePolicyAuthorization(decision policy.Decision) error {
	// Authorize independently of hook fulfillment; hooks never override denial.
	gate := decision
	gate.Findings = append([]policy.Finding(nil), decision.Findings...)
	for i := range gate.Findings {
		gate.Findings[i].Effects.Hooks = nil
	}
	return policy.EnforceDecision(gate)
}

func (h *Hooks) dispatchPolicyHooks(ctx context.Context, decision policy.Decision, run *hookRunContext) error {
	var failures []error
	for _, finding := range decision.Findings {
		for _, requirement := range finding.Effects.Hooks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := h.executePolicyHook(requirement, run); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func (h *Hooks) executePolicyHook(requirement policy.HookRequirement, run *hookRunContext) error {
	if HookEvent(requirement.Event).Normalize() != run.event.Normalize() {
		return fmt.Errorf("%w: selected hook targets a different lifecycle event", errUtils.ErrPolicyRequirements)
	}
	hook, kind, err := h.resolvePolicyHook(requirement.Ref)
	if err != nil {
		return err
	}
	if run.info.DryRun {
		log.Info("Policy hook preview (not executed)", "hook", requirement.Ref, "event", run.event)
		return nil
	}
	if !requirement.Required && NewSkipPredicate(ResolveSkipHooks(run.cmd))(requirement.Ref) {
		return nil
	}
	// Pin from governing policy only; do not merge workspace dependency overrides.
	trusted := &Hooks{}
	if err := trusted.installDeps(run.atmosConfig, run.info, h.policyBundle.Tools); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPolicyRequirements, err)
	}
	if err := trusted.verifyHookBinary(requirement.Ref, hook); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPolicyRequirements, err)
	}
	if requirement.RequireSuccess {
		hook.OnFailure = OnFailureFail
	}
	if err := trusted.runResolvedHook(requirement.Ref, kind, hook, run); err != nil {
		return fmt.Errorf("%w: hook %s: %w", errUtils.ErrPolicyRequirements, requirement.Ref, err)
	}
	return nil
}

func (h *Hooks) resolvePolicyHook(ref string) (*Hook, *Kind, error) {
	data, exists := h.policyBundle.Hooks[ref]
	if !exists {
		return nil, nil, fmt.Errorf("%w: missing governing hook %s", errUtils.ErrPolicyRequirements, ref)
	}
	var hook Hook
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&hook); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid governing hook %s", errUtils.ErrPolicyRequirements, ref)
	}
	// The predicate owns selection; a secondary skip cannot fulfill an obligation.
	if len(hook.Events) != 0 || !hook.When.IsZero() || hasPolicySkipControl(hook.With) {
		return nil, nil, fmt.Errorf("%w: governing hooks cannot contain skip/background controls", errUtils.ErrPolicyRequirements)
	}
	kind, exists := GetKind(hook.Kind)
	if !exists || kind.Engine == nil || isDeprecatedCIKind(hook.Kind) {
		return nil, nil, fmt.Errorf("%w: unavailable hook kind %s", errUtils.ErrPolicyRequirements, hook.Kind)
	}
	return &hook, kind, nil
}

func hasPolicySkipControl(value any) bool {
	switch object := value.(type) {
	case map[string]any:
		for key, item := range object {
			if slices.Contains([]string{"when", "background", "continue_on_error", "on_failure"}, key) {
				return true
			}
			if hasPolicySkipControl(item) {
				return true
			}
		}
	case []any:
		for _, item := range object {
			if hasPolicySkipControl(item) {
				return true
			}
		}
	}
	return false
}
