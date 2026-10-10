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
	require.Contains(t, body, "Restore evidence.")
}

func TestDefaultRegistryContainsEveryRequiredGate(t *testing.T) {
	registry, err := defaultQualityRegistry()
	require.NoError(t, err)
	require.Len(t, registry.definitions, 6)
	for _, id := range []string{"ci", "coverage", "review", "signatures", "threads", "files"} {
		require.True(t, registry.ids[id], id)
	}
	for _, definition := range registry.definitions {
		require.False(t, definition.Advisory)
	}
}
