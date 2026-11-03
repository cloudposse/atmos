package templatesource

import (
	"errors"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Candidate syntax covers Go key expressions, key properties, and aliases used as keys.
// Parsed YAML still determines ownership; data sections and logical names remain supported.
var templatedMappingKeySyntax = regexp.MustCompile(`(?m)(^|[,{])[ \t]*(?:(?:!<[^>\n]+>|![^ \t\n,{}\[\]]+|&[^ \t\n,{}\[\]]+)[ \t]+)*(?:\*[^ \t\n,:{}\[\]]+[ \t]*:|[^:\n]*\{\{)`)

// hasTemplatedMappingKey confirms a key colon outside Go actions and YAML quotes.
// The cheap candidate check keeps ordinary value templates out of the scanner.
func hasTemplatedMappingKey(input string) bool {
	if !templatedMappingKeySyntax.MatchString(input) {
		return false
	}
	scanner := mappingKeyScanner{beforeValue: true, separated: true}
	for index := 0; index < len(input); {
		consumed, found := scanner.advance(input[index:])
		if found {
			return true
		}
		scanner.separated = strings.ContainsRune(" \t\r\n", rune(input[index+consumed-1]))
		index += consumed
	}
	return false
}

type mappingKeyScanner struct {
	beforeValue bool
	templated   bool
	quote       byte
	separated   bool
}

func (scanner *mappingKeyScanner) advance(input string) (int, bool) {
	if strings.HasPrefix(input, templateOpening) {
		if action := wholeValueAction.FindStringIndex(input); action != nil {
			scanner.templated = scanner.templated || scanner.beforeValue
			return action[1], false
		}
	}
	if scanner.quote != 0 {
		return scanner.advanceQuoted(input), false
	}
	if input[0] == '#' && scanner.separated {
		end := strings.IndexByte(input, '\n')
		if end < 0 {
			end = len(input)
		}
		return end, false
	}
	// URI tag colons describe a key property, not the mapping's value boundary.
	if strings.HasPrefix(input, "!<") {
		if end := strings.IndexByte(input, '>'); end >= 0 {
			return end + 1, false
		}
	}
	return 1, scanner.advanceUnquoted(input[0])
}

func (scanner *mappingKeyScanner) advanceQuoted(input string) int {
	if input[0] == scanner.quote {
		if scanner.quote == '\'' && len(input) > 1 && input[1] == '\'' {
			return 2
		}
		scanner.quote = 0
	} else if scanner.quote == '"' && input[0] == '\\' && len(input) > 1 {
		return 2
	}
	return 1
}

func (scanner *mappingKeyScanner) advanceUnquoted(value byte) bool {
	switch value {
	case ':':
		if scanner.beforeValue && scanner.templated {
			return true
		}
		scanner.beforeValue = false
	case '\n', '{', '[', ',':
		scanner.beforeValue, scanner.templated = true, false
	case '\'', '"':
		scanner.quote = value
	case '*':
		scanner.templated = scanner.templated || scanner.beforeValue
	}
	return false
}

func templatedYAMLKey(source *sourceIndex, key *yaml.Node) bool {
	return key.Kind != yaml.ScalarNode || strings.Contains(key.Value, templateOpening) || maskedScalarTemplate(source, key)
}

type authProbeState struct {
	node      *yaml.Node
	remaining int
}

type authShapeProbe struct {
	source   *sourceIndex
	visiting map[authProbeState]bool
	results  map[authProbeState]bool
}

func (probe *authShapeProbe) couldBeAuthMapping(node *yaml.Node) bool {
	if probe.results == nil {
		probe.visiting = make(map[authProbeState]bool)
		probe.results = make(map[authProbeState]bool)
	}
	return probe.path(node, []string{"identities", "", "spec", "env"})
}

// path probes only fixed credential shape; an empty segment is a logical name.
// Completed node/path results bound shared merge DAG work; cycles fail closed.
func (probe *authShapeProbe) path(node *yaml.Node, keys []string) (matched bool) {
	if node == nil {
		return false
	}
	if len(keys) == 0 {
		return true
	}
	state := authProbeState{node: node, remaining: len(keys)}
	if result, found := probe.results[state]; found {
		return result
	}
	if probe.visiting[state] {
		return true
	}
	probe.visiting[state] = true
	defer func() {
		delete(probe.visiting, state)
		probe.results[state] = matched
	}()

	switch node.Kind {
	case yaml.AliasNode:
		return probe.path(node.Alias, keys)
	case yaml.SequenceNode:
		return probe.sequence(node, keys)
	case yaml.MappingNode:
		return probe.entries(node, keys)
	}
	return false
}

func (probe *authShapeProbe) sequence(node *yaml.Node, keys []string) bool {
	for _, child := range node.Content {
		if probe.path(child, keys) {
			return true
		}
	}
	return false
}

func (probe *authShapeProbe) entries(node *yaml.Node, keys []string) bool {
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if key.Tag == "!!merge" {
			if probe.path(value, keys) {
				return true
			}
			continue
		}
		if keys[0] != "" && key.Value != keys[0] && !templatedYAMLKey(probe.source, key) {
			continue
		}
		if probe.path(value, keys[1:]) {
			return true
		}
	}
	return false
}

// An unquoted whole-value action also parses as a mapping. Its next byte must
// be a value boundary; a colon or key suffix still denotes a templated key.
const (
	goActionPattern = `\{\{(?:[^"'\x60]|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\x60[^\x60]*\x60)*?\}\}`
	projectedAction = "ATMOS_TEMPLATE_ACTION"
)

var (
	wholeValueAction = regexp.MustCompile("^" + goActionPattern)
	templateAction   = regexp.MustCompile(goActionPattern)
)

func generatedMappingValue(source *sourceIndex, node *yaml.Node) bool {
	start := scalarStart(source, node)
	if start < 0 || !strings.HasPrefix(source.text[start:], templateOpening) {
		return false
	}
	action := wholeValueAction.FindStringIndex(source.text[start:])
	if action == nil {
		return false
	}
	rest := strings.TrimLeft(source.text[start+action[1]:], indentWhitespace)
	return rest == "" || strings.ContainsRune("\r\n,}]", rune(rest[0]))
}

func templatedAuthKeyError() error {
	return errUtils.Build(errUtils.ErrInvalidAuthConfig).
		WithExplanation("Auth structural keys must be literal YAML keys before general template rendering.").
		WithHint("Keep auth, identities, spec, and env keys literal; templates may select component and identity names or credential string values.").Err()
}

// rejectMalformedAuthSource inspects an inert action projection only on decode
// failure. Dynamic application keys and non-output actions can precede ordinary
// YAML keys; parsed ownership takes precedence over raw credential-key evidence.
func rejectMalformedAuthSource(input string) bool {
	rawAuth := hasRawAuthPathKeys(input)
	if !rawAuth && !templatedMappingKeySyntax.MatchString(input) {
		return false
	}
	projected := templateAction.ReplaceAllStringFunc(input, func(action string) string {
		if controlLine.MatchString(action) {
			return ""
		}
		return projectedAction
	})
	decoder := yaml.NewDecoder(strings.NewReader(projected))
	source := newSourceIndex(projected)
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return rawAuth || hasTemplatedMappingKey(input)
		}
		markProjectedKeys(&document)
		if visitAuthMappings(source, &document, func(auth *yaml.Node) error {
			if source.authProbe.couldBeAuthMapping(auth) {
				return templatedAuthKeyError()
			}
			return nil
		}) != nil {
			return true
		}
	}
}

func markProjectedKeys(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			if strings.Contains(node.Content[index].Value, projectedAction) {
				node.Content[index].Value = templateOpening
			}
		}
	}
	for _, child := range node.Content {
		markProjectedKeys(child)
	}
}
