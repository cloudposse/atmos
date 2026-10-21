package ci

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
)

// PolicyFactProvider materializes explicitly requested provider facts before evaluation.
//
//go:generate go run go.uber.org/mock/mockgen -source=policy_facts.go -destination=mock_policy_facts_test.go -package=ci
type PolicyFactProvider interface {
	CollectPolicyApprovers(context.Context, *policy.Request) (policy.Dataset, error)
}

// PolicyFacts connects the shared engine to Native CI without evaluator I/O.
type PolicyFacts struct{}

// CollectFacts fetches reviews only for rules explicitly requiring approver evidence.
func (PolicyFacts) CollectFacts(ctx context.Context, request *policy.Request, rules []policy.Rule) error {
	defer perf.Track(nil, "ci.PolicyFacts.CollectFacts")()
	if !requiresApprovers(request, rules) {
		return nil
	}
	provider, ok := Detect().(PolicyFactProvider)
	if !ok || request.CI == nil {
		return fmt.Errorf("%w: CI provider cannot collect approvers", errUtils.ErrPolicyInputUnavailable)
	}
	dataset, err := provider.CollectPolicyApprovers(ctx, request)
	if err != nil {
		return err
	}
	if request.CI.Facts == nil {
		request.CI.Facts = map[string]policy.Dataset{}
	}
	request.CI.Facts["approvers"] = dataset
	return nil
}

func requiresApprovers(request *policy.Request, rules []policy.Rule) bool {
	for i := range rules {
		rule := &rules[i]
		if rule.On != request.Kind {
			continue
		}
		selected, err := rule.Match.Matches(request.Subject)
		if err != nil || !selected {
			continue
		}
		for _, requirement := range rule.RequiredCI {
			if requirement.Dataset == "facts" && requirement.Fact == "approvers" {
				return true
			}
		}
	}
	return false
}
