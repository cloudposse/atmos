//go:build mage

package qualitygate

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

var errRegistryTest = errors.New("evidence unavailable")

func TestGateRegistryRejectsInvalidDefinitions(t *testing.T) {
	_, err := newGateRegistry()
	require.ErrorIs(t, err, errGateRegistration)
	valid := gateDefinition{ID: "example", Name: "Example", Remediation: "Fix the example.", Check: func(*qualityClient, *githubapi.PR) (qualityGate, error) { return qualityGate{Passed: true}, nil }}
	for _, scenario := range []string{"id", "name", "check", "remediation", "link", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			definition := valid
			if scenario != "duplicate" {
				definition.ID = "new"
			}
			switch scenario {
			case "id":
				definition.ID = " "
			case "name":
				definition.Name = ""
			case "check":
				definition.Check = nil
			case "remediation":
				definition.Remediation = ""
			case "link":
				definition.EvidencePath = "https://untrusted.example"
			}
			registry, err := newGateRegistry(valid)
			require.NoError(t, err)
			require.ErrorIs(t, registry.register(definition), errGateRegistration)
			require.Len(t, registry.definitions, 1)
		})
	}
}

func TestRegisteredGatesDriveReportAndGuidance(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	var calls []string
	makeCheck := func(id string, passed bool, err error) func(*qualityClient, *githubapi.PR) (qualityGate, error) {
		return func(_ *qualityClient, got *githubapi.PR) (qualityGate, error) {
			require.Same(t, &pr, got)
			calls = append(calls, id)
			return qualityGate{Name: "ignored metadata", Passed: passed, Detail: "measured evidence"}, err
		}
	}
	registry, err := newGateRegistry(
		gateDefinition{ID: "required", Name: "Required example", Check: makeCheck("required", true, nil), EvidencePath: "/checks", Remediation: "Fix required behavior."},
		gateDefinition{ID: "advisory", Name: "Advisory example", Check: makeCheck("advisory", false, nil), Advisory: true, EvidencePath: "/files", Remediation: "Investigate advisory behavior."},
	)
	require.NoError(t, err)
	gates := registry.evaluate(nil, &pr)
	require.Equal(t, []string{"required", "advisory"}, calls)
	body, ready := qualityMarkdown(&pr, gates, "")
	require.True(t, ready)
	require.Contains(t, body, "[Advisory example](https://github.com/cloudposse/atmos/pull/42/files)")
	require.Contains(t, body, "Investigate advisory behavior.")
	require.NotContains(t, body, "Fix required behavior.")
	require.Less(t, strings.Index(body, "Required example"), strings.Index(body, "Advisory example"))
	require.NoError(t, registry.register(gateDefinition{ID: "error", Name: "Unavailable example", Check: makeCheck("error", true, errRegistryTest), Remediation: "Restore evidence."}))
	gates = registry.evaluate(nil, &pr)
	require.False(t, gates[2].Passed, "a check error must override a passing verdict")
	body, ready = qualityMarkdown(&pr, gates, "")
	require.False(t, ready)
	require.Contains(t, body, "Unavailable: evidence unavailable")
	require.Contains(t, body, "GitHub evidence was unavailable")
	require.Contains(t, body, "Retry the reporter")
}

func TestDefaultRegistryContainsEveryRequiredGate(t *testing.T) {
	registry, err := defaultQualityRegistry()
	require.NoError(t, err)
	require.Len(t, registry.definitions, 10)
	require.Equal(t, "ready-for-review", registry.definitions[0].ID, "draft state is the first gate")
	require.Equal(t, "merge-conflicts", registry.definitions[1].ID)
	for _, id := range []string{"ready-for-review", "merge-conflicts", "release-policy", "ci", "coverage", "review", "signatures", "threads", "files", "benchmark-report"} {
		require.True(t, registry.ids[id], id)
	}
	for _, definition := range registry.definitions {
		require.False(t, definition.Advisory)
	}
}

// TestCheckMergeConflicts distinguishes conflicts, unknown mergeability, and unrelated branch protection states.
func TestCheckMergeConflicts(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name       string
		mergeable  *bool
		state      string
		wantPassed bool
		wantErr    bool
	}{
		{name: "mergeable", mergeable: &yes, state: "clean", wantPassed: true},
		{name: "blocked is not a conflict", mergeable: &yes, state: "blocked", wantPassed: true},
		{name: "behind is not a conflict", mergeable: &yes, state: "behind", wantPassed: true},
		{name: "unstable is not a conflict", mergeable: &yes, state: "unstable", wantPassed: true},
		{name: "not mergeable", mergeable: &no, state: "dirty"},
		{name: "not mergeable without state", mergeable: &no},
		{name: "dirty state", mergeable: &yes, state: "dirty"},
		{name: "dirty state without mergeable", state: "dirty"},
		{name: "not yet computed", state: "unknown", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := githubtest.PR(githubtest.Run())
			pr.Mergeable, pr.MergeableState = tc.mergeable, tc.state
			gate, err := checkMergeConflicts(nil, &pr)
			if tc.wantErr {
				require.ErrorContains(t, err, "mergeability")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantPassed, gate.Passed)
			if tc.wantPassed {
				require.Equal(t, "No merge conflicts with the target branch", gate.Detail)
				return
			}
			require.Equal(t, "This PR has merge conflicts with its target branch.", gate.Problem)
			require.Contains(t, gate.NextAction, "Rebase")
		})
	}
}

// TestMergeConflictsGateThroughRegistry verifies mergeability affects the report verdict and target-branch guidance.
func TestMergeConflictsGateThroughRegistry(t *testing.T) {
	allPass := func(*qualityClient, *githubapi.PR) (qualityGate, error) { return qualityGate{Passed: true}, nil }
	registry, err := defaultQualityRegistry()
	require.NoError(t, err)
	for i := range registry.definitions {
		if id := registry.definitions[i].ID; id != "ready-for-review" && id != "merge-conflicts" {
			registry.definitions[i].Check = allPass
		}
	}
	yes, no := true, false
	cases := []struct {
		name      string
		mergeable *bool
		wantReady bool
		wantUnav  bool
	}{
		{name: "clean", mergeable: &yes, wantReady: true},
		{name: "conflicting", mergeable: &no},
		{name: "unknown is unavailable, not a verdict", wantUnav: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := githubtest.PR(githubtest.Run())
			pr.Mergeable = tc.mergeable
			gates := registry.evaluate(nil, &pr)
			require.Equal(t, "merge-conflicts", gates[1].ID)
			require.Equal(t, tc.wantUnav, gates[1].Unavailable)
			body, ready := qualityMarkdown(&pr, gates, "")
			require.Equal(t, tc.wantReady, ready)
			switch {
			case tc.wantUnav:
				require.Contains(t, body, "GitHub evidence was unavailable")
			case !tc.wantReady:
				require.Contains(t, body, "This PR has merge conflicts with its target branch.")
				require.Contains(t, body, "https://github.com/cloudposse/atmos/pull/42/conflicts")
			}
		})
	}
}
