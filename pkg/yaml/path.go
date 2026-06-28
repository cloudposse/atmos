package yaml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// doubleQuote is the YAML/yq double-quote character used when quoting keys and values.
const doubleQuote = `"`

// simpleKeyRe matches keys that can be written as a bare `.key` segment in a yq
// path expression. Anything outside this set must be quoted as `.["key"]` so
// dots, spaces, and symbols inside keys do not get misparsed.
var simpleKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// isSimpleKey reports whether key can be written as a bare `.key` path segment.
func isSimpleKey(key string) bool {
	return simpleKeyRe.MatchString(key)
}

// DotPathToYqPath converts a user-facing dot-notation path into a yq path
// expression. Dot-notation is the default addressing syntax for the Atmos
// config/stack/vendor editors.
//
// Examples:
//
//	vars.region                       -> .vars.region
//	sources[0].version                -> .sources[0].version
//	components.terraform.vpc.vars.cidr -> .components.terraform.vpc.vars.cidr
//	metadata."weird.key"              -> .metadata.["weird.key"]
//
// Keys that are not simple identifiers are emitted using yq's quoted
// `.["..."]` form so embedded dots and symbols are preserved literally.
func DotPathToYqPath(dotPath string) (string, error) {
	defer perf.Track(nil, "yaml.DotPathToYqPath")()

	trimmed := strings.TrimSpace(dotPath)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidYAMLExpression)
	}

	// Already a yq expression (starts with '.') — pass through unchanged so
	// power users can mix syntaxes.
	if strings.HasPrefix(trimmed, ".") {
		return trimmed, nil
	}

	segments, err := splitDotPath(trimmed)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, seg := range segments {
		switch {
		case seg.isIndex:
			b.WriteString("[")
			b.WriteString(strconv.Itoa(seg.index))
			b.WriteString("]")
		case isSimpleKey(seg.key):
			b.WriteString(".")
			b.WriteString(seg.key)
		default:
			b.WriteString(`.["`)
			b.WriteString(strings.ReplaceAll(seg.key, doubleQuote, `\"`))
			b.WriteString(`"]`)
		}
	}

	return b.String(), nil
}

// pathSegment is a single key or array index in a parsed dot-path.
type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

// dotPathScanner holds the mutable state used while tokenizing a dot-path.
type dotPathScanner struct {
	runes    []rune
	path     string
	segments []pathSegment
	cur      strings.Builder
	// sawContent tracks whether a key or index has been consumed since the last
	// dot separator; lastWasDot tracks whether a dot was the final structural
	// token (to catch a trailing dot after the loop).
	sawContent bool
	lastWasDot bool
}

// flushKey appends the accumulated key buffer as a segment, if non-empty.
func (s *dotPathScanner) flushKey() {
	if s.cur.Len() > 0 {
		s.segments = append(s.segments, pathSegment{key: s.cur.String()})
		s.cur.Reset()
	}
}

// step processes the rune at index i and returns the index of the last rune it
// consumed (so the caller can advance past multi-rune quoted/index segments).
func (s *dotPathScanner) step(i int) (int, error) {
	switch s.runes[i] {
	case '.':
		if !s.sawContent {
			return 0, fmt.Errorf("%w: empty path segment in %q", ErrInvalidYAMLExpression, s.path)
		}
		s.flushKey()
		s.sawContent = false
		s.lastWasDot = true
	case '"':
		next, text, err := scanQuotedSegment(s.runes, i, s.path)
		if err != nil {
			return 0, err
		}
		s.cur.WriteString(text)
		i = next
		s.sawContent = true
		s.lastWasDot = false
	case '[':
		s.flushKey()
		next, index, err := scanIndexSegment(s.runes, i, s.path)
		if err != nil {
			return 0, err
		}
		s.segments = append(s.segments, pathSegment{index: index, isIndex: true})
		i = next
		s.sawContent = true
		s.lastWasDot = false
	default:
		s.cur.WriteRune(s.runes[i])
		s.sawContent = true
		s.lastWasDot = false
	}
	return i, nil
}

// splitDotPath tokenizes a dot-notation path into key and index segments,
// honoring `key[0]` bracket indices and `"quoted.key"` segments. A dot
// separator that produces an empty key segment (e.g. `a..b` or a trailing `a.`)
// is rejected so a typo cannot silently target a different key. The empty buffer
// that legitimately follows a `[N]` index (e.g. `sources[0].version`) is allowed.
func splitDotPath(path string) ([]pathSegment, error) {
	s := &dotPathScanner{runes: []rune(path), path: path}
	for i := 0; i < len(s.runes); i++ {
		next, err := s.step(i)
		if err != nil {
			return nil, err
		}
		i = next
	}
	if s.lastWasDot {
		return nil, fmt.Errorf("%w: trailing '.' in path %q", ErrInvalidYAMLExpression, path)
	}
	s.flushKey()

	if len(s.segments) == 0 {
		return nil, fmt.Errorf("%w: path %q produced no segments", ErrInvalidYAMLExpression, path)
	}
	return s.segments, nil
}

// QuotePathSegment renders a literal map key as a dot-path segment, quoting it as
// "key" when it is not a simple identifier so embedded dots and brackets are
// parsed literally (e.g. a component named "vpc.prod" or "foo[0]") instead of as
// nested path syntax. Simple identifiers are returned unchanged.
func QuotePathSegment(key string) string {
	defer perf.Track(nil, "yaml.QuotePathSegment")()

	if isSimpleKey(key) {
		return key
	}
	return doubleQuote + key + doubleQuote
}

// scanQuotedSegment reads a `"quoted"` key starting at the opening quote (start),
// returning the index of the closing quote and the unquoted text.
func scanQuotedSegment(runes []rune, start int, path string) (int, string, error) {
	j := start + 1
	var quoted strings.Builder
	for j < len(runes) && runes[j] != '"' {
		quoted.WriteRune(runes[j])
		j++
	}
	if j >= len(runes) {
		return 0, "", fmt.Errorf("%w: unterminated quote in path %q", ErrInvalidYAMLExpression, path)
	}
	return j, quoted.String(), nil
}

// scanIndexSegment reads a `[N]` array index starting at the opening bracket
// (start), returning the index of the closing bracket and the parsed integer.
func scanIndexSegment(runes []rune, start int, path string) (int, int, error) {
	j := start + 1
	var idx strings.Builder
	for j < len(runes) && runes[j] != ']' {
		idx.WriteRune(runes[j])
		j++
	}
	if j >= len(runes) {
		return 0, 0, fmt.Errorf("%w: unterminated '[' in path %q", ErrInvalidYAMLExpression, path)
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(idx.String()))
	if convErr != nil {
		return 0, 0, fmt.Errorf("%w: invalid array index %q in path %q", ErrInvalidYAMLExpression, idx.String(), path)
	}
	return j, n, nil
}

// encodeStringValue renders a Go string as a double-quoted YAML/yq scalar,
// escaping backslashes and quotes so the resulting assignment expression is
// safe for arbitrary input.
func encodeStringValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, doubleQuote, `\"`)
	return doubleQuote + escaped + doubleQuote
}
