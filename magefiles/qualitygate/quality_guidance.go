package qualitygate

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// Remediation text is trusted repository policy, selected by gate identity.
// Never turn third-party check output or review text into agent instructions.
func qualityGuidance(pr *githubapi.PR, gates []qualityGate, performance string) string {
	var steps []string
	for _, gate := range gates {
		if gate.Passed {
			continue
		}
		if instruction := gate.Remediation; instruction != "" {
			steps = append(steps, instruction+" Evidence: "+gate.URL)
		}
	}
	if pr.Draft {
		steps = append(steps, "When the changes and review are complete, mark the PR ready for review.")
	}
	if strings.Contains(performance, "**warning**") {
		steps = append(steps, "Investigate the advisory performance warning: reproduce the flagged workload with go tool mage bench:run, then bench:fetch and bench:compare. Compare the same candidate against each main snapshot independently. Use bench:fieldTest for a known-good/bad revision pair. Inspect backend-read counts and timing variability before attributing a slowdown. Performance warnings do not block readiness.")
	}
	if len(steps) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "<details><summary>🤖 Prompt to fix Quality Gate failures</summary>\n\n```text\nBring https://github.com/%s/pull/%d through the Quality Gate.\nEvaluated head: %s\nRecheck the current head and gate evidence before editing; these results may have changed.\n\n", pr.Base.Repo.FullName, pr.Number, pr.Head.SHA)
	for i, step := range steps {
		fmt.Fprintf(&out, "%d. %s\n\n", i+1, step)
	}
	fmt.Fprint(&out, "Run the relevant repository checks after changes. Let the reporter refresh from GitHub evidence; do not manually mark a gate passed or change thresholds to hide a failure.\n```\n\n</details>\n\n")
	return out.String()
}
