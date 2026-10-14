package ci

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
)

// PolicyApprovalProvider resolves provider-native review obligations using fresh facts.
// This optional capability does not change the common CI metadata vocabulary.
type PolicyApprovalProvider interface {
	ResolvePolicyApprovals(context.Context, *policy.Request, policy.Decision) (policy.Decision, error)
}

// ResolvePolicyApprovals never collects reviews for a denial, error, or unmatched rule.
func ResolvePolicyApprovals(ctx context.Context, request *policy.Request, decision policy.Decision) (policy.Decision, error) {
	defer perf.Track(nil, "ci.ResolvePolicyApprovals")()
	if decision.Status != policy.AwaitingApproval {
		return decision, nil
	}
	provider := Detect()
	resolver, ok := provider.(PolicyApprovalProvider)
	if !ok {
		return decision, fmt.Errorf("%w: active CI provider cannot verify approvals", errUtils.ErrPolicyInputUnavailable)
	}
	return resolver.ResolvePolicyApprovals(ctx, request, decision)
}
