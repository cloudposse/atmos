package ci

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
)

func TestRequiredApproverFacts(t *testing.T) {
	restore := SwapRegistryForTest()
	defer restore()
	request := &policy.Request{Kind: policy.Command}
	rule := policy.Rule{On: policy.Command, RequiredCI: []policy.CIRequirement{{Provider: "github-actions", Schema: "github-actions/v1", Dataset: "facts", Fact: "approvers"}}}
	require.True(t, requiresApprovers(request, []policy.Rule{rule}))
	err := (PolicyFacts{}).CollectFacts(t.Context(), request, []policy.Rule{rule})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	rule.On = policy.TerraformPlan
	require.False(t, requiresApprovers(request, []policy.Rule{rule}))
	require.NoError(t, (PolicyFacts{}).CollectFacts(t.Context(), request, []policy.Rule{rule}))
	rule.On = policy.Command
	rule.RequiredCI[0].Fact = "other"
	require.False(t, requiresApprovers(request, []policy.Rule{rule}))
}
