package ci_test

import (
	"context"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/stretchr/testify/require"
)

func TestPolicyApprovalsRequireProvider(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	decision, err := ci.ResolvePolicyApprovals(context.Background(), nil, policy.Decision{Status: policy.Deny})
	require.NoError(t, err)
	require.Equal(t, policy.Deny, decision.Status)
	_, err = ci.ResolvePolicyApprovals(context.Background(), nil, policy.Decision{Status: policy.AwaitingApproval})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}
