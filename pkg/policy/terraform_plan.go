package policy

import (
	"encoding/json"
	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateTerraformPlan validates the native show -json plan envelope without
// replacing it with a summary or discarding unknown/sensitive/forward-compatible fields.
// It does not attest that supplied JSON came from a saved binary; the collector does that.
func ValidateTerraformPlan(data json.RawMessage) error {
	defer perf.Track(nil, "policy.ValidateTerraformPlan")()

	object, err := decodeObject(data)
	if err != nil {
		return fmt.Errorf("%w: invalid plan JSON", errUtils.ErrPolicyInputInvalid)
	}
	version, ok := object["format_version"].(string)
	if !ok || !strings.HasPrefix(version, "1.") {
		return fmt.Errorf("%w: expected Terraform plan JSON format 1.x", errUtils.ErrPolicyInputInvalid)
	}
	var plan tfjson.Plan
	if err := json.Unmarshal(data, &plan); err != nil || plan.PlannedValues == nil {
		return fmt.Errorf("%w: expected a plan, not state or console output", errUtils.ErrPolicyInputInvalid)
	}
	if err := validatePlanFlags(object); err != nil {
		return err
	}
	return nil
}

func validatePlanFlags(object map[string]any) error {
	for key, blockingValue := range map[string]bool{"errored": true, "complete": false} {
		if raw, exists := object[key]; exists {
			value, ok := raw.(bool)
			if !ok || value == blockingValue {
				return fmt.Errorf("%w: plan is errored or incomplete", errUtils.ErrPolicyInputUnavailable)
			}
		}
	}
	if raw, exists := object["deferred_changes"]; exists {
		changes, ok := raw.([]any)
		if !ok || len(changes) != 0 {
			return fmt.Errorf("%w: plan has deferred changes", errUtils.ErrPolicyInputUnavailable)
		}
	}
	return nil
}

// EnforceDecision converts a pure decision into a local gate. This executor has no
// approval verifier or hook executor: outstanding obligations must not become success.
// Denial explanations include only static rule identifiers and author-supplied
// reasons, never evaluated values.
func EnforceDecision(decision Decision) error {
	defer perf.Track(nil, "policy.EnforceDecision")()

	if decision.Status == EvaluationError {
		return errUtils.ErrPolicyEvaluation
	}
	if decision.Status == Deny {
		return denialError(decision.Findings)
	}
	if decision.Status != Allow && decision.Status != NotApplicable {
		return errUtils.ErrPolicyRequirements
	}
	for _, finding := range decision.Findings {
		if finding.Status != Allow && finding.Status != NotApplicable {
			return errUtils.ErrPolicyRequirements
		}
		if len(finding.Effects.Approvals) != 0 || len(finding.Effects.Hooks) != 0 {
			return errUtils.ErrPolicyRequirements
		}
	}
	return nil
}

// denialError retains optional author guidance from every blocking rule.
// Reasons are literal text: they never evaluate templates or interpolate inputs.
func denialError(findings []Finding) error {
	var reasons []string
	for _, finding := range findings {
		if finding.Status == Deny {
			if reason := strings.TrimSpace(finding.Effects.Reason); reason != "" {
				reasons = append(reasons, finding.RuleID+":\n"+reason)
			}
		}
	}
	if len(reasons) == 0 {
		return errUtils.ErrPolicyDenied
	}
	return errUtils.Build(errUtils.ErrPolicyDenied).
		WithExplanation(strings.Join(reasons, "\n\n")).
		Err()
}
