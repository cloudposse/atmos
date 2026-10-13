package doctor

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

type edit struct {
	doc   *document
	path  []string
	kind  string
	old   string
	value string
	// from is the legacy value the edit replaces, for messages.
	from    string
	finding int
}

func scanDocument(d *document, p *project, o *Options, r *Report, v validator.Validator) ([]edit, error) {
	if d.node == nil {
		return nil, nil
	}
	var edits []edit
	if err := scanSchema(d, o, r, v); err != nil {
		return nil, err
	}
	walk(d.node, nil, func(n *yaml.Node, path []string) {
		if len(path) == 0 {
			return
		}
		candidate := findCandidate(d, n, path)
		if candidate != nil && enabled(o, candidate.kind, categoryDeprecations) {
			if assessCandidate(p, r, candidate) {
				edits = append(edits, *candidate)
			}
		}
		modernizationFinding(d, o, r, n, path)
	})
	return edits, nil
}

func renameCandidate(d *document, n *yaml.Node, path []string, old string) *edit {
	v := mappingValue(n, old)
	if v == nil || v.Tag != "!!str" || (v.Value != semanticComponent && v.Value != semanticStack) {
		return nil
	}
	return &edit{doc: d, path: path, kind: ruleCommandProvides, old: old, value: v.Value, from: v.Value}
}

// Only declared command trees qualify; user data may also contain a key named commands.
func commandFieldPath(root *yaml.Node, path []string) bool {
	if len(path) < 4 || len(path)%2 != 0 {
		return false
	}
	for i := 0; i < len(path)-2; i += 2 {
		if path[i] != "commands" {
			return false
		}
		if node := nodeAt(rootNode(root), path[:i+1]); node == nil || node.Kind != yaml.SequenceNode {
			return false
		}
	}
	last := path[len(path)-2]
	if last != "arguments" && last != "flags" {
		return false
	}
	node := nodeAt(rootNode(root), path[:len(path)-1])
	return node != nil && node.Kind == yaml.SequenceNode
}

func hookEventPath(path []string) bool {
	if len(path) < 4 || path[len(path)-4] != "hooks" || path[len(path)-2] != "events" {
		return false
	}
	prefix := path[:len(path)-4]
	if len(prefix) == 0 {
		return true
	}
	if len(prefix) == 1 {
		return oneOfComponentType(prefix[0])
	}
	return len(prefix) == 3 && prefix[0] == "components" && oneOfComponentType(prefix[1])
}

func oneOfComponentType(value string) bool {
	switch value {
	case "terraform", "helmfile", "helm", "kubernetes", "packer", "ansible":
		return true
	}
	return false
}

// migrationConflict explains why the replacement already exists, or returns "" when it does not.
// Only the exact replacement key conflicts: a sibling such as init.pass_vars does not.
func migrationConflict(n *yaml.Node, e *edit) string {
	switch e.kind {
	case ruleCommandProvides:
		if mappingValue(n, "provides") != nil {
			return "provides is already declared, so " + e.old + " is redundant; keep the value you intend and delete the other by hand"
		}
	case ruleTerraformReconfigure:
		if mappingValue(mappingValue(n, "init"), "reconfigure") != nil {
			return "init.reconfigure is already declared and takes precedence over init_run_reconfigure; confirm it is the value you want, then delete init_run_reconfigure by hand"
		}
	}
	return ""
}

//nolint:unparam // The generic helper is exercised with other keys in tests.
func contains(path []string, key string) bool {
	for _, part := range path {
		if part == key {
			return true
		}
	}
	return false
}

func knownHook(value string) bool {
	normalized := strings.ReplaceAll(value, "-", dot)
	for _, event := range []hooks.HookEvent{hooks.BeforeTerraformInit, hooks.AfterTerraformInit, hooks.BeforeTerraformPlan, hooks.AfterTerraformPlan, hooks.BeforeTerraformApply, hooks.AfterTerraformApply, hooks.BeforeTerraformDeploy, hooks.AfterTerraformDeploy, hooks.BeforeTerraformOutput, hooks.AfterTerraformOutput, hooks.BeforeTerraformTest, hooks.AfterTerraformTest, hooks.BeforeHelmfileApply, hooks.AfterHelmfileApply} {
		if string(event) == normalized {
			return true
		}
	}
	return false
}

func displayNodePath(path []string) string {
	var b strings.Builder
	for i, part := range path {
		if _, err := strconv.Atoi(part); err == nil {
			fmt.Fprintf(&b, "[%s]", part)
		} else {
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(part)
		}
	}
	return b.String()
}

func scanSchema(d *document, o *Options, r *Report, v validator.Validator) error {
	if d.schema == "" {
		return nil
	}
	if enabled(o, ruleSchema, categoryValidation) {
		validateDocumentSchema(d, r, v)
	}
	if !enabled(o, ruleDeprecatedField, categoryDeprecations) {
		return nil
	}
	fields, err := validator.FindDeprecatedYAMLFields(&schema.AtmosConfiguration{}, d.schema, d.content)
	if err != nil {
		return err
	}
	for _, field := range fields {
		r.Findings = append(r.Findings, deprecationFinding(d, field))
	}
	return nil
}

func validateDocumentSchema(d *document, r *Report, v validator.Validator) {
	results, err := v.ValidateYAMLContent(d.schema, d.content)
	if err != nil {
		r.Findings = append(r.Findings, Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Path: schemaRootField, Line: 1, Column: 1, Message: err.Error(), Status: statusManual})
		return
	}
	for _, failure := range meaningfulFailures(results) {
		line, column := locateField(d.node, failure.Field())
		r.Findings = append(r.Findings, Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Path: failure.Field(), Line: line, Column: column, Message: failure.Description(), Status: statusManual})
	}
}

func deprecationFinding(d *document, field validator.DeprecatedField) Finding {
	f := Finding{RuleID: ruleDeprecatedField, Category: categoryDeprecations, Severity: validation.SeverityWarning, File: d.path, Path: field.Path, Message: field.Path + " is deprecated", Remediation: "Use " + field.Replacement, Status: statusManual, Documentation: "https://atmos.tools/cli/commands/doctor"}
	if strings.Contains(field.Path, "depends_on") {
		f.Reason = "Legacy and modern dependency handling differ for missing targets, optional edges, and context matching; automatic migration has not passed the behavioral contract"
	}
	if strings.HasSuffix(field.Path, "name_pattern") {
		f.Status = statusVerificationIncomplete
		f.Reason = "Complete stack and Terraform workspace equivalence is required; no automatic naming migration is enabled"
	}
	walk(d.node, nil, func(n *yaml.Node, path []string) {
		if displayNodePath(path) == field.Path {
			f.Line = n.Line
			f.Column = n.Column
			if inertSemanticType(field.Path, n) {
				f.Remediation = inertRemediation(n.Value)
			}
		}
	})
	return f
}

// inertRemediation explains what to do with a deprecated semantic type the runtime ignores.
func inertRemediation(value string) string {
	for _, known := range []string{semanticComponent, semanticStack} {
		if strings.EqualFold(value, known) {
			return fmt.Sprintf("Replace it with provides: %s by hand; the value is case-sensitive, so %q has no effect", known, value)
		}
	}
	return fmt.Sprintf("Remove it: only %q and %q have an effect, and %q is neither", semanticComponent, semanticStack, value)
}

// inertSemanticType reports a deprecated command argument type or flag semantic_type whose value is not
// one the runtime acts on, so migrating it to provides would add a setting that does nothing.
func inertSemanticType(path string, n *yaml.Node) bool {
	legacy := strings.HasSuffix(path, "].semantic_type") || strings.HasSuffix(path, "].type") && strings.Contains(path, ".arguments[")
	return legacy && n.Kind == yaml.ScalarNode && n.Value != semanticComponent && n.Value != semanticStack
}

func findCandidate(d *document, n *yaml.Node, path []string) *edit {
	var candidate *edit
	if d.kind == documentConfig && n.Kind == yaml.MappingNode && commandFieldPath(d.node, path) {
		parent := path[len(path)-2]
		if parent == "arguments" {
			candidate = renameCandidate(d, n, path, "type")
		}
		if parent == "flags" {
			candidate = renameCandidate(d, n, path, "semantic_type")
		}
	}
	if candidate == nil {
		candidate = reconfigureCandidate(d, n, path)
	}
	if isLegacyHook(d, n, path) {
		candidate = &edit{doc: d, path: path, kind: ruleHookEvent, value: strings.ReplaceAll(n.Value, "-", dot), from: n.Value}
	}
	return candidate
}

func assessCandidate(p *project, r *Report, candidate *edit) bool {
	d := candidate.doc
	path := candidate.path
	n := nodeAt(rootNode(d.node), path)
	f := Finding{RuleID: candidate.kind, Category: categoryDeprecations, Severity: validation.SeverityWarning, File: d.path, Path: displayNodePath(path), Line: n.Line, Column: n.Column, Message: candidate.message(), Remediation: remediationFix, Documentation: "https://atmos.tools/cli/commands/doctor", Status: "available", Fixable: true}
	if reason := p.blocker(candidate); reason != "" {
		f.Fixable = false
		f.Status = statusVerificationIncomplete
		f.Reason = "Not fixed automatically because " + reason
		f.Remediation = candidate.manualRemediation()
	}
	if candidate.old != "" {
		old := mappingValue(n, candidate.old)
		if old != nil {
			f.Line = old.Line
			f.Column = old.Column
		}
		if conflict := migrationConflict(n, candidate); conflict != "" {
			f.Fixable = false
			f.Status = statusManual
			f.Reason = conflict
			f.Remediation = candidate.conflictRemediation()
		}
	}
	index := replaceDeprecation(r, d, &f)
	candidate.finding = index
	return f.Fixable
}

func modernizationFinding(d *document, o *Options, r *Report, n *yaml.Node, path []string) {
	last := path[len(path)-1]
	if enabled(o, ruleModernization, ruleModernization) && n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		if last == "uses" && strings.HasPrefix(n.Value, "cloudposse/github-action-atmos") {
			r.Findings = append(r.Findings, Finding{RuleID: ruleModernization, Category: ruleModernization, Severity: validation.SeverityWarning, File: d.path, Line: n.Line, Column: n.Column, Message: "Legacy Atmos GitHub Action", Remediation: "Review migration to Native CI and direct Atmos commands", Status: statusManual, Documentation: "https://atmos.tools/ci"})
		}
	}
}

// componentSourceKey matches a `source:` declaration and the `uri:` inside a `source:` mapping,
// which is how component.yaml (ComponentVendorConfig) and source provisioning name their upstream.
func componentSourceKey(path []string) bool {
	return pathEndsWith(path, "source") || pathEndsWith(path, "source", "uri")
}

// pathEndsWith reports whether path ends with the given keys, in order.
func pathEndsWith(path []string, suffix ...string) bool {
	return len(path) >= len(suffix) && slices.Equal(path[len(path)-len(suffix):], suffix)
}

func reconfigureCandidate(d *document, n *yaml.Node, path []string) *edit {
	var candidate *edit
	if d.kind == documentConfig && strings.Join(path, dot) == "components.terraform" && n.Kind == yaml.MappingNode {
		if old := mappingValue(n, "init_run_reconfigure"); old != nil && old.Tag == "!!bool" {
			value := "auto"
			var enabled bool
			_ = old.Decode(&enabled)
			if !enabled {
				value = "never"
			}
			candidate = &edit{doc: d, path: path, kind: ruleTerraformReconfigure, old: "init_run_reconfigure", value: value}
		}
	}
	return candidate
}

func isLegacyHook(d *document, n *yaml.Node, path []string) bool {
	return d.kind == "stack" && n.Kind == yaml.ScalarNode && n.Tag == "!!str" && hookEventPath(path) && strings.Contains(n.Value, "-") && knownHook(n.Value)
}

func replaceDeprecation(r *Report, d *document, f *Finding) int {
	// Replace the generic schema warning at the same source location.
	index := -1
	for i := range r.Findings {
		if r.Findings[i].File == d.path && r.Findings[i].Line == f.Line && r.Findings[i].RuleID == ruleDeprecatedField {
			index = i
			break
		}
	}
	if index < 0 {
		index = len(r.Findings)
		r.Findings = append(r.Findings, *f)
	} else {
		r.Findings[index] = *f
	}
	return index
}
