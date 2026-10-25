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
func scanYAMLFunctions(d *document, o *Options, r *Report, strict ...bool) {
	if !enabled(o, ruleYAMLFunctions, categoryValidation) {
		return
	}
	switch d.kind {
	case documentConfig, documentStack, documentVendor:
		scanFunctionNode(d, d.node, nil, r, strict...)
	}
}

func scanFunctionNode(d *document, n *yaml.Node, path []string, r *Report, strict ...bool) {
	if n == nil || n.Tag == fntag.ToYAML(fntag.Literal) {
		return
	}
	issue := functionIssue(d.kind == documentConfig, n, strict...)
	if issue == nil {
		issue = conditionFunctionIssue(d.kind == documentConfig, n, path, strict...)
	}
	if issue != nil {
		r.Findings = append(r.Findings, Finding{
			RuleID: ruleYAMLFunctions, Category: categoryValidation, Severity: issue.severity,
			File: d.path, Path: displayNodePath(path), Line: n.Line, Column: n.Column,
			Message: issue.message, Remediation: issue.remediation, Status: statusManual,
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
		scanFunctionNode(d, child, childPath, r, strict...)
	}
}

// functionProblem describes one YAML function problem found on a node.
type functionProblem struct {
	severity    validation.Severity
	message     string
	remediation string
}

// functionIssue classifies a node's authored function use. A YAML-tagged node is always a
// function call, so a bad name is an error. An untagged string is a call only where the runtime
// would resolve it, so a look-alike string is only a warning: the runtime keeps it as text.
// Configuration documents (atmos.yaml and friends) accept a smaller set of functions than
// stack manifests and never evaluate untagged strings.
func functionIssue(config bool, n *yaml.Node, strict ...bool) *functionProblem {
	if len(strict) > 0 && strict[0] && (n.Tag == "!!str" || n.Tag == "") {
		return nil
	}
	name, tagged := authoredFunctionName(n, config)
	if name == "" {
		return nil
	}
	if !tagged {
		return legacyFunctionIssue(name)
	}
	if !fntag.IsValidYAML(name) {
		return &functionProblem{
			severity:    validation.SeverityError,
			message:     "Unknown YAML function " + name,
			remediation: functionHelp(name, "Use a supported Atmos YAML function name"),
		}
	}
	if config && !fntag.IsAtmosConfigYAML(name) {
		return &functionProblem{
			severity:    validation.SeverityError,
			message:     fmt.Sprintf("%s is not supported in atmos.yaml; supported: %s", name, strings.Join(fntag.AtmosConfigYAML(), ", ")),
			remediation: "Use a function atmos.yaml supports, or move this value into a stack manifest",
		}
	}
	return nil
}

func legacyFunctionIssue(name string) *functionProblem {
	if fntag.IsValidYAML(name) {
		return nil
	}
	return &functionProblem{
		severity:    validation.SeverityWarning,
		message:     fmt.Sprintf("%s is kept as text at runtime", name),
		remediation: functionHelp(name, "If this is literal text, no change is needed; otherwise use a supported Atmos YAML function name"),
	}
}

func functionHelp(name, fallback string) string {
	if suggestion := suggestYAMLFunction(name); suggestion != "" {
		return fmt.Sprintf("Did you mean %s?", suggestion)
	}
	return fallback
}

// authoredFunctionName returns the function a node names and whether it is a YAML tag.
func authoredFunctionName(n *yaml.Node, config bool) (string, bool) {
	if strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!") {
		return n.Tag, true
	}
	// Native explicit tags (including verbatim tag:yaml.org,2002:* forms,
	// normalized by yaml.v3) and application URI tags retain YAML semantics.
	// In particular, !!str explicitly declares data, not a function invocation.
	if config || n.Style&yaml.TaggedStyle != 0 {
		return "", false
	}
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" || !strings.HasPrefix(n.Value, "!") {
		return "", false
	}
	// A string the runtime resolves as a function (for example "!terraform.state[0]") is one.
	if fntag.MatchesStringForm(n.Value) {
		return "", false
	}
	return quotedFunctionName(n.Value), false
}

// quotedFunctionName returns the first word of a quoted string that looks like a function in one of
// Atmos's dotted namespaces. Quoted functions are strings in the YAML tree, and the check is restricted
// to those namespaces so ordinary strings like "!important" remain data.
func quotedFunctionName(value string) string {
	name := strings.Fields(value)[0]
	namespace, _, dotted := strings.Cut(name, dot)
	if !dotted {
		return ""
	}
	for _, known := range fntag.AllYAML() {
		if strings.HasPrefix(known, namespace+dot) {
			return name
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

// conditionFunctionIssue checks condition syntax without mistaking arbitrary
// component data (including a data key named "when") for a condition field.
func conditionFunctionIssue(config bool, node *yaml.Node, path []string, strict ...bool) *functionProblem {
	if node.Kind != yaml.ScalarNode {
		return nil
	}
	condition := isConditionFunctionPath(config, path)
	if config && node.Tag == "!cel" && !condition {
		return &functionProblem{severity: validation.SeverityError, message: "!cel is supported in condition fields", remediation: "Use !cel in an import or step condition"}
	}
	if !condition || len(strict) == 0 || !strict[0] {
		return nil
	}
	if node.Tag == "!!str" && strings.HasPrefix(strings.TrimSpace(node.Value), "!cel") {
		return &functionProblem{severity: validation.SeverityError, message: "Quoted !cel is literal text, not a condition", remediation: "Use !cel \"expression\" or an unprefixed condition expression"}
	}
	return nil
}

func isConditionFunctionPath(config bool, path []string) bool {
	for i, part := range path {
		if i < 2 || (part != "when" && part != "continue") {
			continue
		}
		if path[i-2] == "steps" || (config && path[i-2] == "import") {
			return true
		}
	}
	return false
}
