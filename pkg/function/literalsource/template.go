package literalsource

import (
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

const placeholderRuneStart = '\ue000'

var envelopePattern = regexp.MustCompile(regexp.QuoteMeta(prefix) + `[A-Za-z0-9_-]*:END`)

// ProtectEncoded prevents even delimiter pairs that overlap our envelope syntax from
// interpreting a literal. Tokens use characters absent from the input and delimiters.
// Restore after each pass, before parsing the rendered YAML or choosing new delimiters.
func ProtectEncoded(input, left, right string) (string, func(string) string) {
	defer perf.Track(nil, "literalsource.ProtectEncoded")()

	identity := func(value string) string { return value }
	if !strings.Contains(input, prefix) {
		return input, identity
	}
	used := input + left + right
	chars := []rune{}
	for r := rune(placeholderRuneStart); len(chars) < 2; r++ {
		if !strings.ContainsRune(used, r) {
			chars = append(chars, r)
		}
	}
	replacements := []string{}
	masked := envelopePattern.ReplaceAllStringFunc(input, func(value string) string {
		if _, ok := Decode(value); !ok {
			return value
		}
		token := string(chars[0]) + strings.Repeat(string(chars[1]), len(replacements)/2+1) + string(chars[0])
		replacements = append(replacements, token, value)
		return token
	})
	if len(replacements) == 0 {
		return input, identity
	}
	return masked, strings.NewReplacer(replacements...).Replace
}

// ProtectYAML hides tagged scalar bodies before a manifest's first template pass.
// Its restore callback reinstates the original tagged strings only after all passes.
func ProtectYAML(input string) (string, func(string) string, error) {
	defer perf.Track(nil, "literalsource.ProtectYAML")()

	identity := func(value string) string { return value }
	if !strings.Contains(input, "!literal") {
		return input, identity, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(input), &root); err != nil {
		masked, restore := protectTemplateScalars(input)
		return masked, restore, nil
	}
	replacements := []string{}
	protectNode(&root, input, &replacements)
	if len(replacements) == 0 {
		masked, restore := protectTemplateScalars(input)
		return masked, restore, nil
	}
	encoded, err := yaml.Marshal(&root)
	if err != nil {
		return "", identity, err
	}
	return string(encoded), strings.NewReplacer(replacements...).Replace, nil
}

func protectNode(node *yaml.Node, input string, replacements *[]string) {
	if node.Tag == "!literal" {
		if node.Kind != yaml.ScalarNode {
			return
		}
		original := node.Value
		token := Encode("literal-source:" + strconv.Itoa(len(*replacements)))
		for strings.Contains(input, token) {
			token = Encode(token)
		}
		*replacements = append(*replacements, strconv.Quote(token), strconv.Quote(original))
		node.Value, node.Style = token, yaml.DoubleQuotedStyle
		return
	}
	for _, child := range node.Content {
		protectNode(child, input, replacements)
	}
}
