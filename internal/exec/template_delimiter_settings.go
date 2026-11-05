package exec

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

// protectTemplateDelimiterSettings shields the delimiter declaration itself, which
// is configuration rather than a template expression. Preserve source bytes so
// raw template syntax elsewhere is never changed by a YAML marshal round trip.
// Restore before decoding each pass so newly introduced delimiters still take effect.
func protectTemplateDelimiterSettings(input string) (string, func(string) string) {
	identity := func(s string) string { return s }
	node := templateDelimiterSettingsNode(input)
	if node == nil {
		return input, identity
	}
	replacements := []string{}
	// Replace in reverse source order to keep the remaining node positions valid.
	for i := len(node.Content) - 1; i >= 0; i-- {
		scalar := node.Content[i]
		if scalar.Kind != yaml.ScalarNode {
			continue
		}
		start := templateScalarOffset(input, scalar.Line, scalar.Column)
		end := templateScalarEnd(input, start, scalar)
		if end <= start {
			continue
		}
		token := literalsource.Encode("template-delimiter:" + strconv.Itoa(i))
		for strings.Contains(input, token) {
			token = literalsource.Encode(token)
		}
		quoted := strconv.Quote(token)
		replacements = append(replacements, quoted, input[start:end])
		input = input[:start] + quoted + input[end:]
	}
	if len(replacements) == 0 {
		return input, identity
	}
	return input, strings.NewReplacer(replacements...).Replace
}

func templateDelimiterSettingsNode(input string) *yaml.Node {
	if !strings.Contains(input, "delimiters") {
		return nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(input), &root); err != nil || len(root.Content) == 0 {
		return nil
	}
	node := root.Content[0]
	for _, key := range []string{"settings", "templates", "settings", "delimiters"} {
		node = templateSettingsChild(node, key)
		if node == nil {
			return nil
		}
	}
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	return node
}

func templateSettingsChild(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func templateScalarOffset(input string, line, column int) int {
	offset := 0
	for current := 1; current < line; current++ {
		next := strings.IndexByte(input[offset:], '\n')
		if next < 0 {
			return len(input)
		}
		offset += next + 1
	}
	for current := 1; current < column && offset < len(input); current++ {
		_, size := utf8.DecodeRuneInString(input[offset:])
		offset += size
	}
	return offset
}

func templateScalarEnd(input string, start int, node *yaml.Node) int {
	if start >= len(input) {
		return start
	}
	quote := input[start]
	if quote != '\'' && quote != '"' {
		if node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return templateBlockScalarEnd(input, start)
		}
		// Tagged and anchored nodes need their prefix preserved. They are not
		// emitted by the merged-map serializer used for component rendering.
		if node.Anchor != "" || quote == '!' {
			return start
		}
		return start + len(node.Value)
	}
	return templateQuotedScalarEnd(input, start, quote)
}

func templateQuotedScalarEnd(input string, start int, quote byte) int {
	for end := start + 1; end < len(input); end++ {
		if quote == '"' && input[end] == '\\' {
			end++
			continue
		}
		if input[end] != quote {
			continue
		}
		if quote == '\'' && end+1 < len(input) && input[end+1] == quote {
			end++
			continue
		}
		return end + 1
	}
	return start
}

func templateBlockScalarEnd(input string, start int) int {
	lineStart := strings.LastIndexByte(input[:start], '\n') + 1
	header := input[lineStart:]
	indent := len(header) - len(strings.TrimLeft(header, " "))
	end := start
	for end < len(input) {
		newline := strings.IndexByte(input[end:], '\n')
		if newline < 0 {
			return len(input)
		}
		next := end + newline + 1
		rest := input[next:]
		line, _, _ := strings.Cut(rest, "\n")
		if strings.TrimSpace(line) != "" && len(line)-len(strings.TrimLeft(line, " ")) <= indent {
			return next - 1
		}
		end = next
	}
	return end
}

func effectiveComponentTemplateDelimiters(config *schema.AtmosConfiguration, settings *schema.Settings) []string {
	if settings.Templates.Settings.Delimiters != nil {
		return settings.Templates.Settings.Delimiters
	}
	return config.Templates.Settings.Delimiters
}
