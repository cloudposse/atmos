// Package naming describes naming migrations without executing configuration.
package naming

import (
	"regexp"
	"slices"
	"strings"
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/perf"
	tmpl "github.com/cloudposse/atmos/pkg/template"
)

// Identity keys results by authored identity, never by the name being migrated.
type Identity struct{ Manifest, ComponentType, Component string }

// Result contains the runtime identities a naming migration must preserve.
type Result struct{ Stack, Workspace, Cluster, KubeconfigCluster string }

var tokenPattern = regexp.MustCompile(`\{([a-z][a-z_-]*)\}`)

// Upgrade replaces legacy tokens in template text, preserving quoted literals and actions.
// Unknown or context-dependent tokens are returned for manual review, never guessed.
func Upgrade(value string) (string, []string, []string) {
	defer perf.Track(nil, "naming.Upgrade")()
	tree := parse.New("naming")
	tree.Mode = parse.SkipFuncCheck | parse.ParseComments
	if _, err := tree.Parse(value, "{{", "}}", map[string]*parse.Tree{}); err != nil {
		return value, nil, nil
	}
	upgrade := tokenUpgrade{}
	upgrade.visit(tree.Root)
	if len(upgrade.tokens) == 0 {
		return value, nil, nil
	}
	return tree.Root.String(), upgrade.tokens, upgrade.unsupported
}

type tokenUpgrade struct {
	tokens, unsupported []string
}

func (u *tokenUpgrade) replace(token string) string {
	if !slices.Contains(u.tokens, token) {
		u.tokens = append(u.tokens, token)
	}
	key := strings.Trim(token, "{}")
	var expression string
	switch key {
	case "namespace", "tenant", "environment", "stage", "region":
		expression = ".vars." + key
	case "component":
		expression = ".atmos_component"
	case "base-component":
		expression = ".component"
	case "attributes":
		expression = `join "-" .vars.attributes`
	default:
		if !slices.Contains(u.unsupported, token) {
			u.unsupported = append(u.unsupported, token)
		}
		return token
	}
	return "{{ " + expression + " }}"
}

func (u *tokenUpgrade) visit(list *parse.ListNode) {
	if list == nil {
		return
	}
	for _, node := range list.Nodes {
		switch n := node.(type) {
		case *parse.TextNode:
			n.Text = []byte(tokenPattern.ReplaceAllStringFunc(string(n.Text), u.replace))
		case *parse.IfNode:
			u.visit(n.List)
			u.visit(n.ElseList)
		case *parse.RangeNode:
			u.visit(n.List)
			u.visit(n.ElseList)
		case *parse.WithNode:
			u.visit(n.List)
			u.visit(n.ElseList)
		}
	}
}

// SafeTemplate permits only static field reads and pure formatting functions.
// It is deliberately conservative: evaluation must never access files, environment, or providers.
func SafeTemplate(value string) bool {
	defer perf.Track(nil, "naming.SafeTemplate")()
	refs, ok := tmpl.StaticFieldRefs(value)
	if !ok {
		return false
	}
	for _, ref := range refs {
		if len(ref.Path) == 0 || !slices.Contains([]string{"vars", "settings", "env", "metadata", "atmos_component", "component"}, ref.Path[0]) {
			return false
		}
	}
	funcs, ok := tmpl.StaticFunctionNames(value)
	if !ok {
		return false
	}
	for _, f := range funcs {
		if !slices.Contains([]string{"and", "or", "not", "eq", "ne", "lt", "le", "gt", "ge", "len", "print", "printf", "println", "lower", "upper", "title", "trim", "trimAll", "trimPrefix", "trimSuffix", "replace", "join", "default", "coalesce", "ternary", "empty", "contains", "hasPrefix", "hasSuffix", "trunc", "substr", "quote", "squote", "cat", "list", "dict"}, f) {
			return false
		}
	}
	return true
}

// Field recognizes only Atmos-owned naming fields, not similarly named user data.
func Field(path []string) bool {
	defer perf.Track(nil, "naming.Field")()
	switch len(path) {
	case 2:
		return rootNamingField(path[0], path[1])
	case 3:
		return slices.Equal(path[:2], []string{"components", "helmfile"}) && isNamingField(path[2], "cluster_name")
	default:
		return componentNamingField(path)
	}
}

const componentNamingPathLength = 5

func rootNamingField(section, field string) bool {
	return (section == "stacks" && isNamingField(field, "name")) ||
		(section == "metadata" && isNamingField(field, "terraform_workspace"))
}

func componentNamingField(path []string) bool {
	if len(path) != componentNamingPathLength {
		return false
	}
	return slices.Equal(path[:2], []string{"components", "terraform"}) && path[3] == "metadata" && isNamingField(path[4], "terraform_workspace")
}

func isNamingField(field, prefix string) bool {
	return field == prefix+"_pattern" || field == prefix+"_template"
}
