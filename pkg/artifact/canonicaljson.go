package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The nesting limit bounds depth so hostile kind data cannot exhaust the stack.
	maxJSONDepth = 64

	// The numeric base of canonical integers.
	decimalBase = 10

	// The bit width of canonical integers.
	int64Bits = 64

	// The first character that JSON strings need not escape.
	firstPrintable = 0x20
)

// jsonKind enumerates the node types of the canonicalizer's ordered tree.
type jsonKind int

const (
	jsonObject jsonKind = iota
	jsonArray
	jsonString
	jsonInteger
	jsonBool
	jsonNull
)

// jsonNode is one parsed JSON value. Objects keep their members as an ordered slice rather than a
// map, so no map[string]any or float64 is ever involved in canonicalization.
type jsonNode struct {
	kind    jsonKind
	text    string // String value, canonical integer text, or "true"/"false".
	members []jsonMember
	items   []*jsonNode
}

type jsonMember struct {
	key   string
	value *jsonNode
}

// CanonicalizeJSON re-encodes JSON text into the canonical form used by Records.
//
// The canonical form has object members sorted by key (bytewise on UTF-8), no insignificant
// whitespace, integers in plain decimal, and strings with only the escapes required by JSON (the
// quote, the backslash, and control characters as lowercase \u00xx); every other character is
// written as literal UTF-8, including <, >, &, U+2028 and U+2029. The result is independent of the
// Go version's encoding/json escaping behavior. The input must be valid UTF-8, contain no
// duplicate object keys, no floating-point numbers, no integers outside the int64 range, no
// U+FFFD replacement characters (which indicate invalid input), and no trailing data.
func CanonicalizeJSON(raw []byte) ([]byte, error) {
	defer perf.Track(nil, "artifact.CanonicalizeJSON")()

	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("%w: JSON is not valid UTF-8", errUtils.ErrArtifactRecordInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	root, err := parseJSONNode(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: unexpected data after the JSON value", errUtils.ErrArtifactRecordInvalid)
	}
	var out bytes.Buffer
	root.write(&out)
	return out.Bytes(), nil
}

// parseJSONNode reads one complete JSON value from the decoder's token stream.
func parseJSONNode(decoder *json.Decoder, depth int) (*jsonNode, error) {
	if depth > maxJSONDepth {
		return nil, fmt.Errorf("%w: JSON nesting exceeds %d levels", errUtils.ErrArtifactRecordInvalid, maxJSONDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: malformed JSON: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	return parseJSONToken(decoder, token, depth)
}

// parseJSONToken converts one token (and, for containers, the tokens that follow) into a node.
func parseJSONToken(decoder *json.Decoder, token json.Token, depth int) (*jsonNode, error) {
	switch value := token.(type) {
	case json.Delim:
		if value == '{' {
			return parseJSONObject(decoder, depth)
		}
		return parseJSONArray(decoder, depth)
	case string:
		return parseJSONStringNode(value)
	case json.Number:
		return parseJSONInteger(value)
	case bool:
		return &jsonNode{kind: jsonBool, text: strconv.FormatBool(value)}, nil
	case nil:
		return &jsonNode{kind: jsonNull}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported JSON token %v", errUtils.ErrArtifactRecordInvalid, token)
	}
}

func parseJSONStringNode(value string) (*jsonNode, error) {
	if err := checkJSONString(value); err != nil {
		return nil, err
	}
	return &jsonNode{kind: jsonString, text: value}, nil
}

func parseJSONObject(decoder *json.Decoder, depth int) (*jsonNode, error) {
	node := &jsonNode{kind: jsonObject}
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: malformed JSON object: %w", errUtils.ErrArtifactRecordInvalid, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("%w: JSON object key is not a string", errUtils.ErrArtifactRecordInvalid)
		}
		if err := checkJSONString(key); err != nil {
			return nil, err
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate JSON object key %q", errUtils.ErrArtifactRecordInvalid, key)
		}
		seen[key] = struct{}{}
		value, err := parseJSONNode(decoder, depth+1)
		if err != nil {
			return nil, err
		}
		node.members = append(node.members, jsonMember{key: key, value: value})
	}
	if _, err := decoder.Token(); err != nil { // Consume '}'.
		return nil, fmt.Errorf("%w: malformed JSON object: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	sort.Slice(node.members, func(i, j int) bool { return node.members[i].key < node.members[j].key })
	return node, nil
}

func parseJSONArray(decoder *json.Decoder, depth int) (*jsonNode, error) {
	node := &jsonNode{kind: jsonArray}
	for decoder.More() {
		item, err := parseJSONNode(decoder, depth+1)
		if err != nil {
			return nil, err
		}
		node.items = append(node.items, item)
	}
	if _, err := decoder.Token(); err != nil { // Consume ']'.
		return nil, fmt.Errorf("%w: malformed JSON array: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	return node, nil
}

// parseJSONInteger accepts only plain decimal integers that fit in an int64.
func parseJSONInteger(number json.Number) (*jsonNode, error) {
	value, err := strconv.ParseInt(number.String(), decimalBase, int64Bits)
	if err != nil {
		return nil, fmt.Errorf("%w: number %s is not an int64 integer; floats are not allowed", errUtils.ErrArtifactRecordInvalid, number)
	}
	return &jsonNode{kind: jsonInteger, text: strconv.FormatInt(value, decimalBase)}, nil
}

// checkJSONString rejects U+FFFD, which is how encoding/json surfaces invalid UTF-8 and lone
// surrogate escapes. Accepting it would silently corrupt data.
func checkJSONString(s string) error {
	if strings.ContainsRune(s, utf8.RuneError) {
		return fmt.Errorf("%w: string contains an invalid UTF-8 sequence or U+FFFD", errUtils.ErrArtifactRecordInvalid)
	}
	return nil
}

func (n *jsonNode) write(out *bytes.Buffer) {
	switch n.kind {
	case jsonObject:
		out.WriteByte('{')
		for i, member := range n.members {
			if i > 0 {
				out.WriteByte(',')
			}
			writeJSONString(out, member.key)
			out.WriteByte(':')
			member.value.write(out)
		}
		out.WriteByte('}')
	case jsonArray:
		out.WriteByte('[')
		for i, item := range n.items {
			if i > 0 {
				out.WriteByte(',')
			}
			item.write(out)
		}
		out.WriteByte(']')
	case jsonString:
		writeJSONString(out, n.text)
	case jsonInteger, jsonBool:
		out.WriteString(n.text)
	case jsonNull:
		out.WriteString("null")
	}
}

const hexDigits = "0123456789abcdef"

// writeJSONString writes s with the minimal, Go-version-independent escape set.
func writeJSONString(out *bytes.Buffer, s string) {
	out.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			out.WriteByte('\\')
			out.WriteByte(c)
		case c < firstPrintable:
			out.WriteString(`\u00`)
			out.WriteByte(hexDigits[c>>4])
			out.WriteByte(hexDigits[c&0x0f])
		default:
			out.WriteByte(c)
		}
	}
	out.WriteByte('"')
}

// ReadBounded reads at most limit bytes from r and fails with ErrArtifactTooLarge when the stream
// is longer, so callers never buffer unbounded input.
func ReadBounded(r io.Reader, limit int64) ([]byte, error) {
	defer perf.Track(nil, "artifact.ReadBounded")()

	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: more than %d bytes", errUtils.ErrArtifactTooLarge, limit)
	}
	return data, nil
}
