package doctor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/dependency"
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
	_, validates := e.Stacks.(DependencyValidator)
	_, reports := e.Stacks.(DependencyTargetReporter)
	var supported []edit
	for _, change := range edits {
		if change.kind == ruleComponentDependencies && (!validates || !reports || !e.canCheckStacks(p)) {
			f := &r.Findings[change.finding]
			f.Fixable = false
			f.Status = statusVerificationIncomplete
			f.Reason = "Required dependencies cannot be verified because " + e.dependencyBlocker(p, validates && reports)
			f.Remediation = change.manualRemediation()
			continue
		}
		supported = append(supported, change)
	}
	return supported
}

// dependencyBlocker names what keeps the dependency graph from being validated and compared.
func (e *Engine) dependencyBlocker(p *project, capable bool) string {
	if !capable {
		return "no dependency validator is available to report missing targets and other graph errors"
	}
	return e.stackBlocker(p)
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
		r.Checks = append(r.Checks, Check{ID: ruleDependencies, Status: "incomplete", Reason: "Dependency validation is incomplete because " + e.dependencyBlocker(p, ok)})
		return nil
	}
	config, err := staticConfig(p)
	if err != nil {
		r.Checks = append(r.Checks, Check{ID: ruleDependencies, Status: "incomplete", Reason: "Configuration could not be decoded for dependency validation: " + err.Error()})
		return nil //nolint:nilerr // Coverage gap, as with stack validation.
	}
	inputs, err := e.staticInputs(p, config)
	if err != nil {
		return err
	}
	e.emit(Event{Phase: "Check", Label: "Dependencies"})
	if err := v.ValidateDependencies(ctx, config, inputs); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.Findings = append(r.Findings, dependencyFindings(p, err)...)
	}
	status := checkStatus(r, ruleDependencies)
	r.Checks = append(r.Checks, Check{ID: ruleDependencies, Status: status, Files: len(inputs.Files)})
	e.emit(Event{Phase: "Check", Label: "Dependencies", Done: true, Status: status})
	return nil
}

// dependencyFindings turns the errors of the dependency graph into findings, one per error, each
// located at the dependency that causes it when the declaration can be found in a stack manifest.
func dependencyFindings(p *project, err error) []Finding {
	var findings []Finding
	for _, leaf := range graphErrors(err) {
		finding := Finding{RuleID: ruleDependencies, Category: categoryValidation, Severity: validation.SeverityError, Message: dependencyMessage(leaf), Status: statusManual, Remediation: "Define or enable required targets, correct dependency references, and resolve dependency cycles"}
		if at, ok := locateDependencyError(p, leaf); ok {
			finding.File, finding.Path, finding.Line, finding.Column = at.doc.path, at.path, at.line, at.column
		}
		findings = append(findings, finding)
	}
	return findings
}

// graphErrors flattens joined and wrapped graph errors into the errors that name a component. Errors
// that do not, such as a stack that failed to load, stay whole.
func graphErrors(err error) []error {
	//nolint:errorlint // The located errors are matched where they sit in the tree, not through wrappers.
	switch err.(type) {
	case *dependency.DependencyTargetError, *dependency.DependencyDeclarationError:
		return []error{err}
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var leaves []error
		for _, child := range joined.Unwrap() {
			leaves = append(leaves, graphErrors(child)...)
		}
		return leaves
	}
	if wrapped := errors.Unwrap(err); wrapped != nil && namesComponent(err) {
		return graphErrors(wrapped)
	}
	return []error{err}
}

// namesComponent reports whether err contains an error that identifies a component.
func namesComponent(err error) bool {
	var target *dependency.DependencyTargetError
	var declaration *dependency.DependencyDeclarationError
	return errors.As(err, &target) || errors.As(err, &declaration)
}

// dependencyMessage describes a graph error without the key=value detail of the underlying error.
func dependencyMessage(err error) string {
	var target *dependency.DependencyTargetError
	var cycle *dependency.CircularDependencyError
	var declaration *dependency.DependencyDeclarationError
	switch {
	case errors.As(err, &target):
		need := fmt.Sprintf("%s in %s requires %s in %s", target.Component, target.Stack, target.TargetComponent, target.TargetStack)
		if target.Reason == dependency.TargetDisabled {
			return need + ", but that component is disabled"
		}
		return need + ", but no such concrete component exists (it is missing or abstract)"
	case errors.As(err, &cycle):
		return "Dependency cycle: " + strings.Join(cycle.Path, " → ")
	case errors.Is(err, dependency.ErrSelfDependency) && errors.As(err, &declaration):
		return fmt.Sprintf("%s in %s depends on itself", declaration.Component, declaration.Stack)
	}
	return err.Error()
}

// dependencySite is where a dependency is declared in a stack manifest.
type dependencySite struct {
	doc          *document
	path         string
	line, column int
}

// locateDependencyError finds the declaration behind a graph error: the dependency entry that names the
// missing target when there is one, otherwise the component that declares it.
func locateDependencyError(p *project, err error) (dependencySite, bool) {
	var target *dependency.DependencyTargetError
	var declaration *dependency.DependencyDeclarationError
	switch {
	case errors.As(err, &target):
		return locateDependency(p, target.Component, target.TargetComponent)
	case errors.As(err, &declaration):
		var cycle *dependency.CircularDependencyError
		if errors.As(declaration.Err, &cycle) && len(cycle.Path) > 1 {
			// The cycle is closed by the last node's dependency on the first.
			return locateDependency(p, declaration.Component, componentOfNode(cycle.Path[1], declaration.Stack))
		}
		return locateDependency(p, declaration.Component, declaration.Component)
	}
	return dependencySite{}, false
}

// componentOfNode recovers the component name from a graph node ID of the form component-stack.
func componentOfNode(node, stack string) string {
	return strings.TrimSuffix(node, "-"+stack)
}

// dependencySections are the places a component declares dependencies, modern first.
var dependencySections = [][]string{{"dependencies", "components"}, {"settings", "depends_on"}}

// locateDependency finds the manifest entry that makes component depend on target. Declarations can be
// inherited, so a component of another name that declares the entry is used when the component itself does
// not; failing that the component's own node is the best available location.
func locateDependency(p *project, component, target string) (dependencySite, bool) {
	components := terraformComponentNodes(p)
	for _, own := range []bool{true, false} {
		for i := range components {
			if (components[i].name == component) != own {
				continue
			}
			if site, ok := dependencyEntry(components[i].doc, components[i].name, components[i].node, target); ok {
				return site, true
			}
		}
	}
	for i := range components {
		if components[i].name == component {
			path := "components.terraform." + component
			line, column := locateField(components[i].doc.node, path)
			return dependencySite{doc: components[i].doc, path: path, line: line, column: column}, true
		}
	}
	return dependencySite{}, false
}

// stackComponent is a Terraform component as one stack manifest declares it.
type stackComponent struct {
	doc  *document
	name string
	node *yaml.Node
}

// terraformComponentNodes lists the Terraform components declared in the stack manifests, in file order.
func terraformComponentNodes(p *project) []stackComponent {
	var components []stackComponent
	for _, d := range p.docs {
		if d.kind != documentStack || d.node == nil {
			continue
		}
		terraform := nodeAt(rootNode(d.node), []string{"components", "terraform"})
		if terraform == nil || terraform.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(terraform.Content); i += 2 {
			components = append(components, stackComponent{doc: d, name: terraform.Content[i].Value, node: terraform.Content[i+1]})
		}
	}
	return components
}

// dependencyEntry finds the entry of one component that names target.
func dependencyEntry(d *document, name string, node *yaml.Node, target string) (dependencySite, bool) {
	for _, section := range dependencySections {
		entries := nodeAt(node, section)
		if entries == nil {
			continue
		}
		for i, entry := range dependencyEntries(entries) {
			if dependencyTarget(entry.node) != target {
				continue
			}
			path := strings.Join(append([]string{"components", "terraform", name}, append(append([]string{}, section...), entry.key(i))...), dot)
			line, column := locateField(d.node, path)
			return dependencySite{doc: d, path: path, line: line, column: column}, true
		}
	}
	return dependencySite{}, false
}

// dependencyListEntry is one entry of a dependency list or map, with the key that addresses it.
type dependencyListEntry struct {
	node *yaml.Node
	name string
}

func (e dependencyListEntry) key(index int) string {
	if e.name != "" {
		return e.name
	}
	return strconv.Itoa(index)
}

// dependencyEntries lists the entries of a dependency sequence, or the values of the legacy map form.
func dependencyEntries(n *yaml.Node) []dependencyListEntry {
	var entries []dependencyListEntry
	switch n.Kind {
	case yaml.SequenceNode:
		for _, item := range n.Content {
			entries = append(entries, dependencyListEntry{node: item})
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			entries = append(entries, dependencyListEntry{node: n.Content[i+1], name: n.Content[i].Value})
		}
	}
	return entries
}

// dependencyTarget returns the component an entry names: a plain string or a mapping's component or name.
func dependencyTarget(entry *yaml.Node) string {
	if entry.Kind == yaml.ScalarNode {
		return entry.Value
	}
	for _, key := range []string{"component", "name"} {
		if value := mappingValue(entry, key); value != nil {
			return value.Value
		}
	}
	return ""
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
