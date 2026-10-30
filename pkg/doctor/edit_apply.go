package doctor

import (
	"slices"
	"strconv"
	"strings"

	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func expressionPath(path []string) string {
	var b strings.Builder
	for _, part := range path {
		if _, err := strconv.Atoi(part); err == nil {
			b.WriteString("[" + part + "]")
		} else {
			b.WriteString("[" + strconv.Quote(part) + "]")
		}
	}
	return "." + b.String()
}

// applyEdit stages one repair as a yq expression, preserving the source style. The legacy
// key's comments move onto its replacement as part of the same expression, so nothing the
// author wrote about the setting is lost.
func applyEdit(content []byte, e *edit) ([]byte, error) {
	p := expressionPath(e.path)
	switch e.kind {
	case ruleLegacyNaming:
		old := p + "[" + strconv.Quote(e.old) + "]"
		if e.old == e.target {
			return atmosyaml.Stage(e.doc.path, content, old+" = "+strconv.Quote(e.value))
		}
		return atmosyaml.Stage(e.doc.path, content, moveKeyExpression(old, p+"["+strconv.Quote(e.target)+"]", strconv.Quote(e.value)))
	case ruleCloudFormationRegion:
		old := expressionPath(append(slices.Clone(e.path), "settings", "aws_cloudformation", "region"))
		replacement := expressionPath(append(slices.Clone(e.path), "region"))
		return atmosyaml.Stage(e.doc.path, content, moveKeyExpression(old, replacement, old))
	case ruleCloudFormationTimeout:
		old := expressionPath(append(slices.Clone(e.path), "timeout_in_minutes"))
		replacement := expressionPath(append(slices.Clone(e.path), "timeout"))
		return atmosyaml.Stage(e.doc.path, content, moveKeyExpression(old, replacement, strconv.Quote(e.value)))
	case ruleCommandProvides:
		// Renaming the key node in place keeps every comment attached to it.
		return atmosyaml.Stage(e.doc.path, content, "("+p+"["+strconv.Quote(e.old)+"] | key) = \"provides\"")
	case ruleHookEvent, ruleComponentMonorepo:
		return atmosyaml.Stage(e.doc.path, content, p+" = "+strconv.Quote(e.value))
	case ruleComponentDependencies:
		return atmosyaml.Stage(e.doc.path, content, moveKeyExpression(p+".settings.depends_on", p+".dependencies.components", p+".settings.depends_on"))
	case ruleTerraformReconfigure:
		return atmosyaml.Stage(e.doc.path, content, moveKeyExpression(p+".init_run_reconfigure", p+".init.reconfigure", strconv.Quote(e.value)))
	default:
		return nil, ErrVerification
	}
}

// commentSlot is one comment of a mapping entry: a head, line, or foot comment on its key or value.
type commentSlot struct {
	variable string
	comment  string
	key      bool
}

var commentSlots = []commentSlot{
	{"keyHead", "head_comment", true},
	{"keyLine", "line_comment", true},
	{"keyFoot", "foot_comment", true},
	{"valueHead", "head_comment", false},
	{"valueLine", "line_comment", false},
	{"valueFoot", "foot_comment", false},
}

// selector narrows an entry path to the key or the value node of the entry.
func (s commentSlot) selector(entry string) string {
	if s.key {
		return group(entry + " | key")
	}
	return entry
}

// moveKeyExpression builds a yq expression that sets replacement to value, carries the comments
// of the old key and value onto the replacement key and value, and then deletes the old key.
func moveKeyExpression(old, replacement, value string) string {
	steps := make([]string, 0, 2*len(commentSlots)+2)
	for _, slot := range commentSlots {
		steps = append(steps, group(slot.selector(old)+" | "+slot.comment)+" as $"+slot.variable)
	}
	steps = append(steps, replacement+" = "+value)
	for _, slot := range commentSlots {
		steps = append(steps, slot.selector(replacement)+" "+slot.comment+" = $"+slot.variable)
	}
	steps = append(steps, "del("+old+")")
	return strings.Join(steps, " | ")
}

// group parenthesizes a yq sub-expression.
func group(expression string) string {
	return "(" + expression + ")"
}
