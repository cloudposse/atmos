package githubactions

import (
	"strings"

	"github.com/rhysd/actionlint"

	"github.com/cloudposse/atmos/pkg/perf"
)

// selfReferenceRule adapts same-repository action references for the bundled
// actionlint version. Rewrite the parsed AST, not source text: shell scripts,
// comments and diagnostic positions stay intact.
// Both prefixes have the same width. Keep all normal action/input checks enabled.
type selfReferenceRule struct {
	actionlint.RuleBase
}

func withSelfReferences(rules []actionlint.Rule) []actionlint.Rule {
	rule := &selfReferenceRule{RuleBase: actionlint.NewRuleBase("action", "Support same-repository action references")}
	return append([]actionlint.Rule{rule}, rules...)
}

func (r *selfReferenceRule) VisitWorkflowPre(workflow *actionlint.Workflow) error {
	defer perf.Track(nil, "githubactions.selfReferenceRule.VisitWorkflowPre")()

	for _, job := range workflow.Jobs {
		if job.WorkflowCall != nil {
			r.normalize(job.WorkflowCall.Uses)
		}
		for _, step := range job.Steps {
			action, ok := step.Exec.(*actionlint.ExecAction)
			if !ok || action.Uses == nil || !strings.HasPrefix(action.Uses.Value, "$/") {
				continue
			}
			r.normalize(action.Uses)
		}
	}
	return nil
}

func (r *selfReferenceRule) normalize(uses *actionlint.String) {
	if uses == nil || !strings.HasPrefix(uses.Value, "$/") {
		return
	}
	path := strings.TrimPrefix(uses.Value, "$/")
	if strings.ContainsAny(path, "@\\\r\n") || strings.Contains(path, "${{") {
		r.Error(uses.Pos, "same-repository reference requires a literal repository-relative path without a ref")
		return
	}
	if strings.Contains("/"+path+"/", "/../") || strings.HasPrefix(path, "/") {
		r.Error(uses.Pos, "same-repository path must stay inside the repository")
		return
	}
	uses.Value = "./" + path
}
