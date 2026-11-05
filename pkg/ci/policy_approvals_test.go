package ci_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/policy"
)

// approvalResolverProvider is an active CI provider with the optional approval capability.
type approvalResolverProvider struct {
	provider.Provider
	resolved policy.Decision
	err      error
	requests []*policy.Request
}

func (*approvalResolverProvider) Name() string { return "approval-resolver" }
func (*approvalResolverProvider) Detect() bool { return true }

func (p *approvalResolverProvider) ResolvePolicyApprovals(_ context.Context, request *policy.Request, _ policy.Decision) (policy.Decision, error) {
	p.requests = append(p.requests, request)
	return p.resolved, p.err
}

// plainProvider is active but cannot verify approvals.
type plainProvider struct{ provider.Provider }

func (*plainProvider) Name() string { return "plain" }
func (*plainProvider) Detect() bool { return true }

func TestPolicyApprovalsRequireProvider(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	decision, err := ci.ResolvePolicyApprovals(context.Background(), nil, policy.Decision{Status: policy.Deny})
	require.NoError(t, err)
	require.Equal(t, policy.Deny, decision.Status)
	_, err = ci.ResolvePolicyApprovals(context.Background(), nil, policy.Decision{Status: policy.AwaitingApproval})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestPolicyApprovalsDelegateToActiveProvider(t *testing.T) {
	request := &policy.Request{Kind: policy.Operation}
	awaiting := policy.Decision{Status: policy.AwaitingApproval}

	t.Run("resolved decision replaces the awaiting one", func(t *testing.T) {
		restore := ci.SwapRegistryForTest()
		defer restore()
		fake := &approvalResolverProvider{resolved: policy.Decision{Status: policy.Allow}}
		ci.Register(fake)
		var events []policy.Explanation
		cleanup := policy.BeginExplanation(func(e policy.Explanation) { events = append(events, e) }, nil)
		defer cleanup()

		decision, err := ci.ResolvePolicyApprovals(t.Context(), request, awaiting)
		require.NoError(t, err)
		require.Equal(t, policy.Allow, decision.Status)
		require.Equal(t, []*policy.Request{request}, fake.requests)
		require.Equal(t, []policy.Explanation{{Kind: policy.Operation, Status: "approval verification: allow"}}, events)
	})

	t.Run("provider failure is reported and returned", func(t *testing.T) {
		restore := ci.SwapRegistryForTest()
		defer restore()
		ci.Register(&approvalResolverProvider{resolved: awaiting, err: errUtils.ErrPolicyInputUnavailable})
		var events []policy.Explanation
		cleanup := policy.BeginExplanation(func(e policy.Explanation) { events = append(events, e) }, nil)
		defer cleanup()

		decision, err := ci.ResolvePolicyApprovals(t.Context(), request, awaiting)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Equal(t, policy.AwaitingApproval, decision.Status)
		require.Equal(t, []policy.Explanation{{Kind: policy.Operation, Status: "approval verification failed"}}, events)
	})

	t.Run("active provider without the capability fails closed", func(t *testing.T) {
		restore := ci.SwapRegistryForTest()
		defer restore()
		ci.Register(&plainProvider{})
		decision, err := ci.ResolvePolicyApprovals(t.Context(), request, awaiting)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Equal(t, policy.AwaitingApproval, decision.Status)
	})

	t.Run("only awaiting decisions reach the provider", func(t *testing.T) {
		restore := ci.SwapRegistryForTest()
		defer restore()
		fake := &approvalResolverProvider{resolved: policy.Decision{Status: policy.Allow}}
		ci.Register(fake)
		for _, status := range []policy.DecisionStatus{policy.Allow, policy.Deny, policy.NotApplicable, policy.EvaluationError} {
			decision, err := ci.ResolvePolicyApprovals(t.Context(), request, policy.Decision{Status: status})
			require.NoError(t, err)
			require.Equal(t, status, decision.Status)
		}
		require.Empty(t, fake.requests)
	})
}
