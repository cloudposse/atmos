package qualitygate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// gateDefinition is the complete contract for a trusted, compiled-in gate.
// Check returns evidence and a verdict; the registry supplies policy metadata.
// Advisory checks are displayed but cannot block the readiness label.
type gateDefinition struct {
	ID, Name, EvidencePath, Remediation string
	Advisory                            bool
	Check                               func(*qualityClient, *githubapi.PR) (qualityGate, error)
}

type gateRegistry struct {
	definitions []gateDefinition
	ids         map[string]bool
}

var errGateRegistration = errors.New("invalid quality gate registration")

func newGateRegistry(definitions ...gateDefinition) (*gateRegistry, error) {
	if len(definitions) == 0 {
		return nil, fmt.Errorf("%w: at least one gate is required", errGateRegistration)
	}
	registry := &gateRegistry{ids: make(map[string]bool)}
	for _, definition := range definitions {
		if err := registry.register(definition); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// register preserves registration order and rejects incomplete or duplicate policy.
//
//nolint:gocritic // Copy the policy so callers cannot mutate a registered definition.
func (r *gateRegistry) register(definition gateDefinition) error {
	if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Remediation) == "" || definition.Check == nil {
		return fmt.Errorf("%w: ID, name, check and remediation are required", errGateRegistration)
	}
	if r.ids[definition.ID] {
		return fmt.Errorf("%w: duplicate ID %q", errGateRegistration, definition.ID)
	}
	if definition.EvidencePath != "" && !strings.HasPrefix(definition.EvidencePath, "/") {
		return fmt.Errorf("%w: evidence path must be relative to the PR", errGateRegistration)
	}
	r.ids[definition.ID] = true
	r.definitions = append(r.definitions, definition)
	return nil
}

func (r *gateRegistry) evaluate(q *qualityClient, pr *githubapi.PR) []qualityGate {
	gates := make([]qualityGate, 0, len(r.definitions))
	for _, definition := range r.definitions {
		gate, err := definition.Check(q, pr)
		if err != nil {
			gate.Passed = false
			gate.Detail = "Unavailable: " + err.Error()
		}
		definition.decorate(pr, &gate)
		gates = append(gates, gate)
	}
	return gates
}

func (d *gateDefinition) decorate(pr *githubapi.PR, gate *qualityGate) {
	gate.ID, gate.Name, gate.Remediation, gate.Advisory = d.ID, d.Name, d.Remediation, d.Advisory
	gate.URL = fmt.Sprintf("https://github.com/%s/pull/%d%s", pr.Base.Repo.FullName, pr.Number, d.EvidencePath)
}

// Add a gate here: its check, report link and agent guidance travel together.
func defaultQualityRegistry() (*gateRegistry, error) {
	return newGateRegistry(
		gateDefinition{
			ID:           "ci",
			Name:         "All CI checks and workflows",
			Check:        (*qualityClient).tests,
			EvidencePath: "/checks",
			Remediation:  "Inspect all check runs, workflow jobs, and external commit statuses for the current PR, including merge-commit checks. Fix failing tests, lint, builds, and other checks. Wait for queued/running jobs. Retry an infrastructure failure after checking its logs; a passing Tests workflow alone is insufficient.",
		},
		gateDefinition{
			ID:           "coverage",
			Name:         "Codecov (current head)",
			Check:        (*qualityClient).coverage,
			EvidencePath: "/checks",
			Remediation:  "Open Codecov's current-head result and identify uncovered behavior or a failed upload. Add meaningful tests for uncovered changes, run them, and ensure coverage is uploaded for the latest commit. Wait for Codecov's reported success using the existing repository thresholds; older-commit results do not qualify.",
		},
		gateDefinition{
			ID:           "review",
			Name:         "CodeRabbit approval (current head)",
			Check:        (*qualityClient).review,
			EvidencePath: "",
			Remediation:  "Read CodeRabbit's outstanding feedback, address each finding or explain a justified disagreement, and obtain a fresh CodeRabbit approval on the latest commit. A comment or approval of an earlier commit is insufficient.",
		},
		gateDefinition{
			ID:           "signatures",
			Name:         "Verified commit signatures",
			Check:        (*qualityClient).signatures,
			EvidencePath: "/commits",
			Remediation:  "Find the PR commits whose signatures GitHub has not verified. Use the repository's signing workflow for your commits and coordinate any history rewrite with the author, then verify GitHub recognizes every signature.",
		},
		gateDefinition{
			ID:           "threads",
			Name:         "Review threads resolved or dismissed",
			Check:        (*qualityClient).threads,
			EvidencePath: "",
			Remediation:  "Inspect unresolved review threads, including outdated ones. Address each finding and record the resolution; resolve the thread after the fix is verified, or have an authorized reviewer dismiss the obsolete review with a reason. Do not blindly resolve threads to make the gate pass.",
		},
		gateDefinition{
			ID:           "files",
			Name:         "Fewer than 150 changed files",
			Check:        checkFileCount,
			EvidencePath: "/files",
			Remediation:  "Split the change into focused PRs so this PR changes at most 149 files and CodeRabbit can review it. Preserve dependencies between the resulting PRs, then request review of the updated head.",
		},
	)
}

func checkFileCount(_ *qualityClient, pr *githubapi.PR) (qualityGate, error) {
	return qualityFileCount(pr)
}
