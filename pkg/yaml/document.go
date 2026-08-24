package yaml

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"

	goyaml "gopkg.in/yaml.v3"
)

// maxDetectedIndent caps indentation-width detection; anything wider than this
// is treated as an outlier and falls back to DefaultIndent.
const maxDetectedIndent = 8

// blockScalarIntroRe matches a line that introduces a block scalar (`|` or `>`
// with optional indentation indicator and chomping modifier, and an optional
// trailing comment). Lines that follow such an introducer are scalar content,
// not structure, so they must not drive indent detection.
var blockScalarIntroRe = regexp.MustCompile(`[|>][0-9]*[+-]?[0-9]*\s*(#.*)?$`)

// ensureSingleDocument rejects streams containing more than one YAML document.
// The yq engine applies an expression to every document in a stream, so a
// Set/Delete on a multi-document file would silently rewrite all documents;
// the editor makes that an explicit error instead. Invalid YAML is
// deliberately not reported here — the yq evaluation surfaces parse errors
// with better context.
func ensureSingleDocument(content []byte) error {
	dec := goyaml.NewDecoder(bytes.NewReader(content))
	docs := 0
	for {
		var node goyaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			//nolint:nilerr // Parse errors are intentionally deferred to the yq evaluation, which reports them with context.
			return nil
		}
		docs++
		if docs > 1 {
			return ErrYAMLMultiDocUnsupported
		}
	}
}

// isEmptyDocument reports whether content contains no YAML at all (empty or
// whitespace-only). The yq engine evaluates expressions zero times on such
// input, so editing operations must seed a null document to have anything to
// write into.
func isEmptyDocument(content []byte) bool {
	return len(bytes.TrimSpace(content)) == 0
}

// nullDocument is the seed used when editing an empty file: a single null
// document that assignments can turn into a mapping or sequence.
var nullDocument = []byte("null\n")

// editBase returns the document an editing expression should run against:
// the content itself, or a seeded null document when the content is empty.
func editBase(content []byte) []byte {
	if isEmptyDocument(content) {
		return nullDocument
	}
	return content
}

// detectIndent infers the document's indentation width from the first line
// that starts a nesting level, so a whole-file re-encode keeps (for example) a
// 4-space file at 4 spaces instead of re-indenting everything to the default.
// Only a transition from a column-0 line to an indented line is trusted, and
// lines following a block-scalar introducer are ignored (they are scalar
// content). Falls back to DefaultIndent when nothing usable is found.
func detectIndent(content []byte) int {
	indent, _ := detectIndentWidth(content)
	return indent
}

// detectIndentWidth infers indentation and reports whether the value came
// from document structure rather than the package default.
func detectIndentWidth(content []byte) (int, bool) {
	prev := ""
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(trimmed)
		if indent > 0 && prev != "" && !strings.HasPrefix(prev, " ") && !blockScalarIntroRe.MatchString(prev) {
			if indent >= DefaultIndent && indent <= maxDetectedIndent {
				return indent, true
			}
			return DefaultIndent, false
		}
		prev = line
	}
	return DefaultIndent, false
}

// Line-ending byte sequences used by restoreLineEndings.
var (
	lfBytes   = []byte("\n")
	crlfBytes = []byte("\r\n")
)

// restoreLineEndings re-applies CRLF line endings to out when the original
// content used CRLF exclusively. The yq encoder always emits LF, so without
// this a one-key edit of a CRLF file would rewrite every line ending. Files
// with mixed endings are left as the encoder produced them (LF).
func restoreLineEndings(original, out []byte) []byte {
	crlfCount := bytes.Count(original, crlfBytes)
	if crlfCount == 0 || crlfCount != bytes.Count(original, lfBytes) {
		return out
	}
	// Normalize defensively before expanding so no CRLF is doubled.
	normalized := bytes.ReplaceAll(out, crlfBytes, lfBytes)
	return bytes.ReplaceAll(normalized, lfBytes, crlfBytes)
}
