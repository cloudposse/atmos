package ci

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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

func TestCollectApproverFacts(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new facts", true: "preserve other facts"}[existing], func(t *testing.T) {
			restore := SwapRegistryForTest()
			defer restore()
			controller := gomock.NewController(t)
			provider := NewMockProvider(controller)
			facts := NewMockPolicyFactProvider(controller)
			provider.EXPECT().Name().Return("test-ci").AnyTimes()
			provider.EXPECT().Detect().Return(true).AnyTimes()
			Register(struct {
				Provider
				PolicyFactProvider
			}{provider, facts})
			request := &policy.Request{Kind: policy.Command, CI: &policy.CISnapshot{}}
			original := policy.ApproverDataset([]string{"keep"}, policy.Provenance{})
			if existing {
				request.CI.Facts = map[string]policy.Dataset{"other": original}
			}
			rule := policy.Rule{On: policy.Command, RequiredCI: []policy.CIRequirement{{Dataset: "facts", Fact: "approvers"}}}
			collected := policy.ApproverDataset([]string{"alice"}, policy.Provenance{Revision: "head"})
			facts.EXPECT().CollectPolicyApprovers(gomock.Any(), request).Return(collected, nil)
			require.NoError(t, (PolicyFacts{}).CollectFacts(t.Context(), request, []policy.Rule{rule}))
			require.Equal(t, collected, request.CI.Facts["approvers"])
			if existing {
				require.Equal(t, original, request.CI.Facts["other"])
			}
			facts.EXPECT().CollectPolicyApprovers(gomock.Any(), request).Return(policy.Dataset{}, errUtils.ErrPolicyInputUnavailable)
			require.ErrorIs(t, (PolicyFacts{}).CollectFacts(t.Context(), request, []policy.Rule{rule}), errUtils.ErrPolicyInputUnavailable)
			require.Equal(t, collected, request.CI.Facts["approvers"])
			request.CI = nil
			require.ErrorIs(t, (PolicyFacts{}).CollectFacts(t.Context(), request, []policy.Rule{rule}), errUtils.ErrPolicyInputUnavailable)
		})
	}
}

func TestApproverFactsRespectSelectors(t *testing.T) {
	request := &policy.Request{Kind: policy.ComponentCommand, Subject: &policy.Subject{ObjectIdentity: policy.ObjectIdentity{Name: "app", Stack: "dev", ComponentType: "terraform"}}}
	rule := policy.Rule{On: policy.ComponentCommand, Match: &policy.Selector{Stacks: []string{"prod"}}, RequiredCI: []policy.CIRequirement{{Dataset: "facts", Fact: "approvers"}}}
	require.False(t, requiresApprovers(request, []policy.Rule{rule}))
	require.NoError(t, (PolicyFacts{}).CollectFacts(t.Context(), request, []policy.Rule{rule}))
	rule.Match = &policy.Selector{TagMode: "invalid"}
	require.False(t, requiresApprovers(request, []policy.Rule{rule}))
}
