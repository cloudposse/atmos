// Package templatesource defers identity-owned credential templates until authentication.
package templatesource

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	authKey          = "auth"
	indentWhitespace = " \t"
	templateOpening  = "{{"
	newline          = "\n"
)

var (
	commentAction  = regexp.MustCompile(`(?m)^[ \t]*\{\{-?[ \t\r\n]*/\*[^*]*\*+(?:[^/*][^*]*\*+)*/[ \t\r\n]*-?\}\}[ \t\r]*$`)
	controlLine    = regexp.MustCompile(`^\{\{-?\s*(if|else|end|range|with|define|block|template)\b(?:[^"'\x60{}]|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\x60[^\x60]*\x60)*\}\}$`)
	controlOpening = regexp.MustCompile(`\{\{-?\s*(if|range|with|define|block)\b(?:[^"'\x60]|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\x60[^\x60]*\x60)*?\}\}`)
	mappingKey     = regexp.MustCompile(`(?m)(^|[,{]|\}\})\s*(?:\?\s+)?("(?:[^"\\]|\\.)*"|'(?:[^']|'')*'|[[:alnum:]_]+)\s*:`)
)

// sourceIndex maps decoder rune columns back to original byte positions once per input.
type sourceIndex struct {
	text          string
	lines         []string
	offsets       []int
	authProbe     authShapeProbe
	authVisited   map[*yaml.Node]bool
	mergeVisits   map[*yaml.Node]mergeVisitState
	controlMarker rune
}

func newSourceIndex(input string) *sourceIndex {
	lines := strings.SplitAfter(input, newline)
	offsets := make([]int, len(lines))
	offset := 0
	for index, line := range lines {
		offsets[index] = offset
		offset += len(line)
	}
	source := &sourceIndex{text: input, lines: lines, offsets: offsets}
	source.authProbe.source = source
	return source
}

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
	if !strings.Contains(input, templateOpening) || !mayContainAuth(input) {
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

// mayContainAuth only skips YAML decoding when a credential path cannot be present.
// Escaped keys must still be decoded even when their ASCII spellings are absent.
func mayContainAuth(input string) bool {
	if strings.ContainsRune(input, '\\') {
		return true
	}
	for _, key := range []string{authKey, "identities", "spec", "env"} {
		if !strings.Contains(input, key) {
			return templatedMappingKeySyntax.MatchString(input)
		}
	}
	return true
}

func documentTemplateSpans(input string) ([]templateSpan, error) {
	source := newSourceIndex(input)
	if controlOpening.MatchString(input) {
		source.controlMarker = projectionMarker(input)
		if source.controlMarker == 0 {
			return nil, inlineTemplateError()
		}
	}
	decoder := yaml.NewDecoder(strings.NewReader(templateYAML(input, source.controlMarker)))
	var spans []templateSpan
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return spans, nil
		}
		if err != nil {
			if !rejectMalformedAuthSource(input) {
				return nil, nil
			}
			return nil, errUtils.Build(errUtils.ErrInvalidAuthConfig).
				WithExplanation("Auth credential ownership cannot be proven from malformed YAML.").
				WithHint("Use valid raw YAML with literal auth, identities, spec, and env keys. Balance template controls and use inline quoted strings or block scalars; ambiguous vars/settings data follows the same rule.").Err()
		}
		if err := visitAuthMappings(source, &document, func(auth *yaml.Node) error {
			values, err := environmentTemplateSpans(source, auth)
			spans = append(spans, values...)
			return err
		}); err != nil {
			return nil, err
		}
	}
}

// hasRawAuthPathKeys collects credential-key syntax without claiming ownership.
// Rejection uses this evidence only when the inert ownership projection cannot decode.
// Quoted keys are decoded independently so escaped spellings retain the same protection.
func hasRawAuthPathKeys(input string) bool {
	keys := make(map[string]bool, 4)
	for _, match := range mappingKey.FindAllStringSubmatch(input, -1) {
		var key string
		if yaml.Unmarshal([]byte(match[2]), &key) != nil {
			continue
		}
		switch key {
		case authKey, "identities", "spec", "env":
			keys[key] = true
		}
	}
	return len(keys) == 4
}

func protectSpans(input string, spans []templateSpan) (string, func(string) string) {
	// Each restoration map owns a fresh nonce, so rendered data cannot predict its tokens.
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	prefix := "ATMOS_AUTH_ENV_TEMPLATE_" + base64.RawURLEncoding.EncodeToString(id[:]) + "_"
	for strings.Contains(input, prefix) {
		prefix += "X"
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	replacements := make([]string, 0, 2*len(spans))
	var output strings.Builder
	output.Grow(len(input))
	last := 0
	for _, span := range spans {
		placeholder := fmt.Sprintf("%s%d_END", prefix, len(replacements))
		replacements = append(replacements, placeholder, span.value)
		output.WriteString(input[last:span.start])
		output.WriteString(placeholder)
		last = span.end
	}
	output.WriteString(input[last:])
	return output.String(), strings.NewReplacer(replacements...).Replace
}

func environmentTemplateSpans(source *sourceIndex, auth *yaml.Node) ([]templateSpan, error) {
	identities, err := mappingPaths(source, auth, "identities")
	if err != nil {
		return nil, err
	}
	var spans []templateSpan
	for _, identitySet := range identities {
		if identitySet.node.Kind != yaml.MappingNode {
			continue
		}
		for index := 1; index < len(identitySet.node.Content); index += 2 {
			envs, err := mappingPaths(source, identitySet.node.Content[index], "spec", "env")
			if err != nil {
				return nil, err
			}
			for _, env := range envs {
				values, err := environmentValueSpans(source, env.node, identitySet.aliased || env.aliased)
				if err != nil {
					return nil, err
				}
				spans = append(spans, values...)
			}
		}
	}
	return spans, nil
}

func environmentValueSpans(source *sourceIndex, env *yaml.Node, aliased bool) ([]templateSpan, error) {
	if env == nil {
		return nil, nil
	}
	if env.Kind != yaml.MappingNode {
		return nil, inlineTemplateError()
	}
	var spans []templateSpan
	for valueIndex := 1; valueIndex < len(env.Content); valueIndex += 2 {
		key := env.Content[valueIndex-1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || strings.Contains(key.Value, templateOpening) {
			return nil, inlineTemplateError()
		}
		value := env.Content[valueIndex]
		if templateBeforeValue(source, key, value) || requiresInlineTemplate(source, value, aliased) {
			return nil, inlineTemplateError()
		}
		if span, ok := environmentValueSpan(source, value); ok {
			spans = append(spans, span)
		}
	}
	return spans, nil
}

func requiresInlineTemplate(source *sourceIndex, value *yaml.Node, aliased bool) bool {
	// Unquoted actions parse as composite YAML nodes without braces in their values.
	// Only scalar values, including aliases to literal scalars, can be shielded safely.
	scalar := value
	if scalar.Kind == yaml.AliasNode {
		scalar = scalar.Alias
	}
	if scalar.Kind != yaml.ScalarNode {
		return true
	}
	if unquotedTemplate(source, value) {
		return true
	}
	if !strings.Contains(scalar.Value, templateOpening) && maskedScalarTemplate(source, scalar) {
		return true
	}
	// An anchor can live outside auth, so hiding only the alias would still
	// let the general renderer execute its template before credentials exist.
	indirect := aliased || value.Kind == yaml.AliasNode || value.Anchor != "" || value.Style&yaml.TaggedStyle != 0
	return indirect && containsTemplate(source, value, make(map[*yaml.Node]bool))
}

// templateBeforeValue rejects controls hidden as comments before a decoded env scalar.
// A control-only value decodes as an implicit null, so inspect its original indented extent.
func templateBeforeValue(source *sourceIndex, key, value *yaml.Node) bool {
	if value.Tag == "!!null" && value.Value == "" {
		return scalarContainsTemplate(source, value)
	}
	if value.Line <= key.Line {
		return false
	}
	start := scalarStart(source, key)
	end := scalarStart(source, value)
	return start >= 0 && end > start && strings.Contains(source.text[start:end], templateOpening)
}

func scalarContainsTemplate(source *sourceIndex, value *yaml.Node) bool {
	start, end := scalarSpan(source, value)
	return start >= 0 && end >= start && strings.Contains(source.text[start:end], templateOpening)
}

// unquotedTemplate prevents projection from turning an unquoted action into an accepted scalar.
func unquotedTemplate(source *sourceIndex, value *yaml.Node) bool {
	if value.Kind != yaml.ScalarNode || value.Style != 0 {
		return false
	}
	start := scalarStart(source, value)
	return start >= 0 && strings.HasPrefix(source.text[start:], templateOpening)
}

func environmentValueSpan(source *sourceIndex, value *yaml.Node) (templateSpan, bool) {
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || !strings.Contains(value.Value, templateOpening) {
		return templateSpan{}, false
	}
	start, end := scalarSpan(source, value)
	if start < 0 || end < start || !strings.Contains(source.text[start:end], templateOpening) {
		return templateSpan{}, false
	}
	return templateSpan{start: start, end: end, value: source.text[start:end]}, true
}

func inlineTemplateError() error {
	return errUtils.Build(errUtils.ErrInvalidAuthConfig).
		WithExplanation("Auth identity spec.env credential templates require inline strings without YAML anchors, aliases, merge keys, or explicit tags.").
		WithHint("Write each credential template inline in spec.env as a quoted string or block scalar.").Err()
}

func containsTemplate(source *sourceIndex, node *yaml.Node, visited map[*yaml.Node]bool) bool {
	if node == nil || visited[node] {
		return false
	}
	visited[node] = true
	if strings.Contains(node.Value, templateOpening) || maskedScalarTemplate(source, node) || containsTemplate(source, node.Alias, visited) {
		return true
	}
	for _, child := range node.Content {
		if containsTemplate(source, child, visited) {
			return true
		}
	}
	return false
}

// templateYAML projects control syntax only for locating YAML scalars.
// Standalone controls become comments, retaining template markers inside quoted/block scalars.
// Inline block openers become same-width scalar text; closing actions retain their markers too.
// The returned source is never rendered, so controls and comments retain their spelling.
func templateYAML(input string, marker rune) string {
	input = commentAction.ReplaceAllStringFunc(input, commentTemplateAction)
	lines := strings.Split(input, newline)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if controlLine.MatchString(trimmed) {
			indent := len(line) - len(strings.TrimLeft(line, indentWhitespace))
			lines[index] = line[:indent] + "#" + line[indent:]
			continue
		}
		// Keep inline wrappers parseable for the mapping walk without erasing their payload.
		// A comment retains the marker inside block/quoted data for alias checks.
		if inlineAuthControl(trimmed) {
			end := strings.LastIndex(line, templateOpening)
			if end > 0 && controlLine.MatchString(strings.TrimSpace(line[end:])) {
				line = line[:end] + " #" + line[end:]
			}
		}
		lines[index] = controlOpening.ReplaceAllStringFunc(line, func(action string) string {
			return string(marker) + strings.Repeat("x", utf8.RuneCountInString(action)-1)
		})
	}
	return strings.Join(lines, newline)
}

// commentTemplateAction preserves line positions and template markers in scalar data.
// Do not add content to blank lines, which may belong to an indented block scalar.
func commentTemplateAction(action string) string {
	lines := strings.Split(action, newline)
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, indentWhitespace))
		lines[index] = line[:indent] + "#" + line[indent:]
	}
	return strings.Join(lines, newline)
}

// inlineAuthControl detects a leading control followed by an explicit credential mapping.
// Its payload must not be erased as a comment or renamed by scalar projection.
func inlineAuthControl(line string) bool {
	opening := controlOpening.FindStringIndex(line)
	if len(opening) != 2 || opening[0] != 0 || opening[1] >= len(line) {
		return false
	}
	payload := strings.TrimSpace(line[opening[1]:])
	return leadingAuthMapping(payload)
}

// leadingAuthMapping recognizes an auth key even when its children are on later lines.
func leadingAuthMapping(payload string) bool {
	key := mappingKey.FindStringSubmatchIndex(payload)
	if len(key) == 0 || key[0] != 0 {
		return false
	}
	var name string
	return yaml.Unmarshal([]byte(payload[key[4]:key[5]]), &name) == nil && name == authKey
}

// inlineAuthControlAt attributes wrapper syntax to a mapping key, not scalar contents.
func inlineAuthControlAt(lines []string, key *yaml.Node) bool {
	if key.Line < 1 || key.Line > len(lines) {
		return false
	}
	line := lines[key.Line-1]
	if !strings.HasPrefix(strings.TrimSpace(line), templateOpening) {
		return false
	}
	columns := []rune(line)
	if key.Column < 1 || key.Column > len(columns) {
		return false
	}
	return inlineAuthControl(strings.TrimSpace(string(columns[key.Column-1:])))
}

func scalarSpan(source *sourceIndex, node *yaml.Node) (int, int) {
	start := scalarStart(source, node)
	if start < 0 {
		return -1, -1
	}
	line := source.lines[node.Line-1]
	if node.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
		return start, quotedEnd(source.text, start)
	}
	indent := len(line) - len(strings.TrimLeft(line, indentWhitespace))
	end := len(source.text)
	for index := node.Line; index < len(source.lines); index++ {
		next := source.lines[index]
		if strings.TrimSpace(next) != "" && len(next)-len(strings.TrimLeft(next, indentWhitespace)) <= indent {
			end = source.offsets[index]
			break
		}
	}
	// Leave the scalar's final line ending outside the placeholder.
	if end > start && source.text[end-1] == '\n' {
		end--
	}
	return start, end
}

func scalarStart(source *sourceIndex, node *yaml.Node) int {
	if node.Line < 1 || node.Line > len(source.lines) || node.Column < 1 {
		return -1
	}
	column := 1
	for offset := range source.lines[node.Line-1] {
		if column == node.Column {
			return source.offsets[node.Line-1] + offset
		}
		column++
	}
	return -1
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

type mappingMatch struct {
	node    *yaml.Node
	aliased bool
}

// mappingPaths visits every occurrence, including alternate template branches and duplicate keys.
func mappingPaths(source *sourceIndex, node *yaml.Node, keys ...string) ([]mappingMatch, error) {
	return appendMappingPaths(source, nil, node, false, keys)
}

func appendMappingPaths(source *sourceIndex, matches []mappingMatch, node *yaml.Node, aliased bool, keys []string) ([]mappingMatch, error) {
	if node == nil {
		return matches, nil
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
		aliased = true
	}
	if templatedMerge(source, node) {
		return nil, inlineTemplateError()
	}
	if len(keys) == 0 {
		return append(matches, mappingMatch{node: node, aliased: aliased}), nil
	}
	if node.Kind != yaml.MappingNode || generatedMappingValue(source, node) {
		return matches, nil
	}
	return appendMappingPathEntries(source, matches, node, aliased, keys)
}

func appendMappingPathEntries(source *sourceIndex, matches []mappingMatch, node *yaml.Node, aliased bool, keys []string) ([]mappingMatch, error) {
	for index := 0; index < len(node.Content); index += 2 {
		if templatedYAMLKey(source, node.Content[index]) {
			return nil, templatedAuthKeyError()
		}
		if node.Content[index].Value != keys[0] {
			continue
		}
		var err error
		matches, err = appendMappingPaths(source, matches, node.Content[index+1], aliased, keys[1:])
		if err != nil {
			return nil, err
		}
	}
	return matches, nil
}

func templatedMerge(source *sourceIndex, node *yaml.Node) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Tag == "!!merge" && containsTemplate(source, node.Content[index+1], make(map[*yaml.Node]bool)) {
			return true
		}
	}
	return false
}

func visitAuthMappings(source *sourceIndex, node *yaml.Node, visit func(*yaml.Node) error) error {
	if node.Kind == yaml.AliasNode {
		return visitMergedAuth(source, node, visit)
	}
	if node.Kind == yaml.MappingNode {
		return visitAuthMappingEntries(source, node, visit)
	}
	for _, child := range node.Content {
		if err := visitAuthMappings(source, child, visit); err != nil {
			return err
		}
	}
	return nil
}

func visitAuthMappingEntries(source *sourceIndex, node *yaml.Node, visit func(*yaml.Node) error) error {
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if key.Tag == "!!merge" {
			if err := visitMergedAuth(source, value, visit); err != nil {
				return err
			}
			continue
		}
		// Application vars and settings are data, including any auth-shaped descendants.
		if key.Value == "vars" || key.Value == "settings" {
			continue
		}
		if err := visitAuthEntry(source, key, value, visit); err != nil {
			return err
		}
		if err := visitAuthMappings(source, value, visit); err != nil {
			return err
		}
	}
	return nil
}

func visitAuthEntry(source *sourceIndex, key, value *yaml.Node, visit func(*yaml.Node) error) error {
	if inlineAuthControlAt(source.lines, key) {
		return errUtils.Build(errUtils.ErrInvalidAuthConfig).
			WithExplanation("Inline template controls cannot wrap auth identity declarations safely.").
			WithHint("Put template controls on standalone lines around auth configuration; keep credential templates inline in spec.env.").Err()
	}
	if templatedYAMLKey(source, key) && source.authProbe.couldBeAuthMapping(value) {
		return templatedAuthKeyError()
	}
	if key.Value == authKey {
		return visitAuthOnce(source, value, visit)
	}
	return nil
}
