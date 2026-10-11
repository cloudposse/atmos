package doctor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

type edit struct {
	doc     *document
	path    []string
	kind    string
	old     string
	value   string
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
			if assessCandidate(p, o, r, candidate) {
				edits = append(edits, *candidate)
			}
		}
		modernizationFinding(d, o, r, n, path)
	})
	return edits, nil
}

func renameCandidate(d *document, n *yaml.Node, path []string, old string) *edit {
	v := mappingValue(n, old)
	if v == nil || v.Tag != "!!str" || (v.Value != "component" && v.Value != "stack") {
		return nil
	}
	return &edit{doc: d, path: path, kind: ruleCommandProvides, old: old, value: v.Value}
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

func hasMigrationConflict(n *yaml.Node, e *edit) bool {
	if e.kind == ruleCommandProvides {
		return mappingValue(n, "provides") != nil
	}
	if e.kind == ruleTerraformReconfigure {
		return mappingValue(n, "init") != nil
	}
	return false
}

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

func expressionPath(path []string) string {
	var b strings.Builder
	for _, part := range path {
		if _, err := strconv.Atoi(part); err == nil {
			fmt.Fprintf(&b, "[%s]", part)
		} else {
			fmt.Fprintf(&b, "[%s]", strconv.Quote(part))
		}
	}
	return dot + b.String()
}

func applyEdit(content []byte, e *edit) ([]byte, error) {
	p := expressionPath(e.path)
	switch e.kind {
	case ruleCommandProvides:
		return atmosyaml.Stage(e.doc.path, content, "("+p+"["+strconv.Quote(e.old)+"] | key) = \"provides\"")
	case ruleHookEvent:
		return atmosyaml.Stage(e.doc.path, content, p+" = "+strconv.Quote(e.value))
	case ruleTerraformReconfigure:
		return atmosyaml.Stage(e.doc.path, content, p+".init.reconfigure = "+strconv.Quote(e.value)+" | del("+p+".init_run_reconfigure)")
	default:
		return nil, ErrVerification
	}
}

// verifyDocument uses the real field consumers and compares every other authored value.
// It intentionally refuses transformations that lose comments.
func verifyDocument(before, after []byte, edits []edit) error {
	a, err := parse(before)
	if err != nil {
		return err
	}
	b, err := parse(after)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(comments(a), comments(b)) {
		return fmt.Errorf("%w: comments changed", ErrVerification)
	}
	am, err := semanticDocument(a, edits)
	if err != nil {
		return err
	}
	bm, err := semanticDocument(b, edits)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(am, bm) {
		return fmt.Errorf("%w: effective or unrelated YAML values changed", ErrVerification)
	}
	return nil
}

func comments(n *yaml.Node) []string {
	var values []string
	var visit func(*yaml.Node)
	visit = func(n *yaml.Node) {
		for _, s := range []string{n.HeadComment, n.LineComment, n.FootComment} {
			if s != "" {
				values = append(values, s)
			}
		}
		for _, child := range n.Content {
			visit(child)
		}
	}
	visit(n)
	return values
}

func semanticDocument(n *yaml.Node, edits []edit) (any, error) {
	for _, e := range edits {
		node := nodeAt(rootNode(n), e.path)
		if node == nil {
			return nil, ErrVerification
		}
		if err := normalizeNode(node, &e); err != nil {
			return nil, err
		}
	}
	return canonicalNode(rootNode(n)), nil
}

func canonicalNode(n *yaml.Node) any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, _ := json.Marshal(canonicalNode(n.Content[i]))
			m[string(key)] = canonicalNode(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		result := []any{}
		for _, child := range n.Content {
			result = append(result, canonicalNode(child))
		}
		return result
	case yaml.AliasNode:
		return []string{n.Tag, n.Value, n.Anchor}
	default:
		return []string{n.Tag, n.Value, n.Anchor}
	}
}

func nodeAt(n *yaml.Node, path []string) *yaml.Node {
	for _, part := range path {
		if n == nil {
			return nil
		}
		if n.Kind == yaml.SequenceNode {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(n.Content) {
				return nil
			}
			n = n.Content[index]
		} else {
			n = mappingValue(n, part)
		}
	}
	return n
}

func removeKey(n *yaml.Node, key string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return
		}
	}
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
		r.Findings = append(r.Findings, Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Message: err.Error(), Status: statusManual})
		return
	}
	for _, failure := range results {
		r.Findings = append(r.Findings, Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Path: failure.Field(), Message: failure.Description(), Status: statusManual})
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
		}
	})
	return f
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
		candidate = &edit{doc: d, path: path, kind: ruleHookEvent, value: strings.ReplaceAll(n.Value, "-", dot)}
	}
	return candidate
}

func assessCandidate(p *project, o *Options, r *Report, candidate *edit) bool {
	d := candidate.doc
	path := candidate.path
	n := nodeAt(rootNode(d.node), path)
	f := Finding{RuleID: candidate.kind, Category: categoryDeprecations, Severity: validation.SeverityWarning, File: d.path, Path: displayNodePath(path), Line: n.Line, Column: n.Column, Message: "Legacy " + candidate.kind + " declaration", Remediation: "Run atmos doctor --fix", Documentation: "https://atmos.tools/cli/commands/doctor", Status: "available", Fixable: true}
	if d.dynamic || !p.complete || d.kind == documentConfig && p.configCount != 1 || len(o.Profiles) > 0 {
		f.Fixable = false
		f.Status = statusVerificationIncomplete
		f.Reason = "Dynamic or layered configuration requires complete effective-input verification"
	}
	if candidate.old != "" {
		old := mappingValue(n, candidate.old)
		if old != nil {
			f.Line = old.Line
			f.Column = old.Column
		}
		if hasMigrationConflict(n, candidate) {
			f.Fixable = false
			f.Status = statusManual
			f.Reason = "The replacement is already declared; preserve both values for manual review"
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
		if last == "source" && strings.Contains(n.Value, "cloudposse/terraform-aws-components") {
			r.Findings = append(r.Findings, Finding{RuleID: ruleModernization, Category: ruleModernization, Severity: validation.SeverityWarning, File: d.path, Line: n.Line, Column: n.Column, Message: "Legacy component monorepo source", Remediation: "Select the corresponding cloudposse-terraform-components repository and verify its pinned version", Status: statusManual})
		}
	}
}

func normalizeNode(node *yaml.Node, e *edit) error {
	switch e.kind {
	case ruleCommandProvides:
		var effective string
		if e.old == "type" {
			var a schema.CommandArgument
			if err := node.Decode(&a); err != nil {
				return err
			}
			effective = a.EffectiveProvides()
		} else {
			var f schema.CommandFlag
			if err := node.Decode(&f); err != nil {
				return err
			}
			effective = f.EffectiveProvides()
		}
		removeKey(node, e.old)
		removeKey(node, "provides")
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "provides"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: effective})
	case ruleHookEvent:
		node.Value = string(hooks.HookEvent(strings.ReplaceAll(node.Value, "-", dot)).Normalize())
	case ruleTerraformReconfigure:
		value := schema.Terraform{InitRunReconfigure: true}
		if err := node.Decode(&value); err != nil {
			return err
		}
		effective := string(value.EffectiveInitReconfigure())
		removeKey(node, "init_run_reconfigure")
		init := mappingValue(node, "init")
		if init != nil {
			removeKey(init, "reconfigure")
			if len(init.Content) == 0 {
				removeKey(node, "init")
			}
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "init_run_reconfigure"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: effective})
	}
	return nil
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
