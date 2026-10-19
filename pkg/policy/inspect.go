package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TestCase is the offline fixture format shared with the distributed policy catalog.
type TestCase struct {
	Name      string                `json:"name"`
	Request   Request               `json:"request"`
	Status    DecisionStatus        `json:"status"`
	Error     string                `json:"error,omitempty"`
	Hooks     []HookRequirement     `json:"hooks,omitempty"`
	Approvals []ApprovalRequirement `json:"approvals,omitempty"`
}

// TestResult reports observable fixture outcomes without retaining sensitive inputs.
type TestResult struct {
	Name     string         `json:"name"`
	Passed   bool           `json:"passed"`
	Expected DecisionStatus `json:"expected"`
	Actual   DecisionStatus `json:"actual"`
}

// RunTests evaluates fixtures without collecting CI facts or executing obligations.
func RunTests(ctx context.Context, bundle *Bundle, path string) ([]TestResult, error) {
	defer perf.Track(nil, "policy.RunTests")()
	cases, err := readTestCases(path)
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		return nil, fmt.Errorf("%w: missing policy bundle", errUtils.ErrPolicyInvalid)
	}
	results := make([]TestResult, 0, len(cases))
	engine := NewEngine()
	failed := false
	for i := range cases {
		tc := &cases[i]
		decision, evalErr := engine.Evaluate(ctx, &tc.Request, bundle.Rules)
		passed := decision.Status == tc.Status && testErrorMatches(tc.Error, evalErr)
		passed = passed && testEffectsMatch(decision, tc)
		results = append(results, TestResult{tc.Name, passed, tc.Status, decision.Status})
		failed = failed || !passed
	}
	if failed {
		return results, fmt.Errorf("%w: policy test expectations failed", errUtils.ErrPolicyEvaluation)
	}
	return results, nil
}

func validTestStatus(status DecisionStatus) bool {
	switch status {
	case Allow, Deny, NotApplicable, AwaitingApproval, EvaluationError:
		return true
	}
	return false
}

// sameJSON compares two values by content. Object key order never affects equality: an approval spec
// built from a map sorts its keys, while a fixture keeps the order its author wrote.
func sameJSON(a, b any) bool {
	x, errX := canonicalJSON(a)
	y, errY := canonicalJSON(b)
	return errX == nil && errY == nil && bytes.Equal(x, y)
}

// canonicalJSON re-encodes a value through any, which sorts object keys.
func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}

func testErrorMatches(expected string, err error) bool {
	switch expected {
	case "":
		return err == nil
	case "input_unavailable":
		return errors.Is(err, errUtils.ErrPolicyInputUnavailable)
	case "evaluation":
		return errors.Is(err, errUtils.ErrPolicyEvaluation)
	default:
		return false
	}
}

const maxTestCaseBytes = 16 * 1024 * 1024

func readTestCases(path string) ([]TestCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read test cases: %w", errUtils.ErrPolicyInvalid, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxTestCaseBytes+1))
	if err != nil || len(raw) > maxTestCaseBytes {
		return nil, fmt.Errorf("%w: test cases unreadable or too large", errUtils.ErrPolicyInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cases []TestCase
	if err := decoder.Decode(&cases); err != nil {
		return nil, fmt.Errorf("%w: test cases: %w", errUtils.ErrPolicyInvalid, err)
	}
	if len(cases) == 0 || decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("%w: expected one nonempty test-case array", errUtils.ErrPolicyInvalid)
	}
	if err := validateTestCases(cases); err != nil {
		return nil, err
	}

	return cases, nil
}

func testEffectsMatch(decision Decision, tc *TestCase) bool {
	var hooks []HookRequirement
	var approvals []ApprovalRequirement
	for _, finding := range decision.Findings {
		hooks = append(hooks, finding.Effects.Hooks...)
		approvals = append(approvals, finding.Effects.Approvals...)
	}
	return sameJSON(hooks, tc.Hooks) && (tc.Approvals == nil || sameJSON(approvals, tc.Approvals))
}

func validateTestCases(cases []TestCase) error {
	names := map[string]bool{}
	for i := range cases {
		tc := &cases[i]
		if tc.Name == "" || names[tc.Name] || !validTestStatus(tc.Status) || (tc.Error != "" && tc.Error != "input_unavailable" && tc.Error != "evaluation") {
			return fmt.Errorf("%w: invalid or duplicate test case", errUtils.ErrPolicyInvalid)
		}
		names[tc.Name] = true
	}

	return nil
}
