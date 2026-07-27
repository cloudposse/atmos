package tags

import (
	"sort"
	"strings"
	"text/template/parse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultLeftDelim and DefaultRightDelim are the standard Go template
// delimiters, used when 'templates.settings.delimiters' is not configured.
const (
	DefaultLeftDelim  = "{{"
	DefaultRightDelim = "}}"
)

// forbiddenSelectorFunctions lists Atmos YAML functions that may not appear in
// metadata.tags/metadata.labels values. Selectors drive scoping decisions
// before evaluation, so by design they must be resolvable without
// authentication, process execution, or nondeterminism. Values mirror the
// AtmosYamlFunc* constants in pkg/utils/yaml_utils.go (kept as literals so this
// package stays dependency-light; TestForbiddenSelectorFunctionsMatchConstants
// cross-checks them).
var forbiddenSelectorFunctions = map[string]struct{}{
	"!exec":                        {},
	"!secret":                      {},
	"!store":                       {},
	"!store.get":                   {},
	"!terraform.output":            {},
	"!terraform.state":             {},
	"!aws.account_id":              {},
	"!aws.caller_identity_arn":     {},
	"!aws.caller_identity_user_id": {},
	"!aws.region":                  {},
	"!aws.organization_id":         {},
	"!emulator":                    {},
	"!random":                      {},
}

// forbiddenTemplateIdentifiers are template function identifiers that reach
// external data sources and therefore may not appear in selector templates.
var forbiddenTemplateIdentifiers = map[string]struct{}{
	"ds":         {},
	"datasource": {},
}

// forbiddenAtmosMethods are methods on the "atmos" template namespace that
// evaluate other components or reach stores, both of which require the full
// evaluation pipeline selectors exist to avoid.
var forbiddenAtmosMethods = map[string]struct{}{
	"Component":          {},
	"Store":              {},
	"GomplateDatasource": {},
}

// TemplateDelims returns the effective template delimiters from a raw
// 'templates.settings.delimiters' config value, falling back to the standard
// Go delimiters when the setting is absent or malformed (template processing
// itself reports malformed delimiters; selector validation only needs a
// best-effort parse).
func TemplateDelims(delims []string) (string, string) {
	defer perf.Track(nil, "tags.TemplateDelims")()

	if len(delims) == 2 && delims[0] != "" && delims[1] != "" {
		return delims[0], delims[1]
	}
	return DefaultLeftDelim, DefaultRightDelim
}

// SelectorUnresolved reports whether a metadata.tags/metadata.labels value
// still contains an unresolved marker, meaning its real value can only be
// known after template/YAML-function evaluation:
//   - a Go template marker (the configured left delimiter, "{{" by default), or
//   - an unprocessed Atmos YAML function (custom-tagged scalars are stored as
//     plain strings like "!env DEPLOY_TIER" until function processing runs —
//     see getValueWithTag in pkg/utils).
//
// Callers use this to treat such selectors as undecidable and fall through to
// full evaluation instead of silently misjudging them as out-of-scope. A
// quoted literal that merely looks like a marker is conservatively treated as
// unresolved too — the safe direction, since it only forces evaluation.
func SelectorUnresolved(v any, leftDelim string) bool {
	defer perf.Track(nil, "tags.SelectorUnresolved")()

	if leftDelim == "" {
		leftDelim = DefaultLeftDelim
	}
	return selectorUnresolved(v, leftDelim)
}

// selectorUnresolved walks nested slices/maps without re-entering perf tracking.
func selectorUnresolved(v any, leftDelim string) bool {
	switch value := v.(type) {
	case string:
		return strings.Contains(value, leftDelim) || strings.HasPrefix(strings.TrimSpace(value), "!")
	case []any:
		for _, item := range value {
			if selectorUnresolved(item, leftDelim) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if selectorUnresolved(item, leftDelim) {
				return true
			}
		}
	}
	return false
}

// ValidateSelectorValue enforces the selector purity contract on a
// metadata.tags or metadata.labels value: selectors are evaluated before
// authentication, templating, and YAML-function processing so Atmos can decide
// which components are in scope without evaluating out-of-scope ones. By
// design their values are limited to plain strings, simple Go templates over
// the stack context, and local functions such as !env, !git.*, and !include.
// Anything that requires authentication or process execution (for example
// !terraform.state, !store, !exec, or atmos.Component) is rejected with an
// error, not silently deferred. The field parameter names the manifest
// location (e.g. "metadata.tags") for error reporting; leftDelim/rightDelim
// are the configured template delimiters (empty strings select the defaults).
func ValidateSelectorValue(field string, v any, leftDelim, rightDelim string) error {
	defer perf.Track(nil, "tags.ValidateSelectorValue")()

	if leftDelim == "" || rightDelim == "" {
		leftDelim, rightDelim = DefaultLeftDelim, DefaultRightDelim
	}
	return validateSelectorValue(field, v, leftDelim, rightDelim)
}

// validateSelectorValue walks nested slices/maps without re-entering perf
// tracking, visiting map keys in sorted order for deterministic errors.
func validateSelectorValue(field string, v any, leftDelim, rightDelim string) error {
	switch value := v.(type) {
	case string:
		return validateSelectorString(field, value, leftDelim, rightDelim)
	case []any:
		for _, item := range value {
			if err := validateSelectorValue(field, item, leftDelim, rightDelim); err != nil {
				return err
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validateSelectorValue(field, value[key], leftDelim, rightDelim); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSelectorString checks one scalar selector value for forbidden YAML
// functions and forbidden template calls.
func validateSelectorString(field, value, leftDelim, rightDelim string) error {
	if fn := forbiddenSelectorFunction(value); fn != "" {
		return buildForbiddenSelectorError(field, fn)
	}
	if call := forbiddenTemplateCall(value, leftDelim, rightDelim); call != "" {
		return buildForbiddenSelectorError(field, call)
	}
	return nil
}

// forbiddenSelectorFunction returns the offending YAML function name when the
// value is an unprocessed custom-tagged scalar whose function requires
// authentication or execution, or "" when the value is allowed.
func forbiddenSelectorFunction(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "!") {
		return ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	if _, forbidden := forbiddenSelectorFunctions[fields[0]]; forbidden {
		return fields[0]
	}
	return ""
}

// forbiddenTemplateCall parses the value as a Go template (honoring custom
// delimiters) and returns the first forbidden call found in its parse tree, or
// "" when none. A value that does not parse as a template is not this
// validator's concern — template processing reports its own errors later.
func forbiddenTemplateCall(value, leftDelim, rightDelim string) string {
	if !strings.Contains(value, leftDelim) {
		return ""
	}

	tree := parse.New("selector")
	tree.Mode = parse.SkipFuncCheck
	treeSet := make(map[string]*parse.Tree)
	if _, err := tree.Parse(value, leftDelim, rightDelim, treeSet); err != nil {
		return ""
	}

	names := make([]string, 0, len(treeSet))
	for name := range treeSet {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if treeSet[name] == nil || treeSet[name].Root == nil {
			continue
		}
		if call := forbiddenCallInNode(treeSet[name].Root); call != "" {
			return call
		}
	}
	return ""
}

// forbiddenCallInNode recursively walks a template parse tree looking for
// forbidden identifiers ("ds", "datasource") and forbidden methods on the
// atmos namespace (atmos.Component, atmos.Store, atmos.GomplateDatasource).
// Container nodes are traversed here; leaf and control nodes are handled by
// forbiddenCallInLeafOrControl.
func forbiddenCallInNode(node parse.Node) string {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return ""
		}
		return forbiddenCallInNodes(n.Nodes)
	case *parse.ActionNode:
		return forbiddenCallInNode(n.Pipe)
	case *parse.PipeNode:
		if n == nil {
			return ""
		}
		return forbiddenCallInCommands(n.Cmds)
	case *parse.CommandNode:
		return forbiddenCallInNodes(n.Args)
	default:
		return forbiddenCallInLeafOrControl(node)
	}
}

// forbiddenCallInLeafOrControl checks leaf nodes (identifiers, chains) and
// control structures (if/range/with/template) for forbidden calls.
func forbiddenCallInLeafOrControl(node parse.Node) string {
	switch n := node.(type) {
	case *parse.ChainNode:
		if call := forbiddenAtmosChain(n); call != "" {
			return call
		}
		return forbiddenCallInNode(n.Node)
	case *parse.IdentifierNode:
		if _, forbidden := forbiddenTemplateIdentifiers[n.Ident]; forbidden {
			return n.Ident
		}
	case *parse.IfNode:
		return forbiddenCallInBranch(&n.BranchNode)
	case *parse.RangeNode:
		return forbiddenCallInBranch(&n.BranchNode)
	case *parse.WithNode:
		return forbiddenCallInBranch(&n.BranchNode)
	case *parse.TemplateNode:
		return forbiddenCallInNode(n.Pipe)
	}
	return ""
}

// forbiddenCallInNodes checks a slice of parse nodes.
func forbiddenCallInNodes(nodes []parse.Node) string {
	for _, item := range nodes {
		if call := forbiddenCallInNode(item); call != "" {
			return call
		}
	}
	return ""
}

// forbiddenCallInCommands checks each command of a pipe node.
func forbiddenCallInCommands(cmds []*parse.CommandNode) string {
	for _, cmd := range cmds {
		if call := forbiddenCallInNode(cmd); call != "" {
			return call
		}
	}
	return ""
}

// forbiddenCallInBranch checks the pipe and both branches of if/range/with nodes.
func forbiddenCallInBranch(branch *parse.BranchNode) string {
	if call := forbiddenCallInNode(branch.Pipe); call != "" {
		return call
	}
	if call := forbiddenCallInNode(branch.List); call != "" {
		return call
	}
	if branch.ElseList != nil {
		return forbiddenCallInNode(branch.ElseList)
	}
	return ""
}

// forbiddenAtmosChain reports a forbidden atmos-namespace method call
// (e.g. atmos.Component) expressed as a chain node, or "" when the chain is
// allowed.
func forbiddenAtmosChain(chain *parse.ChainNode) string {
	base, ok := chain.Node.(*parse.IdentifierNode)
	if !ok || base.Ident != "atmos" {
		return ""
	}
	for _, field := range chain.Field {
		if _, forbidden := forbiddenAtmosMethods[field]; forbidden {
			return "atmos." + field
		}
	}
	return ""
}

// buildForbiddenSelectorError constructs the by-design selector purity error.
func buildForbiddenSelectorError(field, construct string) error {
	return errUtils.Build(errUtils.ErrForbiddenSelectorFunction).
		WithCausef("%q is not allowed in %s", construct, field).
		WithExplanationf(
			"The %s selector contains %q, which requires authentication, process execution, or evaluating other components. "+
				"Labels and tags select components before evaluation, so by design their values must be resolvable without any of those.",
			field, construct,
		).
		WithHintf(
			"Move the %q call into vars or settings and keep %s values to plain strings, simple templates over the stack context, "+
				"and local functions such as !env, !git.*, and !include.",
			construct, field,
		).
		WithContext("field", field).
		WithContext("construct", construct).
		Err()
}
