package qualitygate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/magefiles/bench"
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

// Published in the report for a gate whose evidence could not be read. Scheduled
// recovery also matches it to keep such reports eligible until the evidence recovers.
const qualityUnavailableProblem = "could not be evaluated because GitHub evidence was unavailable."

func (r *gateRegistry) evaluate(q *qualityClient, pr *githubapi.PR) []qualityGate {
	gates := make([]qualityGate, 0, len(r.definitions))
	for _, definition := range r.definitions {
		gate, err := definition.Check(q, pr)
		if err != nil {
			gate.Passed = false
			gate.Unavailable = true
			gate.Detail = "Unavailable: " + err.Error()
			gate.Problem = definition.Name + " " + qualityUnavailableProblem
			gate.NextAction = "Retry the reporter and inspect its API error if retrieval still fails. This is missing evidence, not a confirmed failure in the PR."
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
			ID:           "ready-for-review",
			Name:         "Ready for review",
			Check:        checkReadyForReview,
			EvidencePath: "",
			Remediation:  "Mark the PR ready for review when the work is complete. A draft PR is never ready to merge.",
		},
		gateDefinition{
			ID:           "merge-conflicts",
			Name:         "No merge conflicts",
			Check:        checkMergeConflicts,
			EvidencePath: "/conflicts",
			Remediation:  "Rebase the PR branch onto main or merge main into it, resolve the conflicts, and push. GitHub recomputes mergeability after the push.",
		},
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
			Remediation:  "Request a review of the latest changes using the commands documented at https://docs.coderabbit.ai/guides/commands#manually-request-code-reviews.",
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
		gateDefinition{
			ID:           "benchmark-report",
			Name:         "Benchmark report integrity",
			Check:        checkBenchmarkReport,
			EvidencePath: "/checks",
			Remediation:  "Inspect the benchmark reporter job logs. Repair malformed or incomplete measurements, artifact provenance, or reporting failures and rerun the affected workflow. Reconciliation will retry the report. Missing historical baselines and measured slowdowns alone do not fail this gate.",
		},
	)
}

func checkReadyForReview(_ *qualityClient, pr *githubapi.PR) (qualityGate, error) {
	if pr.Draft {
		return qualityGate{Detail: "Draft", Problem: "This PR is a draft.", NextAction: "Mark the PR ready for review when the work is complete."}, nil
	}
	return qualityGate{Passed: true, Detail: "Marked ready for review"}, nil
}

// checkMergeConflicts treats only an explicit mergeable=false or a dirty state
// as a conflict. Other states such as blocked, behind, or unstable describe
// branch protection or CI and are covered by their own gates.
func checkMergeConflicts(_ *qualityClient, pr *githubapi.PR) (qualityGate, error) {
	if pr.Mergeable == nil && pr.MergeableState != "dirty" {
		// GitHub computes mergeability lazily after a push or base move. Report
		// missing evidence so the previous report is kept and the check retried.
		return qualityGate{}, bench.Error("GitHub has not computed mergeability yet")
	}
	if pr.MergeableState == "dirty" || (pr.Mergeable != nil && !*pr.Mergeable) {
		return qualityGate{Detail: "Merge conflicts with main", Problem: "This PR has merge conflicts with main.", NextAction: "Rebase the PR branch onto main or merge main into it, resolve the conflicts, and push."}, nil
	}
	return qualityGate{Passed: true, Detail: "No merge conflicts with main"}, nil
}

func checkBenchmarkReport(q *qualityClient, _ *githubapi.PR) (qualityGate, error) {
	if q.benchmarkReportFailed {
		return qualityGate{Detail: "Benchmark report processing failed; comparison unavailable", Problem: "The benchmark reporter could not validate or process this commit's measurements.", NextAction: "Inspect the benchmark reporter job's error, correct the rejected artifact or reporting failure, and rerun the affected workflow."}, nil
	}
	return qualityGate{Passed: true, Detail: "No benchmark report processing error recorded for this head"}, nil
}

func checkFileCount(_ *qualityClient, pr *githubapi.PR) (qualityGate, error) {
	return qualityFileCount(pr)
}
