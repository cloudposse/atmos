package exec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var authTemplateControlLine = regexp.MustCompile(`^\{\{-?\s*(if|else|end|range|with|define|block|template)\b.*\}\}$`)

type authTemplateSpan struct {
	start, end int
	value      string
}

// protectAuthEnvironmentTemplates defers identity-owned templates until credentials exist.
// General stack rendering must never execute these expressions with its broader function map.
func protectAuthEnvironmentTemplates(input string) (string, func(string) string) {
	unchanged := func(value string) string { return value }
	if !strings.Contains(input, "identities") || !strings.Contains(input, "env") || !strings.Contains(input, "{{") {
		return input, unchanged
	}
	var document yaml.Node
	if yaml.Unmarshal([]byte(authTemplateYAML(input)), &document) != nil {
		return input, unchanged
	}
	prefix := "ATMOS_AUTH_ENV_TEMPLATE_"
	for strings.Contains(input, prefix) {
		prefix += "X"
	}
	var spans []authTemplateSpan
	visitAuthMappings(&document, func(auth *yaml.Node) {
		spans = append(spans, authEnvironmentTemplateSpans(input, auth)...)
	})
	if len(spans) == 0 {
		return input, unchanged
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

func authEnvironmentTemplateSpans(input string, auth *yaml.Node) []authTemplateSpan {
	identities := authMappingValue(auth, "identities")
	if identities == nil || identities.Kind != yaml.MappingNode {
		return nil
	}
	var spans []authTemplateSpan
	for index := 1; index < len(identities.Content); index += 2 {
		env := authMappingValue(authMappingValue(identities.Content[index], "spec"), "env")
		spans = append(spans, authEnvironmentValueSpans(input, env)...)
	}
	return spans
}

func authEnvironmentValueSpans(input string, env *yaml.Node) []authTemplateSpan {
	if env == nil || env.Kind != yaml.MappingNode {
		return nil
	}
	var spans []authTemplateSpan
	for valueIndex := 1; valueIndex < len(env.Content); valueIndex += 2 {
		value := env.Content[valueIndex]
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			continue
		}
		start, end := authTemplateScalarSpan(input, value)
		if start >= 0 && end >= start && strings.Contains(input[start:end], "{{") {
			spans = append(spans, authTemplateSpan{start: start, end: end, value: input[start:end]})
		}
	}
	return spans
}

// authTemplateYAML blanks standalone template controls only for locating YAML scalars.
// The returned source is never rendered, so controls and comments retain their spelling.
func authTemplateYAML(input string) string {
	lines := strings.Split(input, "\n")
	for index, line := range lines {
		if authTemplateControlLine.MatchString(strings.TrimSpace(line)) {
			lines[index] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func authTemplateScalarSpan(input string, node *yaml.Node) (int, int) {
	start, lines := authTemplateScalarStart(input, node)
	if start < 0 {
		return -1, -1
	}
	line := lines[node.Line-1]
	column := []rune(line)
	if node.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
		return start, authTemplateQuotedEnd(input, start)
	}
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	end := start + len(input[start:])
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

func authTemplateScalarStart(input string, node *yaml.Node) (int, []string) {
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

func authTemplateQuotedEnd(input string, start int) int {
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

func authMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func visitAuthMappings(node *yaml.Node, visit func(*yaml.Node)) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			if node.Content[index].Value == "auth" {
				visit(node.Content[index+1])
			}
		}
	}
	for _, child := range node.Content {
		visitAuthMappings(child, visit)
	}
}
