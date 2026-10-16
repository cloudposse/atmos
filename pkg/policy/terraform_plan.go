package policy

import (
	"encoding/json"
	"errors"
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

// denialError names every blocking rule and retains optional author guidance.
// Rule identifiers are static, so they are safe to print even when a rule has no
// reason; reasons are static Markdown and never evaluate templates or interpolate
// inputs. Every denial carries ExitCodePolicyDenied so callers can tell a policy
// decision apart from usage and runtime failures.
func denialError(findings []Finding) error {
	var blocks []string
	for _, finding := range findings {
		if finding.Status != Deny {
			continue
		}
		if reason := strings.TrimSpace(finding.Effects.Reason); reason != "" {
			blocks = append(blocks, finding.RuleID+":\n\n"+reason)
			continue
		}
		blocks = append(blocks, "Denied by "+finding.RuleID+".")
	}
	builder := errUtils.Build(errUtils.ErrPolicyDenied).WithExitCode(errUtils.ExitCodePolicyDenied)
	if len(blocks) > 0 {
		builder = builder.WithExplanation(strings.Join(blocks, "\n\n"))
	}
	return builder.Err()
}

// IsAdmissionError reports whether err is a policy admission failure: a denial,
// an unmet requirement, or a policy that could not be loaded or evaluated.
// Admission fails closed, so callers must surface these errors unchanged rather
// than relabeling them as hook or execution failures.
func IsAdmissionError(err error) bool {
	defer perf.Track(nil, "policy.IsAdmissionError")()

	if err == nil {
		return false
	}
	for _, sentinel := range []error{
		errUtils.ErrPolicyDenied,
		errUtils.ErrPolicyRequirements,
		errUtils.ErrPolicyEvaluation,
		errUtils.ErrPolicyInputInvalid,
		errUtils.ErrPolicyInputUnavailable,
		errUtils.ErrPolicyInvalid,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}
