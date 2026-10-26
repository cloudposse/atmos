package starlark

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// compileAutomation permits .load as an attribute in the automation dialect.
// Upstream reserves load even after a dot. Temporarily capitalize only these
// tokens, then restore their AST names before resolution. Equal-width tokens
// preserve every authored source location without forking the upstream parser.
func compileAutomation(filename, source string, globals starlark.StringDict) (*starlark.Program, error) {
	adapted, positions := loadAttributes(source)
	file, err := fileOptions.Parse(filename, adapted, 0)
	if err != nil {
		return nil, err
	}
	if len(positions) > 0 {
		syntax.Walk(file, func(node syntax.Node) bool {
			if dot, ok := node.(*syntax.DotExpr); ok {
				pos := dot.Name.NamePos
				if positions[[2]int32{pos.Line, pos.Col}] {
					dot.Name.Name = "load"
				}
			}
			return true
		})
	}
	return starlark.FileProgram(file, globals.Has)
}

func execAutomation(thread *starlark.Thread, filename, source string, globals starlark.StringDict) (starlark.StringDict, error) {
	program, err := compileAutomation(filename, source, globals)
	if err != nil {
		return nil, err
	}
	result, err := program.Init(thread, globals)
	// Match ExecFileOptions, including freezing partially evaluated globals on error.
	result.Freeze()
	return result, err
}

// loadAttributes skips strings and comments; all other syntax is still checked
// by the upstream parser. Raw/byte prefixes need no special handling because
// quotes and backslash escapes delimit their contents in the same way.
func loadAttributes(source string) (string, map[[2]int32]bool) {
	if !strings.Contains(source, "load") {
		return source, nil
	}
	offsets := loadAttributeOffsets(source)
	if len(offsets) == 0 {
		return source, nil
	}
	adapted := []byte(source)
	for offset := range offsets {
		adapted[offset] = 'L'
	}
	return string(adapted), sourcePositions(source, offsets)
}

func loadAttributeOffsets(source string) map[int]bool {
	offsets := make(map[int]bool)
	dot := false
	for i := 0; i < len(source); {
		i = skipSourceTrivia(source, i)
		if i == len(source) {
			break
		}
		if source[i] == '\'' || source[i] == '"' {
			i = skipQuotedSource(source, i)
			dot = false
			continue
		}
		if dot && isLoadIdentifier(source, i) {
			offsets[i] = true
		}
		dot = source[i] == '.'
		i++
	}
	return offsets
}

// skipSourceTrivia skips whitespace, comments, and explicit line continuations.
// It leaves malformed continuations in place for the upstream parser to reject.
func skipSourceTrivia(source string, i int) int {
	for i < len(source) {
		switch source[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '#':
			end := strings.IndexAny(source[i:], "\r\n")
			if end < 0 {
				return len(source)
			}
			i += end
		case '\\':
			if i+1 >= len(source) || !strings.ContainsRune("\r\n", rune(source[i+1])) {
				return i
			}
			i++
		default:
			return i
		}
	}
	return i
}

func isLoadIdentifier(source string, offset int) bool {
	return strings.HasPrefix(source[offset:], "load") && identifierEndsAt(source, offset+4)
}

func identifierEndsAt(source string, offset int) bool {
	if offset == len(source) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(source[offset:])
	return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func skipQuotedSource(source string, start int) int {
	width := 1
	if start+2 < len(source) && source[start+1] == source[start] && source[start+2] == source[start] {
		width = 3
	}
	quote := source[start : start+width]
	for i := start + width; i < len(source); {
		switch {
		case source[i] == '\\':
			i += 2
		case strings.HasPrefix(source[i:], quote):
			return i + width
		default:
			i++
		}
	}
	return len(source)
}

// sourcePositions uses Starlark's rune columns and CR/LF/CRLF newline rules.
func sourcePositions(source string, offsets map[int]bool) map[[2]int32]bool {
	positions := make(map[[2]int32]bool, len(offsets))
	line, col := int32(1), int32(1)
	for i := 0; i < len(source); {
		if offsets[i] {
			positions[[2]int32{line, col}] = true
		}
		r, width := utf8.DecodeRuneInString(source[i:])
		i += width
		if r == '\r' && i < len(source) && source[i] == '\n' {
			i++
		}
		if r == '\r' || r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return positions
}
