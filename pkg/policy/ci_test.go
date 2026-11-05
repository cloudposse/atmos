package policy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func testSnapshot() *CISnapshot {
	dataset := Dataset{Status: Available, Complete: true, Data: json.RawMessage(`[]`)}
	return &CISnapshot{Provider: "github-actions", Schema: "github-actions/v1", Metadata: dataset, Event: dataset, Facts: map[string]Dataset{"reviews": dataset}}
}

func TestRequiredCI(t *testing.T) {
	base := CIRequirement{Provider: "github-actions", Schema: "github-actions/v1", Dataset: "facts", Fact: "reviews"}
	cases := []struct {
		name    string
		change  func(*CISnapshot, *CIRequirement)
		wantErr error
	}{
		{"complete empty reviews", func(_ *CISnapshot, _ *CIRequirement) {}, nil},
		{"event", func(_ *CISnapshot, r *CIRequirement) { r.Dataset, r.Fact = "event", "" }, nil},
		{"metadata", func(_ *CISnapshot, r *CIRequirement) { r.Dataset, r.Fact = "metadata", "" }, nil},
		{"provider mismatch", func(s *CISnapshot, _ *CIRequirement) { s.Provider = "gitlab" }, errUtils.ErrPolicyInputUnavailable},
		{"schema mismatch", func(s *CISnapshot, _ *CIRequirement) { s.Schema = "v2" }, errUtils.ErrPolicyInputUnavailable},
		{"missing fact", func(s *CISnapshot, _ *CIRequirement) { delete(s.Facts, "reviews") }, errUtils.ErrPolicyInputUnavailable},
		{"incomplete", func(s *CISnapshot, _ *CIRequirement) {
			s.Facts["reviews"] = Dataset{Status: Available, Data: json.RawMessage(`[]`)}
		}, errUtils.ErrPolicyInputUnavailable},
		{"unavailable", func(s *CISnapshot, _ *CIRequirement) {
			s.Facts["reviews"] = Dataset{Status: Unavailable, Complete: true, Data: json.RawMessage(`[]`)}
		}, errUtils.ErrPolicyInputUnavailable},
		{"null", func(s *CISnapshot, _ *CIRequirement) {
			s.Facts["reviews"] = Dataset{Status: Available, Complete: true, Data: json.RawMessage(" null ")}
		}, errUtils.ErrPolicyInputUnavailable},
		{"empty", func(s *CISnapshot, _ *CIRequirement) { s.Facts["reviews"] = Dataset{Status: Available, Complete: true} }, errUtils.ErrPolicyInputUnavailable},
		{"no provider", func(_ *CISnapshot, r *CIRequirement) { r.Provider = "" }, errUtils.ErrPolicyInvalid},
		{"unknown dataset", func(_ *CISnapshot, r *CIRequirement) { r.Dataset, r.Fact = "unknown", "" }, errUtils.ErrPolicyInvalid},
		{"missing fact name", func(_ *CISnapshot, r *CIRequirement) { r.Fact = "" }, errUtils.ErrPolicyInvalid},
		{"unexpected fact name", func(_ *CISnapshot, r *CIRequirement) { r.Dataset = "event" }, errUtils.ErrPolicyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, requirement := testSnapshot(), base
			tc.change(snapshot, &requirement)
			rule := testRule("review", "false", Effects{})
			rule.RequiredCI = []CIRequirement{requirement}
			request := testRequest()
			request.CI = snapshot
			decision, err := testEngine(t).Evaluate(t.Context(), request, []Rule{rule})
			if tc.wantErr == nil {
				require.NoError(t, err)
				assert.Equal(t, NotApplicable, decision.Status)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Equal(t, EvaluationError, decision.Status)
			}
		})
	}
	require.ErrorIs(t, requireCI(nil, base), errUtils.ErrPolicyInputUnavailable)
}

func TestProviderNativeInputs(t *testing.T) {
	// Synthetic adapter snapshots, not claims of implemented GitLab/Azure collectors.
	cases := []struct {
		provider   string
		metadata   string
		event      Dataset
		expression string
	}{
		{
			"github-actions", `{}`,
			Dataset{Status: Available, Complete: true, Data: json.RawMessage(`{"action":"submitted","unknown_native_field":{"id":9007199254740993}}`)},
			"ci.event.data.action == 'submitted' && ci.event.data.unknown_native_field.id == 9007199254740993 && ci.facts.reviews.data[0].state == 'DISMISSED'",
		},
		{
			"gitlab", `{"CI_PIPELINE_SOURCE":"merge_request_event"}`,
			Dataset{Status: Unavailable, Reason: "environment only"},
			"ci.metadata.data.CI_PIPELINE_SOURCE == 'merge_request_event' && !has(ci.event.data)",
		},
		{
			"azure-devops", `{"Build.SourceBranch":"refs/heads/main"}`,
			Dataset{Status: Unavailable, Reason: "environment only"},
			"ci.metadata.data['Build.SourceBranch'] == 'refs/heads/main' && !has(ci.event.data)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			// Early function checks need no stack or resolved component configuration.
			request := &Request{Kind: Function, Input: json.RawMessage(`{}`), CI: testSnapshot()}
			request.CI.Provider, request.CI.Schema = tc.provider, tc.provider+"/v1"
			request.CI.Metadata.Data = json.RawMessage(tc.metadata)
			request.CI.Event = tc.event
			request.CI.Facts["reviews"] = Dataset{Status: Available, Complete: true, Data: json.RawMessage(`[{"state":"DISMISSED"}]`)}
			rule := testRule("native", "ci.provider == params.provider && ("+tc.expression+")", Effects{Deny: true})
			rule.On = Function
			parameters, err := json.Marshal(map[string]string{"provider": tc.provider})
			require.NoError(t, err)
			rule.Parameters = parameters
			decision, err := testEngine(t).Evaluate(t.Context(), request, []Rule{rule})
			require.NoError(t, err)
			assert.Equal(t, Deny, decision.Status)
		})
	}
}

func TestMalformedSnapshot(t *testing.T) {
	request := testRequest()
	request.CI = testSnapshot()
	request.CI.Event.Data = json.RawMessage(`not json`)
	decision, err := testEngine(t).Evaluate(t.Context(), request, []Rule{testRule("guard", "true", Effects{})})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	assert.Equal(t, EvaluationError, decision.Status)
}
