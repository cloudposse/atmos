package doctor

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/validation"
)

// excludes reports whether any --exclude pattern matches the project-relative path and counts the
// matches of every pattern, so a pattern that never matches can be reported after inspection.
func (p *project) excludes(o *Options, rel string) (bool, error) {
	if len(o.Exclude) == 0 {
		return false, nil
	}
	if len(p.excludeHits) != len(o.Exclude) {
		p.excludeHits = make([]int, len(o.Exclude))
	}
	excluded := false
	for i, pattern := range o.Exclude {
		kept, err := validation.ExcludePaths([]string{rel}, []string{pattern})
		if err != nil {
			return false, err
		}
		if len(kept) == 0 {
			p.excludeHits[i]++
			excluded = true
		}
	}
	return excluded, nil
}

// noteUnmatchedExcludes reports each --exclude pattern that matched no project input. Patterns are
// relative to the project root, which is not always the directory the user ran doctor from, so a
// silent miss would look like a working exclusion. The notice is appended directly, so --only never filters it.
func noteUnmatchedExcludes(p *project, o *Options, r *Report) {
	for i, pattern := range o.Exclude {
		if i < len(p.excludeHits) && p.excludeHits[i] > 0 {
			continue
		}
		r.Findings = append(r.Findings, Finding{
			RuleID:   ruleExclude,
			Category: categoryValidation,
			Severity: validation.SeverityNotice,
			Message:  fmt.Sprintf("--exclude %q matched no project inputs; patterns are relative to the project root %s", pattern, p.root),
			Status:   statusManual,
			// Without this the report would suggest reviewing a declaration, but no declaration is at fault.
			Remediation: "Write --exclude patterns relative to the project root, such as stacks/generated/**, and check their spelling",
		})
	}
}
