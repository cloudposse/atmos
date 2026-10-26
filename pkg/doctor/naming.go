package doctor

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

const ruleLegacyNaming = "legacy-naming"

// NamingReporter reports production names without executing dynamic configuration.
type NamingReporter interface {
	NamingResults(context.Context, *schema.AtmosConfiguration, StackInputs) (map[naming.Identity]naming.Result, error)
}

func scanNaming(d *document, p *project, o *Options, r *Report) []edit {
	if !enabled(o, ruleLegacyNaming, categoryDeprecations) || (d.kind != documentConfig && d.kind != documentStack) {
		return nil
	}
	var edits []edit
	walk(d.node, nil, func(n *yaml.Node, path []string) {
		if !naming.Field(path) || n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
			return
		}
		c, f := namingRecommendation(d, p, n, path)
		if c == nil {
			return
		}
		c.finding = replaceDeprecation(r, d, f)
		if f.Fixable {
			edits = append(edits, *c)
		}
	})
	return edits
}

func namingRecommendation(d *document, p *project, n *yaml.Node, path []string) (*edit, *Finding) {
	field := path[len(path)-1]
	pattern := strings.HasSuffix(field, "_pattern")
	replacement, tokens, unsupported := naming.Upgrade(n.Value)
	if !pattern && len(tokens) == 0 {
		return nil, nil
	}
	target := strings.TrimSuffix(field, "_pattern") + "_template"
	if !pattern {
		target = field
	}
	c := edit{doc: d, path: slices.Clone(path[:len(path)-1]), kind: ruleLegacyNaming, old: field, target: target, from: n.Value, value: replacement}
	f := Finding{RuleID: ruleLegacyNaming, Category: categoryDeprecations, Severity: validation.SeverityWarning, File: d.path, Path: displayNodePath(path), Line: n.Line, Column: n.Column, Message: "Legacy naming syntax; use Go templates", Status: "available", Fixable: true, Remediation: c.manualRemediation(), Documentation: "https://atmos.tools/cli/commands/doctor"}
	if pattern {
		f.Message = "Legacy naming field " + field + "; use " + target
	}
	if len(tokens) > 0 && strings.Contains(n.Value, "{{") {
		f.Message += "; mixed naming styles"
	}
	reason := namingBlocker(d, p, &c, unsupported)
	if reason != "" {
		f.Fixable = false
		f.Status = statusVerificationIncomplete
		f.Reason = reason
	}
	return &c, &f
}

func namingBlocker(d *document, p *project, c *edit, unsupported []string) string {
	reason := p.blocker(c)
	if len(unsupported) > 0 {
		reason = "Tokens have no verified equivalent in this naming context: " + strings.Join(unsupported, ", ")
	}
	if !naming.SafeTemplate(c.from) || !naming.SafeTemplate(c.value) {
		reason = "The naming expression cannot be evaluated safely"
	}
	if c.old != c.target && nodeAt(rootNode(d.node), append(slices.Clone(c.path), c.target)) != nil {
		reason = "Both " + c.old + " and " + c.target + " are declared; choose which value to retain"
	}
	return reason
}

// normalizeNaming permits only the exact proposed field replacement. Runtime equivalence is
// checked separately using NamingReporter before any candidate can be written.
func normalizeNaming(node *yaml.Node, e *edit) error {
	old, modern := mappingValue(node, e.old), mappingValue(node, e.target)
	if e.old == e.target {
		return normalizeNamingValue(old, e)
	}
	if (old == nil) == (modern == nil) {
		return ErrVerification
	}
	if old != nil {
		if old.Value != e.from {
			return ErrVerification
		}
		return nil
	}
	if modern.Value != e.value {
		return ErrVerification
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == e.target {
			node.Content[i].Value = e.old
			node.Content[i+1].Value = e.from
		}
	}
	return nil
}

func normalizeNamingValue(node *yaml.Node, e *edit) error {
	if node == nil || (node.Value != e.from && node.Value != e.value) {
		return ErrVerification
	}
	node.Value = e.from
	return nil
}

func hasNamingMigration(candidates []candidate) bool {
	for _, c := range candidates {
		for _, change := range c.edits {
			if change.kind == ruleLegacyNaming {
				return true
			}
		}
	}
	return false
}

func (e *Engine) namingSnapshot(ctx context.Context, o *Options, overlay map[string][]byte) (map[naming.Identity]naming.Result, error) {
	reporter, ok := e.Stacks.(NamingReporter)
	if !ok {
		return nil, fmt.Errorf("%w: no production naming reporter available", ErrVerification)
	}
	engine := *e
	engine.FS = sourceOverlay{FileSystem: e.FS, files: overlay}
	options := o.rediscoveryOptions()
	options.Fix = false
	options.Affected = false
	p, err := engine.discover(ctx, options, &Report{})
	if err != nil {
		return nil, err
	}
	if !engine.canCheckStacks(p) {
		return nil, fmt.Errorf("%w: %s", ErrVerification, engine.stackBlocker(p))
	}
	config, err := staticConfig(p)
	if err != nil {
		return nil, err
	}
	inputs, err := engine.staticInputs(p, config)
	if err != nil {
		return nil, err
	}
	result, err := reporter.NamingResults(ctx, config, inputs)
	if err == nil && len(result) == 0 {
		return nil, fmt.Errorf("%w: no affected component instances to compare", ErrVerification)
	}
	return result, err
}

// verifyNaming compares results keyed by authored identity, so a name change cannot disappear
// behind a changed map key. All imports and inherited consumers are included, even with --affected.
func (e *Engine) verifyNaming(ctx context.Context, o *Options, candidates []candidate) error {
	if !hasNamingMigration(candidates) {
		return nil
	}
	before, err := e.namingSnapshot(ctx, o, nil)
	if err != nil {
		return err
	}
	overlay := map[string][]byte{}
	for _, c := range candidates {
		overlay[c.doc.path] = c.content
	}
	after, err := e.namingSnapshot(ctx, o, overlay)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("%w: resolved stack, workspace, or cluster names change; review the proposed template manually", ErrVerification)
	}
	return nil
}

// assessNaming verifies recommendations even in diagnostic-only runs. An incomplete naming
// migration never blocks unrelated repairs, and is never advertised as automatically fixable.
func (e *Engine) assessNaming(ctx context.Context, o *Options, r *Report, edits []edit) ([]edit, error) {
	// Verify coordinated replacements together first. A stack-pattern migration can change
	// legacy workspace context, while migrating both fields preserves the final identities.
	combined, stageErr := stageNaming(edits)
	combinedSafe := stageErr == nil && len(combined) > 0 && e.verifyNaming(ctx, o, combined) == nil
	var ready []edit
	for _, change := range edits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if change.kind != ruleLegacyNaming {
			ready = append(ready, change)
			continue
		}
		var err error
		if !combinedSafe {
			var content []byte
			content, err = applyEdit(change.doc.content, &change)
			if err == nil {
				err = e.verifyNaming(ctx, o, []candidate{{doc: change.doc, content: content, edits: []edit{change}}})
			}
		}
		f := &r.Findings[change.finding]
		if err != nil {
			f.Fixable = false
			f.Status = statusVerificationIncomplete
			f.Reason = err.Error()
			if strings.Contains(err.Error(), "names change") {
				f.Status = statusVerificationFailed
			}
			continue
		}
		f.Reason = "Resolved stack, workspace, and cluster names are identical before and after"
		f.Remediation = change.manualRemediation() + "; or run atmos doctor --fix"
		ready = append(ready, change)
	}
	return ready, nil
}

func stageNaming(edits []edit) ([]candidate, error) {
	var candidates []candidate
	byPath := map[string]int{}
	for _, change := range edits {
		if change.kind != ruleLegacyNaming {
			continue
		}
		index, exists := byPath[change.doc.path]
		if !exists {
			index = len(candidates)
			byPath[change.doc.path] = index
			candidates = append(candidates, candidate{doc: change.doc, content: change.doc.content})
		}
		c := &candidates[index]
		var err error
		c.content, err = applyEdit(c.content, &change)
		if err != nil {
			return nil, err
		}
		c.edits = append(c.edits, change)
	}
	return candidates, nil
}
