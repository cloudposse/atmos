package asciicast

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	durationpkg "github.com/cloudposse/atmos/pkg/duration"
)

// tapeModifierSeparator splits a directive keyword from its VHS "@<duration>"
// modifier, e.g. "Type@50ms" or "Wait+Screen@10s".
const tapeModifierSeparator = "@"

// tapeSupportedKeypressForms documents the keypress spellings the parser
// accepts; it is quoted in the error for an unrecognized modifier combination.
const tapeSupportedKeypressForms = "Enter, Space, Tab, Backspace, Delete, Insert, Escape, Home, End, Up, Down, Left, Right, PageUp, PageDown, " +
	"Ctrl+<letter>, Alt+<character>, Shift+Tab, Shift+<letter>, Ctrl+Alt+<letter>"

// tapeKeypressKeywords are the VHS bare-keypress directive keywords with a
// direct pkg/asciicast session-action key name. Ctrl+<letter> is recognized
// dynamically (see tapeModifierKey) since it is not a fixed keyword.
var tapeKeypressKeywords = map[string]string{
	"Enter":     "enter",
	"Space":     "space",
	"Tab":       "tab",
	"Up":        "up",
	"Down":      "down",
	"Left":      "left",
	"Right":     "right",
	"Backspace": "backspace",
	"Escape":    "escape",
	"PageUp":    "pageup",
	"PageDown":  "pagedown",
	"Delete":    "delete",
	"Insert":    "insert",
	"Home":      "home",
	"End":       "end",
}

// parseTapeTokens consumes a flat token stream into an ordered directive
// list. VHS's grammar is keyword-delimited rather than line-delimited (a
// single physical line commonly chains several directives, e.g.
// `Type "cmd" Sleep 500ms Enter`), so the parser walks the token stream
// directive-by-directive without regard to line boundaries; each directive's
// own consume function reads exactly the tokens it needs.
func parseTapeTokens(tokens []tapeToken, file string) ([]TapeDirective, error) {
	var directives []TapeDirective
	pos := 0
	for pos < len(tokens) {
		tok := tokens[pos]
		if tok.Kind != tapeTokenWord {
			return nil, tapeErr(file, tok.Line, fmt.Errorf("%w: unexpected token %q", ErrTapeParseFailed, tok.Value))
		}
		directive, next, err := parseTapeDirective(tokens, pos, file)
		if err != nil {
			return nil, err
		}
		directives = append(directives, directive)
		pos = next
	}
	return directives, nil
}

// tapeSingleArgDirectiveKinds are the VHS keywords whose directive is a bare
// "Keyword <arg>" shape, all parsed identically by consumeTapeSingleArg;
// pulling them into a lookup keeps parseTapeDirective's switch limited to
// the directives that each need their own consume function.
var tapeSingleArgDirectiveKinds = map[string]TapeDirectiveKind{
	"Output":     TapeOutput,
	"Require":    TapeRequire,
	"Source":     TapeSource,
	"Screenshot": TapeScreenshot,
	"Copy":       TapeCopy,
}

// tapeBareDirectiveKinds are the VHS keywords that take no arguments, so the
// directive is complete once the keyword token is consumed.
var tapeBareDirectiveKinds = map[string]TapeDirectiveKind{
	"Hide":  TapeHide,
	"Show":  TapeShow,
	"Paste": TapePaste,
}

func parseTapeDirective(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	tok := tokens[pos]
	keyword, modifier, err := splitTapeKeyword(tok, file)
	if err != nil {
		return TapeDirective{}, 0, err
	}
	if kind, ok := tapeSingleArgDirectiveKinds[keyword]; ok {
		return consumeTapeSingleArg(tokens, pos, file, kind)
	}
	if kind, ok := tapeBareDirectiveKinds[keyword]; ok {
		return TapeDirective{Kind: kind, File: file, Line: tok.Line}, pos + 1, nil
	}
	switch keyword {
	case "Type":
		return consumeTapeType(tokens, pos, file, modifier)
	case "Set":
		return consumeTapeSet(tokens, pos, file)
	case "Sleep":
		return consumeTapeSleep(tokens, pos, file)
	case "Env":
		return consumeTapeEnv(tokens, pos, file)
	case "Wait", "Wait+Screen", "Wait+Line":
		return consumeTapeWait(tokens, pos, file, modifier)
	default:
		return parseTapeKeypressOrError(tokens, pos, file, keyword, modifier)
	}
}

// splitTapeKeyword splits a keyword@modifier token into its keyword and
// modifier, validating the modifier when one is present.
func splitTapeKeyword(tok tapeToken, file string) (keyword, modifier string, err error) {
	keyword, modifier, hasModifier := strings.Cut(tok.Value, tapeModifierSeparator)
	if hasModifier {
		if err := validateTapeModifier(tok, keyword, modifier, file); err != nil {
			return "", "", err
		}
	}
	return keyword, modifier, nil
}

// validateTapeModifier checks a keyword@modifier token: only Type, the Wait
// family, and keypresses accept a modifier, and the modifier must be a valid
// non-negative duration.
func validateTapeModifier(tok tapeToken, keyword, modifier, file string) error {
	if !tapeKeywordAcceptsModifier(keyword) {
		return tapeErr(file, tok.Line, fmt.Errorf("%w: %q", ErrUnsupportedTapeDirective, tok.Value))
	}
	if _, err := durationpkg.ParseNonNegative(modifier); err != nil {
		return tapeErr(file, tok.Line, fmt.Errorf("%w: invalid %q modifier %q on %q: %w", ErrTapeParseFailed, tapeModifierSeparator, modifier, keyword, err))
	}
	return nil
}

func tapeKeywordAcceptsModifier(keyword string) bool {
	switch keyword {
	case "Type", "Wait", "Wait+Screen", "Wait+Line":
		return true
	}
	_, ok := tapeKeyName(keyword)
	return ok
}

// tapeKeyName resolves a keypress keyword (fixed keyword or modifier-key
// combination) to its Atmos session-action key name.
func tapeKeyName(keyword string) (string, bool) {
	if key, ok := tapeKeypressKeywords[keyword]; ok {
		return key, true
	}
	return tapeModifierKey(keyword)
}

func parseTapeKeypressOrError(tokens []tapeToken, pos int, file, keyword, modifier string) (TapeDirective, int, error) {
	tok := tokens[pos]
	if key, ok := tapeKeyName(keyword); ok {
		return consumeTapeKey(tokens, pos, file, key, modifier)
	}
	if strings.Contains(keyword, "+") {
		return TapeDirective{}, 0, tapeErr(file, tok.Line, fmt.Errorf("%w: %q (supported keypresses: %s)", ErrUnsupportedTapeDirective, tok.Value, tapeSupportedKeypressForms))
	}
	return TapeDirective{}, 0, tapeErr(file, tok.Line, fmt.Errorf("%w: %q", ErrUnsupportedTapeDirective, tok.Value))
}

// tapeModifierKey recognizes VHS's modifier-key spellings (case-sensitive
// modifier names, matching how every real tape in this repo writes them, e.g.
// "Ctrl+C") and normalizes them to Atmos's lowercase session-key spelling:
// Ctrl+<letter>, Ctrl+Alt+<letter>, Alt+<character>, Shift+Tab and
// Shift+<letter>.
func tapeModifierKey(word string) (string, bool) {
	modifier, rest, ok := strings.Cut(word, "+")
	if !ok {
		return "", false
	}
	switch modifier {
	case "Ctrl":
		if alt, ok := strings.CutPrefix(rest, "Alt+"); ok {
			return tapeLetterKey("ctrl+alt+", alt)
		}
		return tapeLetterKey("ctrl+", rest)
	case "Alt":
		return tapeAltKey(rest)
	case "Shift":
		if rest == "Tab" {
			return "shift+tab", true
		}
		return tapeLetterKey("shift+", rest)
	}
	return "", false
}

// tapeLetterKey builds prefix + lowercase letter when letter is exactly one
// ASCII letter.
func tapeLetterKey(prefix, letter string) (string, bool) {
	if len(letter) != 1 {
		return "", false
	}
	c := letter[0]
	if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
		return "", false
	}
	return prefix + strings.ToLower(letter), true
}

// tapeAltKey accepts Alt+<single printable character>; letters are lowercased
// because VHS sends the unshifted key with the Alt modifier.
func tapeAltKey(char string) (string, bool) {
	r, size := utf8.DecodeRuneInString(char)
	if r == utf8.RuneError || size != len(char) || !unicode.IsPrint(r) || unicode.IsSpace(r) {
		return "", false
	}
	return "alt+" + strings.ToLower(char), true
}

func consumeTapeSet(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	key, next, err := nextTapeArg(tokens, pos+1, file, "Set key")
	if err != nil {
		return TapeDirective{}, 0, err
	}
	value, next, err := nextTapeSetValue(tokens, next, file)
	if err != nil {
		return TapeDirective{}, 0, err
	}
	return TapeDirective{Kind: TapeSet, File: file, Line: line, Key: key, Value: value}, next, nil
}

// nextTapeSetValue reads a Set directive's value, which -- unlike every other
// directive's arguments -- may legitimately be a /regex/ literal (e.g.
// `Set WaitPattern /prompt$/`). The regex's raw pattern is rewrapped in
// slashes so the stored Value round-trips the same way a quoted string would.
func nextTapeSetValue(tokens []tapeToken, pos int, file string) (string, int, error) {
	if pos < len(tokens) && tokens[pos].Kind == tapeTokenRegex {
		return "/" + tokens[pos].Value + "/", pos + 1, nil
	}
	return nextTapeArg(tokens, pos, file, "Set value")
}

func consumeTapeSingleArg(tokens []tapeToken, pos int, file string, kind TapeDirectiveKind) (TapeDirective, int, error) {
	line := tokens[pos].Line
	value, next, err := nextTapeArg(tokens, pos+1, file, "argument")
	if err != nil {
		return TapeDirective{}, 0, err
	}
	return TapeDirective{Kind: kind, File: file, Line: line, Value: value}, next, nil
}

// consumeTapeType parses Type's text argument plus the optional "@<duration>"
// per-character typing delay.
func consumeTapeType(tokens []tapeToken, pos int, file, speed string) (TapeDirective, int, error) {
	directive, next, err := consumeTapeSingleArg(tokens, pos, file, TapeType)
	if err != nil {
		return TapeDirective{}, 0, err
	}
	directive.Speed = speed
	return directive, next, nil
}

func consumeTapeSleep(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	duration, next, err := nextTapeArg(tokens, pos+1, file, "Sleep duration")
	if err != nil {
		return TapeDirective{}, 0, err
	}
	return TapeDirective{Kind: TapeSleep, File: file, Line: line, Duration: duration}, next, nil
}

func consumeTapeEnv(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	key, next, err := nextTapeArg(tokens, pos+1, file, "Env key")
	if err != nil {
		return TapeDirective{}, 0, err
	}
	value, next, err := nextTapeArg(tokens, next, file, "Env value")
	if err != nil {
		return TapeDirective{}, 0, err
	}
	return TapeDirective{Kind: TapeEnv, File: file, Line: line, Key: key, Value: value}, next, nil
}

// consumeTapeWait parses Wait, Wait+Screen and Wait+Line with an optional
// "@<duration>" timeout modifier (e.g. "Wait+Screen@10s /ready/").
func consumeTapeWait(tokens []tapeToken, pos int, file, timeout string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	next := pos + 1
	regex := ""
	if next < len(tokens) && tokens[next].Kind == tapeTokenRegex {
		regex = tokens[next].Value
		next++
	}
	return TapeDirective{Kind: TapeWait, File: file, Line: line, Regex: regex, Timeout: timeout}, next, nil
}

// consumeTapeKey parses a bare keypress directive, with an optional trailing
// integer repeat count (e.g. "Down 25") and an optional "@<duration>" modifier
// on the keyword (e.g. "Enter@200ms 3") that sets the delay between repeats.
func consumeTapeKey(tokens []tapeToken, pos int, file, key, speed string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	next := pos + 1
	repeat := 0
	if next < len(tokens) && tokens[next].Kind == tapeTokenWord {
		if n, err := strconv.Atoi(tokens[next].Value); err == nil {
			repeat = n
			next++
		}
	}
	return TapeDirective{Kind: TapeKey, File: file, Line: line, Key: key, Repeat: repeat, Speed: speed}, next, nil
}

// nextTapeArg returns the value of the token at pos as a directive argument;
// it accepts both bare-word and quoted/backtick-string tokens (VHS allows
// either for most arguments), but rejects a bare /regex/ literal or running
// out of tokens.
func nextTapeArg(tokens []tapeToken, pos int, file, what string) (string, int, error) {
	if pos >= len(tokens) {
		line := 0
		if pos > 0 {
			line = tokens[pos-1].Line
		}
		return "", 0, tapeErr(file, line, fmt.Errorf("%w: missing %s", ErrTapeParseFailed, what))
	}
	tok := tokens[pos]
	if tok.Kind == tapeTokenRegex {
		return "", 0, tapeErr(file, tok.Line, fmt.Errorf("%w: unexpected regex literal for %s", ErrTapeParseFailed, what))
	}
	return tok.Value, pos + 1, nil
}
