package doctor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agnivade/levenshtein"
	"gopkg.in/yaml.v3"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/validation"
)

// scanYAMLFunctions checks authored syntax against the runtime's canonical catalog.
// It never evaluates functions, templates, arguments, credentials, or remote state.
func scanYAMLFunctions(d *document, o *Options, r *Report) {
	if !enabled(o, ruleYAMLFunctions, categoryValidation) {
		return
	}
	switch d.kind {
	case documentConfig, documentStack, documentVendor, documentAtmosWorkflow, documentScaffold:
		scanFunctionNode(d, d.node, nil, r)
	}
}

type functionVisit struct {
	node    *yaml.Node
	context fntag.Context
}

func scanFunctionNode(d *document, n *yaml.Node, path []string, r *Report) {
	scanner := functionScanner{doc: d, report: r, seen: make(map[functionVisit]bool)}
	scanner.walk(n, path, nil, false)
}

// Aliases reuse a declaration in the same context without duplicate findings.
// When an alias crosses a consumer boundary, check that use at the alias line.
// The node/context guard also terminates cyclic aliases.
type functionScanner struct {
	doc    *document
	report *Report
	seen   map[functionVisit]bool
}

func (scanner *functionScanner) walk(n *yaml.Node, path []string, alias *yaml.Node, key bool) {
	if n == nil {
		return
	}
	context := functionContext(scanner.doc, path)
	if key {
		context = fntag.MappingKeyContext
	}
	visit := functionVisit{n, context}
	if scanner.seen[visit] {
		return
	}
	scanner.seen[visit] = true
	if n.Kind == yaml.AliasNode {
		scanner.walk(n.Alias, path, n, key)
		return
	}
	if n.Tag == fntag.ToYAML(fntag.Literal) && fntag.Scope(fntag.Literal, context).NativeTag {
		return
	}
	if issue := functionIssue(context, n); issue != nil {
		scanner.reportIssue(n, path, alias, issue)
	}
	scanner.walkChildren(n, path, alias)
}

func (scanner *functionScanner) reportIssue(n *yaml.Node, path []string, alias *yaml.Node, issue *functionProblem) {
	position := n
	if alias != nil {
		position = alias
	}
	status, reason := statusManual, ""
	if issue.incomplete {
		status, reason = statusVerificationIncomplete, issue.message
	}
	scanner.report.Findings = append(scanner.report.Findings, Finding{
		RuleID: ruleYAMLFunctions, Category: categoryValidation, Severity: issue.severity,
		File: scanner.doc.path, Path: displayNodePath(path), Line: position.Line, Column: position.Column,
		Message: issue.message, Remediation: issue.remediation, Status: status, Reason: reason,
	})
}

func (scanner *functionScanner) walkChildren(n *yaml.Node, path []string, alias *yaml.Node) {
	for i, child := range n.Content {
		childPath := path
		switch n.Kind {
		case yaml.MappingNode:
			childPath = append(append([]string{}, path...), n.Content[i-i%2].Value)
		case yaml.SequenceNode:
			childPath = append(append([]string{}, path...), strconv.Itoa(i))
		}
		scanner.walk(child, childPath, alias, n.Kind == yaml.MappingNode && i%2 == 0)
	}
}

// functionProblem describes one YAML function problem found on a node.
type functionProblem struct {
	severity    validation.Severity
	message     string
	remediation string
	incomplete  bool
}

// functionIssue checks syntax against the consumer at this exact authored path.
func functionIssue(context fntag.Context, n *yaml.Node) *functionProblem {
	name, tagged := authoredFunctionName(n, context == fntag.ConfigContext || context == fntag.SkillRefContext || context == fntag.StackCopiedContext)
	if name == "" || utils.IsForeignYAMLTag(name) {
		return nil
	}
	if !fntag.IsValidYAML(name) {
		return unknownFunctionIssue(name, tagged)
	}
	support := fntag.Scope(name, context)
	if tagged {
		if !support.NativeTag {
			return unsupportedFunctionIssue(name, context)
		}
		if functionNodeShape(n)&support.Shapes == 0 {
			return &functionProblem{severity: validation.SeverityError, message: fmt.Sprintf("%s is not supported on this YAML node type in %s", name, context), remediation: "Use the scalar or collection form documented for this function"}
		}
	} else if !support.StringForm {
		return unsupportedFunctionIssue(name, context)
	}
	return nil
}

func unknownFunctionIssue(name string, tagged bool) *functionProblem {
	if !tagged {
		return &functionProblem{severity: validation.SeverityWarning, message: fmt.Sprintf("%s is kept as text at runtime", name), remediation: functionHelp(name, "If this is literal text, no change is needed; otherwise use a supported Atmos YAML function name")}
	}
	return &functionProblem{severity: validation.SeverityError, message: "Unknown YAML function " + name, remediation: functionHelp(name, "Use a supported Atmos YAML function name")}
}

func unsupportedFunctionIssue(name string, context fntag.Context) *functionProblem {
	if context == fntag.StackUnknownContext {
		return &functionProblem{severity: validation.SeverityWarning, message: fmt.Sprintf("YAML function scope verification incomplete: %s in %s", name, context), remediation: "Confirm that this field reaches a component evaluator; unknown or extension fields may be copied or discarded without evaluation", incomplete: true}
	}

	if context == fntag.ConfigContext || context == fntag.SkillRefContext {
		return &functionProblem{severity: validation.SeverityError, message: fmt.Sprintf("%s is not supported in atmos.yaml; supported: %s", name, strings.Join(fntag.AtmosConfigYAML(), ", ")), remediation: "Use a function atmos.yaml supports, or move this value into a stack manifest"}
	}
	return &functionProblem{severity: validation.SeverityError, message: fmt.Sprintf("%s is not supported in %s", name, context), remediation: "Use a function supported in this field, supply a literal value, or resolve it in a stack value before consuming it here"}
}

func functionNodeShape(n *yaml.Node) fntag.Shape {
	switch n.Kind {
	case yaml.ScalarNode:
		return fntag.Scalar
	case yaml.SequenceNode:
		return fntag.Sequence
	case yaml.MappingNode:
		return fntag.Mapping
	}
	return 0
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
	for _, name := range fntag.StringFormYAML() {
		if fntag.MatchesTag(n.Value, name) {
			return name, false
		}
	}
	if fntag.MatchesTag(n.Value, "!cel") {
		return "!cel", false
	}
	name := quotedFunctionName(n.Value)
	if fntag.IsValidYAML(name) {
		return "", false
	}
	return name, false
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

// addFunctionLimit preserves the report schema while making incomplete scope
// coverage visible even in a run selecting only yaml-functions.
func addFunctionLimit(o *Options, r *Report, file, reason string) {
	addFinding(o, r, &Finding{
		RuleID: ruleYAMLFunctions, Category: categoryValidation,
		Severity: validation.SeverityWarning, File: file, Message: "YAML function scope verification incomplete",
		Reason: reason, Status: statusVerificationIncomplete,
		Remediation: "Review the unresolved context manually; doctor never evaluates functions to discover inputs",
	})
}
