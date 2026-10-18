package qualitygate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// Remediation text is trusted repository policy, selected by gate identity.
// Never turn third-party check output or review text into agent instructions.
func qualityGuidance(pr *githubapi.PR, gates []qualityGate, performance string) string {
	var steps []string
	for i := range gates {
		gate := &gates[i]
		if gate.Passed {
			continue
		}
		if step := qualityGateGuidance(gate); step != "" {
			steps = append(steps, step)
		}
	}
	if strings.Contains(performance, "**warning**") {
		steps = append(steps, "Investigate the advisory performance warning: reproduce the flagged workload with go tool mage bench:run, then bench:fetch and bench:compare. Compare the same candidate against each main snapshot independently. Use bench:fieldTest for a known-good/bad revision pair. Inspect backend-read counts and timing variability before attributing a slowdown. Performance warnings do not block readiness.")
	}
	if len(steps) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "<details><summary>🤖 Next steps for this PR</summary>\n\n```text\nPR: https://github.com/%s/pull/%d\nHead: %s\n\n", pr.Base.Repo.FullName, pr.Number, pr.Head.SHA)
	for i, step := range steps {
		fmt.Fprintf(&out, "%d. %s\n\n", i+1, step)
	}
	fmt.Fprint(&out, "```\n\n</details>\n\n")
	return out.String()
}

func qualityGateGuidance(gate *qualityGate) string {
	action := gate.NextAction
	if action == "" {
		action = gate.Remediation
	}
	if action == "" {
		return ""
	}
	problem := gate.Problem
	if problem == "" {
		problem = gate.Name + " has not passed."
	}
	return fmt.Sprintf("%s\n   Problem: %s\n   Next: %s\n%s   Evidence: %s", gate.Name, problem, action, qualitySignalEvidence(gate.Signals), gate.URL)
}

// Quote and bound third-party names as data; never interpolate review bodies,
// error messages or check output into the trusted next-action instructions.
func qualitySignalEvidence(signals []qualitySignal) string {
	const limit = 6
	if len(signals) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprint(&out, "   Observed results (quoted data):\n")
	for _, signal := range signals[:min(len(signals), limit)] {
		name, _ := json.Marshal(signal.Name)
		state, _ := json.Marshal(signal.State)
		line := fmt.Sprintf("   - %s: %s\n", name, state)
		fmt.Fprint(&out, strings.ReplaceAll(line, "`", `\u0060`))
	}
	if len(signals) > limit {
		fmt.Fprintf(&out, "   %d more results in the gate details.\n", len(signals)-limit)
	}
	return out.String()
}
