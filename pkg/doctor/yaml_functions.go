package doctor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agnivade/levenshtein"
	"gopkg.in/yaml.v3"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/validation"
)

// scanYAMLFunctions checks authored syntax against the runtime's canonical catalog.
// It never evaluates functions, templates, arguments, credentials, or remote state.
func scanYAMLFunctions(d *document, o *Options, r *Report) {
	if !enabled(o, ruleYAMLFunctions, categoryValidation) {
		return
	}
	switch d.kind {
	case documentConfig, documentStack, documentVendor:
		scanFunctionNode(d, d.node, nil, r)
	}
}

func scanFunctionNode(d *document, n *yaml.Node, path []string, r *Report) {
	if n == nil || n.Tag == fntag.ToYAML(fntag.Literal) {
		return
	}
	name := authoredFunctionName(n)
	if name != "" && !fntag.IsValidYAML(name) {
		help := "Use a supported Atmos YAML function name"
		if suggestion := suggestYAMLFunction(name); suggestion != "" {
			help = fmt.Sprintf("Did you mean %s?", suggestion)
		}
		r.Findings = append(r.Findings, Finding{
			RuleID: ruleYAMLFunctions, Category: categoryValidation, Severity: validation.SeverityError,
			File: d.path, Path: displayNodePath(path), Line: n.Line, Column: n.Column,
			Message: fmt.Sprintf("Unknown YAML function %s", name), Remediation: help, Status: statusManual,
		})
	}
	// Visit authored nodes only. Alias targets are checked at their declaration,
	// avoiding duplicate findings and recursion through cyclic aliases.
	for i, child := range n.Content {
		childPath := path
		switch n.Kind {
		case yaml.MappingNode:
			childPath = append(append([]string{}, path...), n.Content[i-i%2].Value)
		case yaml.SequenceNode:
			childPath = append(append([]string{}, path...), strconv.Itoa(i))
		}
		scanFunctionNode(d, child, childPath, r)
	}
}

func authoredFunctionName(n *yaml.Node) string {
	if strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!") {
		return n.Tag
	}
	// Native explicit tags (including verbatim tag:yaml.org,2002:* forms,
	// normalized by yaml.v3) and application URI tags retain YAML semantics.
	// In particular, !!str explicitly declares data, not a function invocation.
	if n.Style&yaml.TaggedStyle != 0 {
		return ""
	}
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" || !strings.HasPrefix(n.Value, "!") {
		return ""
	}
	// Quoted functions are strings in the YAML tree. Restrict this check to
	// Atmos's dotted namespaces so ordinary strings like "!important" remain data.
	name := strings.Fields(n.Value)[0]
	namespace, _, dotted := strings.Cut(name, dot)
	if dotted {
		for _, known := range fntag.AllYAML() {
			if strings.HasPrefix(known, namespace+dot) {
				return name
			}
		}
	}
	return ""
}

// Suggest only a unique close match; ambiguous names need the user's judgment.
func suggestYAMLFunction(name string) string {
	const maxDistance = 2
	best, distance, tied := "", maxDistance+1, false
	for _, known := range fntag.AllYAML() {
		d := levenshtein.ComputeDistance(name, known)
		if d < distance {
			best, distance, tied = known, d, false
		} else if d == distance {
			tied = true
		}
	}
	if distance > maxDistance || tied {
		return ""
	}
	return best
}
