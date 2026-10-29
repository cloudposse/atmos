// Package templatesource defers identity-owned credential templates until authentication.
package templatesource

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	controlLine = regexp.MustCompile(`^\{\{-?\s*(if|else|end|range|with|define|block|template)\b.*\}\}$`)
	authKey     = regexp.MustCompile(`(?m)(^|[,{])\s*["']?auth["']?\s*:`)
)

type templateSpan struct {
	start, end int
	value      string
}

// Protect defers auth identity spec.env templates until credentials exist.
// General stack rendering must never execute these expressions with its broader function map.
// The restore function preserves the original scalar source after general rendering finishes.
func Protect(input string) (string, func(string) string, error) {
	defer perf.Track(nil, "templatesource.Protect")()

	unchanged := func(value string) string { return value }
	if !strings.Contains(input, "{{") {
		return input, unchanged, nil
	}
	spans, err := documentTemplateSpans(input)
	if err != nil {
		return "", unchanged, err
	}
	if len(spans) == 0 {
		return input, unchanged, nil
	}
	protected, restore := protectSpans(input, spans)
	return protected, restore, nil
}

func documentTemplateSpans(input string) ([]templateSpan, error) {
	decoder := yaml.NewDecoder(strings.NewReader(templateYAML(input)))
	var spans []templateSpan
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return spans, nil
		}
		if err != nil {
			if !authKey.MatchString(input) {
				return nil, nil
			}
			return nil, errUtils.Build(errUtils.ErrInvalidAuthConfig).
				WithExplanation("Auth credential templates cannot be protected in malformed YAML.").
				WithHint("Use valid YAML and write each credential template inline in spec.env as a quoted string or block scalar.").Err()
		}
		if err := visitAuthMappings(&document, func(auth *yaml.Node) error {
			values, err := environmentTemplateSpans(input, auth)
			spans = append(spans, values...)
			return err
		}); err != nil {
			return nil, err
		}
	}
}

func protectSpans(input string, spans []templateSpan) (string, func(string) string) {
	prefix := "ATMOS_AUTH_ENV_TEMPLATE_"
	for strings.Contains(input, prefix) {
		prefix += "X"
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	replacements := make([]string, 0, 2*len(spans))
	for _, span := range spans {
		placeholder := fmt.Sprintf("%s%d_END", prefix, len(replacements))
		replacements = append(replacements, placeholder, span.value)
		input = input[:span.start] + placeholder + input[span.end:]
	}
	return input, strings.NewReplacer(replacements...).Replace
}

func environmentTemplateSpans(input string, auth *yaml.Node) ([]templateSpan, error) {
	identities, aliases, err := mappingPath(auth, "identities")
	if err != nil {
		return nil, err
	}
	if identities == nil || identities.Kind != yaml.MappingNode {
		return nil, nil
	}
	var spans []templateSpan
	for index := 1; index < len(identities.Content); index += 2 {
		env, identityAliases, err := mappingPath(identities.Content[index], "spec", "env")
		if err != nil {
			return nil, err
		}
		values, err := environmentValueSpans(input, env, aliases || identityAliases)
		if err != nil {
			return nil, err
		}
		spans = append(spans, values...)
	}
	return spans, nil
}

func environmentValueSpans(input string, env *yaml.Node, aliased bool) ([]templateSpan, error) {
	if env == nil || env.Kind != yaml.MappingNode {
		return nil, nil
	}
	var spans []templateSpan
	for valueIndex := 1; valueIndex < len(env.Content); valueIndex += 2 {
		value := env.Content[valueIndex]
		if requiresInlineTemplate(value, aliased) {
			return nil, inlineTemplateError()
		}
		if span, ok := environmentValueSpan(input, value); ok {
			spans = append(spans, span)
		}
	}
	return spans, nil
}

func requiresInlineTemplate(value *yaml.Node, aliased bool) bool {
	// An anchor can live outside auth, so hiding only the alias would still
	// let the general renderer execute its template before credentials exist.
	indirect := aliased || value.Kind == yaml.AliasNode || value.Anchor != "" || value.Style&yaml.TaggedStyle != 0
	return indirect && containsTemplate(value, make(map[*yaml.Node]bool))
}

func environmentValueSpan(input string, value *yaml.Node) (templateSpan, bool) {
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
		return templateSpan{}, false
	}
	start, end := scalarSpan(input, value)
	if start < 0 || end < start || !strings.Contains(input[start:end], "{{") {
		return templateSpan{}, false
	}
	return templateSpan{start: start, end: end, value: input[start:end]}, true
}

func inlineTemplateError() error {
	return errUtils.Build(errUtils.ErrInvalidAuthConfig).
		WithExplanation("Auth identity spec.env credential templates require inline strings without YAML anchors, aliases, merge keys, or explicit tags.").
		WithHint("Write each credential template inline in spec.env as a quoted string or block scalar.").Err()
}

func containsTemplate(node *yaml.Node, visited map[*yaml.Node]bool) bool {
	if node == nil || visited[node] {
		return false
	}
	visited[node] = true
	if strings.Contains(node.Value, "{{") || containsTemplate(node.Alias, visited) {
		return true
	}
	for _, child := range node.Content {
		if containsTemplate(child, visited) {
			return true
		}
	}
	return false
}

// templateYAML blanks standalone template controls only for locating YAML scalars.
// The returned source is never rendered, so controls and comments retain their spelling.
func templateYAML(input string) string {
	lines := strings.Split(input, "\n")
	for index, line := range lines {
		if controlLine.MatchString(strings.TrimSpace(line)) {
			lines[index] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func scalarSpan(input string, node *yaml.Node) (int, int) {
	start, lines := scalarStart(input, node)
	if start < 0 {
		return -1, -1
	}
	line := lines[node.Line-1]
	column := []rune(line)
	if node.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
		return start, quotedEnd(input, start)
	}
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	end := len(input)
	nextOffset := start + len(line) - len(string(column[:node.Column-1]))
	for _, next := range lines[node.Line:] {
		if strings.TrimSpace(next) != "" && len(next)-len(strings.TrimLeft(next, " \t")) <= indent {
			end = nextOffset
			break
		}
		nextOffset += len(next)
	}
	// Leave the scalar's final line ending outside the placeholder.
	if end > start && input[end-1] == '\n' {
		end--
	}
	return start, end
}

func scalarStart(input string, node *yaml.Node) (int, []string) {
	lines := strings.SplitAfter(input, "\n")
	if node.Line < 1 || node.Line > len(lines) {
		return -1, nil
	}
	start := 0
	for _, line := range lines[:node.Line-1] {
		start += len(line)
	}
	column := []rune(lines[node.Line-1])
	if node.Column < 1 || node.Column > len(column) {
		return -1, nil
	}
	return start + len(string(column[:node.Column-1])), lines
}

func quotedEnd(input string, start int) int {
	quote := input[start]
	for end := start + 1; end < len(input); end++ {
		if quote == '"' && input[end] == '\\' {
			end++
		} else if input[end] == quote {
			if quote == '\'' && end+1 < len(input) && input[end+1] == quote {
				end++
				continue
			}
			return end + 1
		}
	}
	return -1
}

func mappingPath(node *yaml.Node, keys ...string) (*yaml.Node, bool, error) {
	aliased := false
	for {
		if node != nil && node.Kind == yaml.AliasNode {
			node = node.Alias
			aliased = true
		}
		if node != nil && templatedMerge(node) {
			return nil, aliased, inlineTemplateError()
		}
		if len(keys) == 0 || node == nil {
			return node, aliased, nil
		}
		node = mappingValue(node, keys[0])
		keys = keys[1:]
	}
}

func templatedMerge(node *yaml.Node) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Tag == "!!merge" && containsTemplate(node.Content[index+1], make(map[*yaml.Node]bool)) {
			return true
		}
	}
	return false
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func visitAuthMappings(node *yaml.Node, visit func(*yaml.Node) error) error {
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			if node.Content[index].Value == "auth" {
				if err := visit(node.Content[index+1]); err != nil {
					return err
				}
			}
		}
	}
	for _, child := range node.Content {
		if err := visitAuthMappings(child, visit); err != nil {
			return err
		}
	}
	return nil
}
