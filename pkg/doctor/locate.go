package doctor

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	schemaRootField = "(root)"
	anyOfErrorType  = "number_any_of"
)

// locateField maps a JSON Schema instance path such as "components.terraform.init" to the
// source position of the deepest key it can resolve. Keys may themselves contain dots, so each
// mapping level prefers the longest matching key. An unresolvable tail falls back to its parent.
func locateField(root *yaml.Node, field string) (line, column int) {
	n := rootNode(root)
	if n == nil {
		return 0, 0
	}
	line, column = n.Line, n.Column
	if field == "" || field == schemaRootField {
		return line, column
	}
	parts := strings.Split(field, dot)
	for len(parts) > 0 {
		key, value, used := descend(n, parts)
		if value == nil {
			return line, column
		}
		line, column = key.Line, key.Column
		n, parts = value, parts[used:]
	}
	return line, column
}

// descend resolves the next key of an instance path below n, following aliases.
func descend(n *yaml.Node, parts []string) (key, value *yaml.Node, used int) {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	switch n.Kind {
	case yaml.MappingNode:
		return longestKey(n, parts)
	case yaml.SequenceNode:
		return sequenceItem(n, parts[0])
	}
	return nil, nil, 0
}

// longestKey finds the mapping entry whose key equals the longest dotted prefix of parts.
func longestKey(n *yaml.Node, parts []string) (key, value *yaml.Node, used int) {
	for used = len(parts); used > 0; used-- {
		name := strings.Join(parts[:used], dot)
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == name {
				return n.Content[i], n.Content[i+1], used
			}
		}
	}
	return nil, nil, 0
}

// sequenceItem resolves a numeric index. The item itself positions the finding because it has no key.
func sequenceItem(n *yaml.Node, part string) (key, value *yaml.Node, used int) {
	index, err := strconv.Atoi(part)
	if err != nil || index < 0 || index >= len(n.Content) {
		return nil, nil, 0
	}
	item := n.Content[index]
	return item, item, 1
}

// meaningfulFailures drops anyOf failures reported at an ancestor of a more specific failure,
// which only restate that some nested value did not match any alternative.
func meaningfulFailures(failures []gojsonschema.ResultError) []gojsonschema.ResultError {
	kept := make([]gojsonschema.ResultError, 0, len(failures))
	for _, failure := range failures {
		if failure.Type() == anyOfErrorType && hasDescendantFailure(failures, failure.Field()) {
			continue
		}
		kept = append(kept, failure)
	}
	return kept
}

func hasDescendantFailure(failures []gojsonschema.ResultError, field string) bool {
	for _, other := range failures {
		child := other.Field()
		if field == schemaRootField {
			if child != schemaRootField {
				return true
			}
			continue
		}
		if strings.HasPrefix(child, field+dot) {
			return true
		}
	}
	return false
}

var yamlErrorLine = regexp.MustCompile(`line (\d+)`)

// parseFinding reports a YAML syntax error with the line the parser named, when it named one.
func parseFinding(file string, err error) *Finding {
	f := &Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityError, File: file, Message: err.Error(), Status: statusManual}
	if match := yamlErrorLine.FindStringSubmatch(err.Error()); match != nil {
		f.Line, _ = strconv.Atoi(match[1])
	}
	return f
}
