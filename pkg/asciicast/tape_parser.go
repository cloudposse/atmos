package asciicast

import (
	"fmt"
	"strconv"
	"strings"
)

// tapeKeypressKeywords are the VHS bare-keypress directive keywords with a
// direct pkg/asciicast session-action key name. Ctrl+<letter> is recognized
// dynamically (see tapeCtrlKey) since it is not a fixed keyword.
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
	"Type":       TapeType,
	"Copy":       TapeCopy,
}

func parseTapeDirective(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	tok := tokens[pos]
	if kind, ok := tapeSingleArgDirectiveKinds[tok.Value]; ok {
		return consumeTapeSingleArg(tokens, pos, file, kind)
	}
	switch tok.Value {
	case "Set":
		return consumeTapeSet(tokens, pos, file)
	case "Hide":
		return TapeDirective{Kind: TapeHide, File: file, Line: tok.Line}, pos + 1, nil
	case "Show":
		return TapeDirective{Kind: TapeShow, File: file, Line: tok.Line}, pos + 1, nil
	case "Paste":
		return TapeDirective{Kind: TapePaste, File: file, Line: tok.Line}, pos + 1, nil
	case "Sleep":
		return consumeTapeSleep(tokens, pos, file)
	case "Env":
		return consumeTapeEnv(tokens, pos, file)
	case "Wait", "Wait+Screen", "Wait+Line":
		return consumeTapeWait(tokens, pos, file)
	default:
		return parseTapeKeypressOrError(tokens, pos, file)
	}
}

func parseTapeKeypressOrError(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	tok := tokens[pos]
	if key, ok := tapeKeypressKeywords[tok.Value]; ok {
		return consumeTapeKey(tokens, pos, file, key)
	}
	if key, ok := tapeCtrlKey(tok.Value); ok {
		return consumeTapeKey(tokens, pos, file, key)
	}
	return TapeDirective{}, 0, tapeErr(file, tok.Line, fmt.Errorf("%w: %q", ErrUnsupportedTapeDirective, tok.Value))
}

// tapeCtrlKey recognizes VHS's "Ctrl+<Letter>" modifier-key spelling
// (case-sensitive, matching how every real tape in this repo writes it, e.g.
// "Ctrl+C") and normalizes it to Atmos's lowercase session-key spelling.
func tapeCtrlKey(word string) (string, bool) {
	const prefix = "Ctrl+"
	if !strings.HasPrefix(word, prefix) || len(word) != len(prefix)+1 {
		return "", false
	}
	return "ctrl+" + strings.ToLower(word[len(prefix):]), true
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

func consumeTapeWait(tokens []tapeToken, pos int, file string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	next := pos + 1
	regex := ""
	if next < len(tokens) && tokens[next].Kind == tapeTokenRegex {
		regex = tokens[next].Value
		next++
	}
	return TapeDirective{Kind: TapeWait, File: file, Line: line, Regex: regex}, next, nil
}

// consumeTapeKey parses a bare keypress directive, with an optional trailing
// integer repeat count (e.g. "Down 25"). VHS's optional "@<interval>" speed
// modifier is not recognized in v1 -- no real tape in this repo uses it, and
// the Sleep-directive idiom already covers pacing between repeated keys.
func consumeTapeKey(tokens []tapeToken, pos int, file, key string) (TapeDirective, int, error) {
	line := tokens[pos].Line
	next := pos + 1
	repeat := 0
	if next < len(tokens) && tokens[next].Kind == tapeTokenWord {
		if n, err := strconv.Atoi(tokens[next].Value); err == nil {
			repeat = n
			next++
		}
	}
	return TapeDirective{Kind: TapeKey, File: file, Line: line, Key: key, Repeat: repeat}, next, nil
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
