package doctor

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/validation"
)

// dependencyCandidate deliberately supports literal component lists only. Legacy
// context selectors and file/folder entries need different transformations.
func dependencyCandidate(d *document, n *yaml.Node, path []string) *edit {
	if d.kind != documentStack || len(path) != 3 || path[0] != "components" || path[1] != "terraform" {
		return nil
	}
	legacy := nodeAt(n, []string{"settings", "depends_on"})
	if legacy == nil || legacy.Kind != yaml.SequenceNode || len(legacy.Content) == 0 {
		return nil
	}
	for _, entry := range legacy.Content {
		if !simpleDependencyEntry(entry) {
			return nil
		}
	}
	return &edit{doc: d, path: path, kind: ruleComponentDependencies, old: "settings.depends_on"}
}

func simpleDependencyEntry(entry *yaml.Node) bool {
	if entry.Kind != yaml.MappingNode || mappingValue(entry, "component") == nil {
		return false
	}
	for i := 0; i+1 < len(entry.Content); i += 2 {
		key, value := entry.Content[i], entry.Content[i+1]
		if (key.Value != "component" && key.Value != "stack") || value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Value == "" {
			return false
		}
	}
	return true
}

// normalizeDependencyMigration compares the exact moved subtree, not legacy runtime
// equivalence. Adopting modern required semantics is intentional for this repair.
func normalizeDependencyMigration(node *yaml.Node) error {
	settings := mappingValue(node, "settings")
	dependencies := mappingValue(node, "dependencies")
	legacy := mappingValue(settings, "depends_on")
	modern := mappingValue(dependencies, "components")
	if (legacy == nil) == (modern == nil) {
		return ErrVerification
	}
	if legacy != nil && dependencies != nil && dependencies.Kind == yaml.MappingNode && len(dependencies.Content) == 0 {
		removeKey(node, "dependencies")
	}
	if modern != nil {
		removeKey(dependencies, "components")
		if len(dependencies.Content) == 0 {
			removeKey(node, "dependencies")
		}
		if settings == nil {
			return ErrVerification
		}
		settings.Content = append(settings.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "depends_on"}, modern)
	}
	return nil
}

// requireDependencyValidation prevents claiming a verified migration when its
// resulting required-dependency errors cannot be checked.
func (e *Engine) requireDependencyValidation(p *project, r *Report, edits []edit) []edit {
	_, hasValidator := e.Stacks.(DependencyValidator)
	var supported []edit
	for _, change := range edits {
		if change.kind == ruleComponentDependencies && (!hasValidator || !e.canCheckStacks(p)) {
			f := &r.Findings[change.finding]
			f.Fixable = false
			f.Status = statusVerificationIncomplete
			f.Reason = "Required dependencies need complete static inputs and a dependency validator to report missing targets and other graph errors"
			f.Remediation = change.manualRemediation()
			continue
		}
		supported = append(supported, change)
	}
	return supported
}

func hasDependencyMigration(candidates []candidate) bool {
	for _, c := range candidates {
		for _, change := range c.edits {
			if change.kind == ruleComponentDependencies {
				return true
			}
		}
	}
	return false
}

func (e *Engine) checkDependencies(ctx context.Context, p *project, o *Options, r *Report) error {
	if !enabled(o, ruleDependencies, categoryValidation) || stackCount(p) == 0 {
		return nil
	}
	v, ok := e.Stacks.(DependencyValidator)
	if !ok || !e.canCheckStacks(p) {
		r.Checks = append(r.Checks, Check{ID: ruleDependencies, Status: "incomplete", Reason: "Dependency validation needs complete static inputs and a dependency validator"})
		return nil
	}
	config, err := staticConfig(p)
	if err != nil {
		r.Checks = append(r.Checks, Check{ID: ruleDependencies, Status: "incomplete", Reason: "Configuration could not be decoded for dependency validation: " + err.Error()})
		return nil //nolint:nilerr // Coverage gap, as with stack validation.
	}
	inputs, err := staticInputs(p, config)
	if err != nil {
		return err
	}
	e.emit(Event{Phase: "Check", Label: "Dependencies"})
	if err := v.ValidateDependencies(ctx, config, inputs); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.Findings = append(r.Findings, Finding{RuleID: ruleDependencies, Category: categoryValidation, Severity: validation.SeverityError, Message: err.Error(), Status: statusManual, Remediation: "Define or enable required targets, correct dependency references, and resolve dependency cycles"})
	}
	status := checkStatus(r, ruleDependencies)
	r.Checks = append(r.Checks, Check{ID: ruleDependencies, Status: status, Files: len(inputs)})
	e.emit(Event{Phase: "Check", Label: "Dependencies", Done: true, Status: status})
	return nil
}

// recordDependencyVerification keeps a successful transformation distinct from
// errors in its resulting graph, including projected errors during dry runs.
func recordDependencyVerification(r, candidate *Report, dryRun bool) error {
	var graphCheck *Check
	for i := range candidate.Checks {
		if candidate.Checks[i].ID == ruleDependencies {
			graphCheck = &candidate.Checks[i]
			break
		}
	}
	if graphCheck == nil || (graphCheck.Status != statusPassed && graphCheck.Status != statusFailed) {
		return fmt.Errorf("%w: candidate dependency validation is incomplete", ErrVerification)
	}
	check := *graphCheck
	check.ID = "candidate-dependencies"
	r.Checks = append(r.Checks, check)
	if dryRun {
		appendProjectedDependencyFindings(r, candidate)
	}
	return nil
}

func appendProjectedDependencyFindings(r, candidate *Report) {
	existing := map[string]bool{}
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.RuleID == ruleDependencies {
			existing[f.Message] = true
		}
	}
	for i := range candidate.Findings {
		f := &candidate.Findings[i]
		if f.RuleID != ruleDependencies || existing[f.Message] {
			continue
		}
		finding := *f
		finding.Reason = "The proposed migration has this dependency error; dry run left the source unchanged"
		r.Findings = append(r.Findings, finding)
	}
}
