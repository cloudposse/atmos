package asciicast

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Tape errors, aliased from the shared sentinel set so callers outside this
// package can match them with errors.Is without importing errors/errors.go.
var (
	ErrTapeParseFailed          = errUtils.ErrTapeParseFailed
	ErrUnsupportedTapeDirective = errUtils.ErrUnsupportedTapeDirective
	ErrTapeSourceNotFound       = errUtils.ErrTapeSourceNotFound
	ErrTapeSourceCycle          = errUtils.ErrTapeSourceCycle
)

// TapeDirectiveKind identifies one recognized VHS-dialect directive kind.
type TapeDirectiveKind int

// Recognized tape directive kinds. Wait+Screen and Wait+Line are parsed but
// intentionally not distinguished from a bare Wait at the AST level: Atmos's
// session engine polls a captured output buffer rather than literally
// emulating VHS's per-frame screen capture, so all three resolve to the same
// runtime behavior; the distinction only matters as authoring guidance (see
// the atmos-vhs skill's "Wait vs Wait+Screen" section).
const (
	TapeSet TapeDirectiveKind = iota
	TapeOutput
	TapeRequire
	TapeSource
	TapeHide
	TapeShow
	TapeType
	TapeSleep
	TapeKey
	TapeWait
	TapeScreenshot
	TapeCopy
	TapePaste
	TapeEnv
)

// TapeDirective is one parsed VHS-dialect directive. Not every field is used
// by every Kind; see the parser's consume* functions for which fields a given
// Kind populates.
type TapeDirective struct {
	Kind     TapeDirectiveKind
	File     string // Originating file path; "" for an inline tape with no Source ancestry.
	Line     int    // 1-based line number where the directive starts, for error messages.
	Key      string // Set key name, keypress name (enter/down/ctrl+c/...), or Env variable name.
	Value    string // Set value / Type text / Output|Source|Require|Screenshot path / Env value / Copy text.
	Duration string // Sleep duration.
	Repeat   int    // Keypress repeat count; 0 means "once" (not repeated).
	Regex    string // Wait regex pattern, without the surrounding slashes.
}

// Tape is a fully parsed (and Source-inlined) VHS-dialect script.
type Tape struct {
	Directives []TapeDirective
}

// TapeError carries file/line context for a tape parse or Source-resolution
// failure, so a reported error always names the offending directive.
type TapeError struct {
	File string
	Line int
	Err  error
}

func (e *TapeError) Error() string {
	defer perf.Track(nil, "asciicast.TapeError.Error")()

	if e.File != "" {
		return fmt.Sprintf("%s:%d: %v", e.File, e.Line, e.Err)
	}
	return fmt.Sprintf("tape line %d: %v", e.Line, e.Err)
}

func (e *TapeError) Unwrap() error {
	defer perf.Track(nil, "asciicast.TapeError.Unwrap")()

	return e.Err
}

func tapeErr(file string, line int, err error) error {
	return &TapeError{File: file, Line: line, Err: err}
}

// TapeFileReader resolves and reads a tape file referenced by a Source
// directive. Production callers use an adapter over pkg/filesystem.FileSystem;
// tests inject an in-memory fake.
type TapeFileReader interface {
	ReadFile(path string) ([]byte, error)
}

// ParseTape lexes and parses source as a VHS-dialect tape and recursively
// inlines any Source directives, resolving relative Source paths against
// baseDir (the directory of the file source came from, or the caller's chosen
// base directory for an inline tape with no originating file). The file
// parameter is used purely for error messages and is not read from disk;
// pass "" for an inline tape block.
func ParseTape(source, file, baseDir string, fr TapeFileReader) (*Tape, error) {
	defer perf.Track(nil, "asciicast.ParseTape")()

	visited := map[string]bool{}
	directives, err := parseAndSplice(source, file, baseDir, fr, visited)
	if err != nil {
		return nil, err
	}
	return &Tape{Directives: directives}, nil
}

// ParseTapeFile reads path from disk via fr and parses it as a VHS-dialect
// tape. Source directives (at any nesting depth) resolve relative to baseDir,
// matching real VHS: `vhs script.tape` resolves every Source in the script
// relative to vhs's own invocation CWD, not relative to whichever file a
// nested Source happened to be read from -- see ParseTape.
func ParseTapeFile(path, baseDir string, fr TapeFileReader) (*Tape, error) {
	defer perf.Track(nil, "asciicast.ParseTapeFile")()

	content, err := fr.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrTapeSourceNotFound, path, err)
	}
	return ParseTape(string(content), path, baseDir, fr)
}

func parseAndSplice(source, file, baseDir string, fr TapeFileReader, visited map[string]bool) ([]TapeDirective, error) {
	tokens, err := tokenizeTape(source, file)
	if err != nil {
		return nil, err
	}
	directives, err := parseTapeTokens(tokens, file)
	if err != nil {
		return nil, err
	}
	return spliceSource(directives, baseDir, fr, visited)
}

func spliceSource(directives []TapeDirective, baseDir string, fr TapeFileReader, visited map[string]bool) ([]TapeDirective, error) {
	spliced := make([]TapeDirective, 0, len(directives))
	for _, directive := range directives {
		if directive.Kind != TapeSource {
			spliced = append(spliced, directive)
			continue
		}
		nested, err := loadSource(&directive, baseDir, fr, visited)
		if err != nil {
			return nil, err
		}
		spliced = append(spliced, nested...)
	}
	return spliced, nil
}

// loadSource resolves and inlines one Source directive's target file. The
// baseDir parameter is passed through unchanged to the recursive
// parseAndSplice call (not rebased to the sourced file's own directory):
// real VHS resolves every Source in a script relative to its single
// invocation CWD, regardless of how many Source directives are chased --
// confirmed against this repo's own demo/landing/*.tape, which all write
// `Source demo/landing/defaults.tape` (a path relative to the repo root vhs
// is invoked from, not relative to demo/landing/ itself, which is what
// filepath.Dir(canonical) would rebase to, producing a nonexistent
// demo/landing/demo/landing/defaults.tape).
func loadSource(directive *TapeDirective, baseDir string, fr TapeFileReader, visited map[string]bool) ([]TapeDirective, error) {
	path := directive.Value
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	canonical := filepath.Clean(path)
	if visited[canonical] {
		return nil, tapeErr(directive.File, directive.Line, fmt.Errorf("%w: %s", ErrTapeSourceCycle, canonical))
	}
	content, err := fr.ReadFile(canonical)
	if err != nil {
		return nil, tapeErr(directive.File, directive.Line, fmt.Errorf("%w: %s: %w", ErrTapeSourceNotFound, canonical, err))
	}
	childVisited := make(map[string]bool, len(visited)+1)
	for k, v := range visited {
		childVisited[k] = v
	}
	childVisited[canonical] = true
	return parseAndSplice(string(content), canonical, baseDir, fr, childVisited)
}

// tapeTokenKind identifies the lexical shape of one scanned token.
type tapeTokenKind int

const (
	tapeTokenWord tapeTokenKind = iota
	tapeTokenString
	tapeTokenRegex
)

type tapeToken struct {
	Kind  tapeTokenKind
	Value string
	Line  int
}

// tapeLexer holds the mutable scan position for tokenizeTape, so the
// dispatch switch in step() can stay a flat, single-branch-per-case
// function: each case delegates its own bookkeeping (index/line
// advancement, token append, error propagation) to a dedicated method
// instead of repeating it inline per case.
type tapeLexer struct {
	runes  []rune
	file   string
	i      int
	line   int
	tokens []tapeToken
}

// tokenizeTape scans source into a flat token stream. Whitespace-delimited
// bare words, "double-quoted" strings (backslash-escaped), `backtick strings`
// (raw, may span multiple physical lines), and /regex/ literals (delimited by
// a leading slash at a token boundary) are recognized; a line whose first
// non-whitespace character is '#' is a comment and is skipped to end of line.
func tokenizeTape(source, file string) ([]tapeToken, error) {
	lx := &tapeLexer{
		runes:  []rune(source),
		file:   file,
		line:   1,
		tokens: make([]tapeToken, 0, len(source)/tapeAvgTokenLen),
	}
	for lx.i < len(lx.runes) {
		if err := lx.step(); err != nil {
			return nil, err
		}
	}
	return lx.tokens, nil
}

func (lx *tapeLexer) step() error {
	r := lx.runes[lx.i]
	switch {
	case r == '\n':
		lx.line++
		lx.i++
	case unicode.IsSpace(r):
		lx.i++
	case r == '#':
		lx.skipComment()
	case r == '"':
		return lx.scanQuoted()
	case r == '`' || r == '\'':
		return lx.scanRaw(r)
	case r == '/' && lx.regexAllowed():
		return lx.scanRegexToken()
	default:
		lx.scanWordToken()
	}
	return nil
}

func (lx *tapeLexer) skipComment() {
	for lx.i < len(lx.runes) && lx.runes[lx.i] != '\n' {
		lx.i++
	}
}

func (lx *tapeLexer) scanQuoted() error {
	tok, next, err := scanTapeQuotedString(lx.runes, lx.i, lx.line, lx.file)
	if err != nil {
		return err
	}
	lx.tokens = append(lx.tokens, tok)
	lx.line += strings.Count(tok.Value, "\n")
	lx.i = next
	return nil
}

func (lx *tapeLexer) scanRaw(delim rune) error {
	result, err := scanTapeRawString(lx.runes, lx.i, lx.line, lx.file, delim)
	if err != nil {
		return err
	}
	lx.tokens = append(lx.tokens, result.Token)
	lx.line = result.EndLine
	lx.i = result.Next
	return nil
}

// regexAllowed reports whether a /regex/ literal is legal at the current
// lexer position, so a leading '/' on an absolute path (e.g. `Output
// /tmp/demo.gif`) scans as a bare word instead. Only Wait/Wait+Screen/
// Wait+Line's optional trailing pattern, and a Set directive's value, ever
// accept a regex token -- see consumeTapeWait and nextTapeSetValue in
// tape_parser.go, which is where a Set key's regex value is actually
// consumed; this lexer-level guard only needs to recognize the two token
// shapes that precede a legal regex start.
func (lx *tapeLexer) regexAllowed() bool {
	n := len(lx.tokens)
	if n == 0 {
		return false
	}
	switch lx.tokens[n-1].Value {
	case "Wait", "Wait+Screen", "Wait+Line":
		return true
	}
	return n >= 2 && lx.tokens[n-2].Value == "Set"
}

func (lx *tapeLexer) scanRegexToken() error {
	tok, next, err := scanTapeRegex(lx.runes, lx.i, lx.line, lx.file)
	if err != nil {
		return err
	}
	lx.tokens = append(lx.tokens, tok)
	lx.i = next
	return nil
}

func (lx *tapeLexer) scanWordToken() {
	tok, next := scanTapeWord(lx.runes, lx.i, lx.line)
	lx.tokens = append(lx.tokens, tok)
	lx.i = next
}

const tapeAvgTokenLen = 6

func scanTapeQuotedString(runes []rune, start, line int, file string) (tapeToken, int, error) {
	var sb strings.Builder
	i := start + 1
	for i < len(runes) {
		r := runes[i]
		switch r {
		case '"':
			return tapeToken{Kind: tapeTokenString, Value: sb.String(), Line: line}, i + 1, nil
		case '\n':
			return tapeToken{}, 0, tapeErr(file, line, fmt.Errorf("%w: unterminated quoted string", ErrTapeParseFailed))
		case '\\':
			if i+1 < len(runes) {
				sb.WriteRune(runes[i+1])
				i += 2
				continue
			}
			sb.WriteRune(r)
			i++
		default:
			sb.WriteRune(r)
			i++
		}
	}
	return tapeToken{}, 0, tapeErr(file, line, fmt.Errorf("%w: unterminated quoted string", ErrTapeParseFailed))
}

// tapeRawStringScan bundles scanTapeRawString's results (token, next scan
// position, and ending line) into one value so the function stays within
// revive's function-result-limit (max 3 return values).
type tapeRawStringScan struct {
	Token   tapeToken
	Next    int
	EndLine int
}

// scanTapeRawString scans a string delimited by delim (backtick or single
// quote) with no escape processing -- matching both VHS's backtick strings
// and its single-quoted strings (used to avoid escaping embedded double
// quotes, e.g. a Type command containing `"$out"`), mirroring shell
// single-quote semantics. May span multiple physical lines.
func scanTapeRawString(runes []rune, start, line int, file string, delim rune) (tapeRawStringScan, error) {
	var sb strings.Builder
	i := start + 1
	endLine := line
	for i < len(runes) {
		r := runes[i]
		if r == delim {
			return tapeRawStringScan{
				Token:   tapeToken{Kind: tapeTokenString, Value: sb.String(), Line: line},
				Next:    i + 1,
				EndLine: endLine,
			}, nil
		}
		if r == '\n' {
			endLine++
		}
		sb.WriteRune(r)
		i++
	}
	return tapeRawStringScan{}, tapeErr(file, line, fmt.Errorf("%w: unterminated %c string", ErrTapeParseFailed, delim))
}

func scanTapeRegex(runes []rune, start, line int, file string) (tapeToken, int, error) {
	var sb strings.Builder
	i := start + 1
	for i < len(runes) {
		r := runes[i]
		switch r {
		case '/':
			return tapeToken{Kind: tapeTokenRegex, Value: sb.String(), Line: line}, i + 1, nil
		case '\n':
			return tapeToken{}, 0, tapeErr(file, line, fmt.Errorf("%w: unterminated regex literal", ErrTapeParseFailed))
		default:
			sb.WriteRune(r)
			i++
		}
	}
	return tapeToken{}, 0, tapeErr(file, line, fmt.Errorf("%w: unterminated regex literal", ErrTapeParseFailed))
}

func scanTapeWord(runes []rune, start, line int) (tapeToken, int) {
	var sb strings.Builder
	i := start
	for i < len(runes) && !unicode.IsSpace(runes[i]) {
		sb.WriteRune(runes[i])
		i++
	}
	return tapeToken{Kind: tapeTokenWord, Value: sb.String(), Line: line}, i
}
