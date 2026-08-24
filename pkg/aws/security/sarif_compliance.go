package security

import (
	"sort"
)

// buildComplianceSARIFLog converts a ComplianceReport into a SARIF log.
func buildComplianceSARIFLog(report *ComplianceReport) *SARIFLog {
	if report == nil {
		return emptySARIFLog()
	}
	rules, results := buildComplianceRulesAndResults(report)
	return wrapSARIFLog(rules, results)
}

// buildComplianceRulesAndResults emits one rule per unique control and one
// result per failing control entry. Sorts a copy of FailingDetails by
// (severity desc, ControlID, Title, Stack, Component) so SARIF rule indexes
// and results are byte-stable across runs.
func buildComplianceRulesAndResults(report *ComplianceReport) ([]Rule, []Result) {
	details := sortedComplianceDetails(report.FailingDetails)
	rules := make([]Rule, 0, len(details))
	results := make([]Result, 0, len(details))
	index := make(map[string]int)

	for i := range details {
		ctrl := &details[i]
		id := ctrl.ControlID
		if id == "" {
			id = slugify(ctrl.Title)
		}
		if _, ok := index[id]; !ok {
			index[id] = len(rules)
			rules = append(rules, complianceRule(id, ctrl, report.Framework))
		}
		idx := index[id]
		results = append(results, complianceResult(id, idx, ctrl, report.Framework))
	}
	return rules, results
}

// sortedComplianceDetails returns a stable, deterministic ordering of failing
// controls so SARIF output is byte-stable for the same input report.
func sortedComplianceDetails(details []ComplianceControl) []ComplianceControl {
	out := make([]ComplianceControl, len(details))
	copy(out, details)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		if out[i].ControlID != out[j].ControlID {
			return out[i].ControlID < out[j].ControlID
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		if out[i].Stack != out[j].Stack {
			return out[i].Stack < out[j].Stack
		}
		return out[i].Component < out[j].Component
	})
	return out
}

// complianceRule builds a SARIF Rule for a compliance control.
func complianceRule(id string, ctrl *ComplianceControl, framework string) Rule {
	return Rule{
		ID:               id,
		Name:             ctrl.Title,
		ShortDescription: &MultiformatText{Text: ctrl.Title},
		DefaultConfig:    &RuleConfig{Level: severityToLevel(ctrl.Severity)},
		Properties: map[string]any{
			propFramework: framework,
			propSeverity:  string(ctrl.Severity),
		},
	}
}

// complianceResult builds a SARIF Result for a failing compliance control.
func complianceResult(id string, ruleIndex int, ctrl *ComplianceControl, framework string) Result {
	return Result{
		RuleID:    id,
		RuleIndex: &ruleIndex,
		Level:     severityToLevel(ctrl.Severity),
		Kind:      sarifKindFail,
		Message:   MultiformatText{Text: ctrl.Title},
		Properties: map[string]any{
			propFramework: framework,
			"control":     ctrl.ControlID,
			propSeverity:  string(ctrl.Severity),
			"stack":       ctrl.Stack,
			"component":   ctrl.Component,
		},
	}
}
