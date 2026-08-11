package asciicast

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTapeFileReader is an in-memory TapeFileReader for Source-inlining tests
// that must not touch real disk.
type fakeTapeFileReader map[string]string

func (f fakeTapeFileReader) ReadFile(path string) ([]byte, error) {
	content, ok := f[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(content), nil
}

// osTapeFileReader reads real files, used only for the legacy-demo.tape
// fixture smoke test below.
type osTapeFileReader struct{}

func (osTapeFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func TestTokenizeTape(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    []tapeToken
		wantErr bool
	}{
		{
			name:   "bare words",
			source: "Set Shell bash",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Set", Line: 1},
				{Kind: tapeTokenWord, Value: "Shell", Line: 1},
				{Kind: tapeTokenWord, Value: "bash", Line: 1},
			},
		},
		{
			name:   "double quoted string",
			source: `Type "atmos list stacks"`,
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: "atmos list stacks", Line: 1},
			},
		},
		{
			name:   "backtick string with embedded quotes",
			source: "Type `export PS1='> '`",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: "export PS1='> '", Line: 1},
			},
		},
		{
			// Real VHS tapes use single quotes to avoid escaping embedded
			// double quotes, e.g. demo/landing/dx.tape's
			// `Type '... || echo "$out" | grep ...' Enter`.
			name:   "single quoted string with embedded double quotes",
			source: `Type 'echo "$out" | grep -qi done'`,
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: `echo "$out" | grep -qi done`, Line: 1},
			},
		},
		{
			name:   "regex literal",
			source: "Wait /❯$/",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Wait", Line: 1},
				{Kind: tapeTokenRegex, Value: "❯$", Line: 1},
			},
		},
		{
			name:   "path containing slashes is a bare word, not a regex",
			source: "Output docs/demo.gif",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Output", Line: 1},
				{Kind: tapeTokenWord, Value: "docs/demo.gif", Line: 1},
			},
		},
		{
			name:   "absolute path is a bare word, not a regex",
			source: "Output /tmp/demo.gif",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Output", Line: 1},
				{Kind: tapeTokenWord, Value: "/tmp/demo.gif", Line: 1},
			},
		},
		{
			name:   "trailing comment after a directive is skipped",
			source: `Type "x" # set up the demo`,
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: "x", Line: 1},
			},
		},
		{
			name:   "whole-line comment is skipped",
			source: "# a comment\nSleep 500ms",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Sleep", Line: 2},
				{Kind: tapeTokenWord, Value: "500ms", Line: 2},
			},
		},
		{
			name:   "leading-hash comment with no space is skipped",
			source: "#Set FontFamily \"Hack Nerd Font\"\nSet Shell bash",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Set", Line: 2},
				{Kind: tapeTokenWord, Value: "Shell", Line: 2},
				{Kind: tapeTokenWord, Value: "bash", Line: 2},
			},
		},
		{
			name:   "hash inside quotes is not a comment",
			source: `Type "# First check you have Atmos installed"`,
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: "# First check you have Atmos installed", Line: 1},
			},
		},
		{
			name:   "backtick string spans multiple lines",
			source: "Type `line one\nline two`\nSleep 1s",
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: "line one\nline two", Line: 1},
				{Kind: tapeTokenWord, Value: "Sleep", Line: 3},
				{Kind: tapeTokenWord, Value: "1s", Line: 3},
			},
		},
		{
			name:    "unterminated quoted string errors",
			source:  `Type "unterminated`,
			wantErr: true,
		},
		{
			name:    "quoted string may not span a raw newline",
			source:  "Type \"line one\nline two\"",
			wantErr: true,
		},
		{
			name:    "unterminated regex errors",
			source:  "Wait /never closed",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tokenizeTape(tt.source, "test.tape")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTapeDirectives(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []TapeDirective
	}{
		{
			name:   "Set with quoted multi-word value",
			source: `Set Theme "Monokai Vivid"`,
			want:   []TapeDirective{{Kind: TapeSet, Key: "Theme", Value: "Monokai Vivid", Line: 1}},
		},
		{
			name:   "Set with bare word value",
			source: "Set Width 1400",
			want:   []TapeDirective{{Kind: TapeSet, Key: "Width", Value: "1400", Line: 1}},
		},
		{
			// Real VHS tapes set WaitPattern to a /regex/ literal, e.g.
			// demo/landing/defaults.tape's `Set WaitPattern /❯$/`.
			name:   "Set with regex value",
			source: "Set WaitPattern /❯$/",
			want:   []TapeDirective{{Kind: TapeSet, Key: "WaitPattern", Value: "/❯$/", Line: 1}},
		},
		{
			name:   "Output",
			source: "Output website/static/img/demos/hero.mp4",
			want:   []TapeDirective{{Kind: TapeOutput, Value: "website/static/img/demos/hero.mp4", Line: 1}},
		},
		{
			name:   "Require",
			source: "Require atmos",
			want:   []TapeDirective{{Kind: TapeRequire, Value: "atmos", Line: 1}},
		},
		{
			name:   "Hide and Show",
			source: "Hide\nShow",
			want: []TapeDirective{
				{Kind: TapeHide, Line: 1},
				{Kind: TapeShow, Line: 2},
			},
		},
		{
			name:   "Type Sleep Enter chain on one line",
			source: `Type "atmos list stacks" Sleep 600ms Enter`,
			want: []TapeDirective{
				{Kind: TapeType, Value: "atmos list stacks", Line: 1},
				{Kind: TapeSleep, Duration: "600ms", Line: 1},
				{Kind: TapeKey, Key: "enter", Line: 1},
			},
		},
		{
			name:   "keypress with repeat count",
			source: "Down 25",
			want:   []TapeDirective{{Kind: TapeKey, Key: "down", Repeat: 25, Line: 1}},
		},
		{
			name:   "keypress without repeat count followed by Sleep",
			source: "Down Sleep 500ms",
			want: []TapeDirective{
				{Kind: TapeKey, Key: "down", Line: 1},
				{Kind: TapeSleep, Duration: "500ms", Line: 1},
			},
		},
		{
			name:   "Ctrl+ modifier key",
			source: "Ctrl+C",
			want:   []TapeDirective{{Kind: TapeKey, Key: "ctrl+c", Line: 1}},
		},
		{
			name:   "Type with no trailing Enter",
			source: `Type "q"`,
			want:   []TapeDirective{{Kind: TapeType, Value: "q", Line: 1}},
		},
		{
			name:   "Wait with regex",
			source: "Wait /❯$/",
			want:   []TapeDirective{{Kind: TapeWait, Regex: "❯$", Line: 1}},
		},
		{
			name:   "Wait+Screen and Wait+Line normalize to Wait",
			source: "Wait+Screen /a/\nWait+Line /b/",
			want: []TapeDirective{
				{Kind: TapeWait, Regex: "a", Line: 1},
				{Kind: TapeWait, Regex: "b", Line: 2},
			},
		},
		{
			name:   "bare Wait with no regex",
			source: "Wait\nSleep 1s",
			want: []TapeDirective{
				{Kind: TapeWait, Line: 1},
				{Kind: TapeSleep, Duration: "1s", Line: 2},
			},
		},
		{
			name:   "Screenshot",
			source: "Screenshot website/static/img/demos/hero.png",
			want:   []TapeDirective{{Kind: TapeScreenshot, Value: "website/static/img/demos/hero.png", Line: 1}},
		},
		{
			name:   "Copy Paste Env are recognized (translator rejects them, not the parser)",
			source: `Copy "text"` + "\nPaste\nEnv KEY value",
			want: []TapeDirective{
				{Kind: TapeCopy, Value: "text", Line: 1},
				{Kind: TapePaste, Line: 2},
				{Kind: TapeEnv, Key: "KEY", Value: "value", Line: 3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tape, err := ParseTape(tt.source, "test.tape", ".", fakeTapeFileReader{})
			require.NoError(t, err)
			for i := range tt.want {
				tt.want[i].File = "test.tape"
			}
			assert.Equal(t, tt.want, tape.Directives)
		})
	}
}

// TestParseTapeSourceKeywordSyntax exercises the Source keyword's own
// grammar in isolation, at the pre-splicing token/parse layer -- ParseTape's
// public entry point always resolves and inlines Source (see
// TestParseTapeSourceInlining), so this checks parseTapeTokens directly.
func TestParseTapeSourceKeywordSyntax(t *testing.T) {
	tokens, err := tokenizeTape("Source demo/landing/defaults.tape", "test.tape")
	require.NoError(t, err)
	directives, err := parseTapeTokens(tokens, "test.tape")
	require.NoError(t, err)
	assert.Equal(t, []TapeDirective{
		{Kind: TapeSource, Value: "demo/landing/defaults.tape", Line: 1, File: "test.tape"},
	}, directives)
}

func TestParseTapeUnsupportedDirectiveError(t *testing.T) {
	_, err := ParseTape("Frobnicate widgets", "test.tape", ".", fakeTapeFileReader{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedTapeDirective))
	var tapeErr *TapeError
	require.ErrorAs(t, err, &tapeErr)
	assert.Equal(t, 1, tapeErr.Line)
}

// TestParseTapeCtrlKeyRejectsNonLetterSuffix ensures "Ctrl+<X>" only accepts
// the documented Ctrl+<Letter> grammar -- a digit or symbol suffix (e.g.
// "Ctrl+1", "Ctrl++") isn't a real Ctrl-modified key and must be rejected as
// an unsupported directive rather than silently becoming an invalid session
// key.
func TestParseTapeCtrlKeyRejectsNonLetterSuffix(t *testing.T) {
	for _, suffix := range []string{"Ctrl+1", "Ctrl++", "Ctrl+!", "Ctrl+."} {
		t.Run(suffix, func(t *testing.T) {
			_, err := ParseTape(suffix, "test.tape", ".", fakeTapeFileReader{})
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnsupportedTapeDirective))
		})
	}
}

func TestParseTapeSourceInlining(t *testing.T) {
	fr := fakeTapeFileReader{
		"defaults.tape": "Set Shell bash\nSet WaitTimeout 300s",
		"hero.tape":     "Source defaults.tape\nType \"atmos list stacks\" Enter",
	}
	tape, err := ParseTape(fr["hero.tape"], "hero.tape", ".", fr)
	require.NoError(t, err)
	require.Len(t, tape.Directives, 4)
	assert.Equal(t, TapeSet, tape.Directives[0].Kind)
	assert.Equal(t, "Shell", tape.Directives[0].Key)
	assert.Equal(t, "defaults.tape", tape.Directives[0].File)
	assert.Equal(t, TapeSet, tape.Directives[1].Kind)
	assert.Equal(t, "WaitTimeout", tape.Directives[1].Key)
	assert.Equal(t, TapeType, tape.Directives[2].Kind)
	assert.Equal(t, TapeKey, tape.Directives[3].Kind)
}

// TestParseTapeFileSourceResolvesRelativeToBaseDirNotReferencingFile
// reproduces a real bug found by running the interpreter against every
// .tape file in this repo: demo/landing/hero.tape (and every other
// demo/landing/*.tape) writes `Source demo/landing/defaults.tape` -- a path
// relative to the repo root vhs is invoked from (demo/landing/atmos.yaml's
// record command does `cd "$repo"` before running `vhs`), NOT relative to
// hero.tape's own directory (demo/landing/). Resolving Source relative to
// the referencing file's directory would look for the nonexistent
// demo/landing/demo/landing/defaults.tape.
func TestParseTapeFileSourceResolvesRelativeToBaseDirNotReferencingFile(t *testing.T) {
	fr := fakeTapeFileReader{
		"demo/landing/defaults.tape": "Set Shell bash",
		"demo/landing/hero.tape":     "Source demo/landing/defaults.tape\nType \"atmos list stacks\" Enter",
	}
	// baseDir "." simulates vhs's own invocation CWD (the repo root), the
	// same base demo/landing/hero.tape's own path is given relative to --
	// not filepath.Dir("demo/landing/hero.tape").
	tape, err := ParseTapeFile("demo/landing/hero.tape", ".", fr)
	require.NoError(t, err)
	require.Len(t, tape.Directives, 3)
	assert.Equal(t, TapeSet, tape.Directives[0].Kind)
	assert.Equal(t, TapeType, tape.Directives[1].Kind)
	assert.Equal(t, TapeKey, tape.Directives[2].Kind)
}

// TestParseTapeFileMissingFileWrapsErrTapeSourceNotFoundWithPath ensures a
// missing top-level tape reports the same matchable sentinel and path
// context as a missing Source target (loadSource), instead of the bare
// underlying "file does not exist" error.
func TestParseTapeFileMissingFileWrapsErrTapeSourceNotFoundWithPath(t *testing.T) {
	fr := fakeTapeFileReader{}
	_, err := ParseTapeFile("demo/landing/missing.tape", ".", fr)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTapeSourceNotFound)
	assert.Contains(t, err.Error(), "demo/landing/missing.tape")
}

// TestParseTapeSourceBaseDirStaysFixedAcrossNestedSources proves the base
// directory does not get rebased to each Sourced file's own directory as
// Source chains recurse -- it stays anchored to the single base the
// top-level parse started with, matching real VHS's single-CWD model.
func TestParseTapeSourceBaseDirStaysFixedAcrossNestedSources(t *testing.T) {
	fr := fakeTapeFileReader{
		"shared/colors.tape":  "Set Theme dark",
		"shared/base.tape":    "Source shared/colors.tape\nSet Shell bash",
		"demo/landing/a.tape": "Source shared/base.tape\nSleep 1s",
	}
	tape, err := ParseTapeFile("demo/landing/a.tape", ".", fr)
	require.NoError(t, err)
	require.Len(t, tape.Directives, 3)
	assert.Equal(t, "Theme", tape.Directives[0].Key)
	assert.Equal(t, "Shell", tape.Directives[1].Key)
	assert.Equal(t, TapeSleep, tape.Directives[2].Kind)
}

func TestParseTapeSourceCycleDetection(t *testing.T) {
	fr := fakeTapeFileReader{
		"a.tape": "Source b.tape",
		"b.tape": "Source a.tape",
	}
	_, err := ParseTape(fr["a.tape"], "a.tape", ".", fr)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTapeSourceCycle))
}

func TestParseTapeSourceNotFound(t *testing.T) {
	_, err := ParseTape("Source missing.tape", "hero.tape", ".", fakeTapeFileReader{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTapeSourceNotFound))
}

// TestParseTapeHeroSnippet parses a trimmed, inline reproduction of
// demo/landing/hero.tape's real content end to end, asserting the exact
// resulting directive sequence (including Source inlining of a
// defaults.tape-like shared setup file).
func TestParseTapeHeroSnippet(t *testing.T) {
	fr := fakeTapeFileReader{
		"defaults.tape": `Set Shell bash
Set TypingSpeed 35ms
Set Width 1280
Set Height 640
Set WaitTimeout 300s`,
		"hero.tape": "Source defaults.tape\n" +
			"Output website/static/img/demos/hero.mp4\n" +
			"Hide\n" +
			`Type "unset NO_COLOR ATMOS_NO_COLOR" Enter` + "\n" +
			"Type `cd \"$(git rev-parse --show-toplevel)/demo/landing/fixtures/hero\"` Enter\n" +
			"Type \"clear\" Enter\n" +
			"Wait /❯$/\n" +
			"Show\n" +
			"Sleep 800ms\n" +
			`Type "atmos list stacks" Sleep 600ms Enter` + "\n" +
			"Wait /❯$/\n" +
			"Screenshot website/static/img/demos/hero.png\n" +
			"Sleep 5s\n",
	}
	tape, err := ParseTape(fr["hero.tape"], "hero.tape", ".", fr)
	require.NoError(t, err)

	var kinds []TapeDirectiveKind
	for _, d := range tape.Directives {
		kinds = append(kinds, d.Kind)
	}
	assert.Equal(t, []TapeDirectiveKind{
		TapeSet, TapeSet, TapeSet, TapeSet, TapeSet, // spliced defaults.tape
		TapeOutput,
		TapeHide,
		TapeType, TapeKey, // unset ... Enter
		TapeType, TapeKey, // cd ... Enter
		TapeType, TapeKey, // clear Enter
		TapeWait,
		TapeShow,
		TapeSleep,
		TapeType, TapeSleep, TapeKey, // atmos list stacks / Sleep 600ms / Enter
		TapeWait,
		TapeScreenshot,
		TapeSleep,
	}, kinds)
}

// TestParseTapeLegacyDemoFixture parses the restored root-level demo.tape
// (see pkg/asciicast/testdata/legacy-demo.tape) end to end, exercising
// multi-word quoted Set values, a hex-color Set value, a .gif Output,
// repeat-count keypresses, and a Type with no trailing Enter -- none of which
// appear in demo/landing/hero.tape.
func TestParseTapeLegacyDemoFixture(t *testing.T) {
	tape, err := ParseTapeFile("testdata/legacy-demo.tape", "testdata", osTapeFileReader{})
	require.NoError(t, err)
	require.NotEmpty(t, tape.Directives)

	var (
		sawThemeSet         bool
		sawHexColorSet      bool
		sawGifOutput        bool
		sawRepeatKeypress   bool
		sawTypeWithoutEnter bool
	)
	for i, d := range tape.Directives {
		switch {
		case d.Kind == TapeSet && d.Key == "Theme" && d.Value == "Monokai Vivid":
			sawThemeSet = true
		case d.Kind == TapeSet && d.Key == "MarginFill" && d.Value == "#674EFF":
			sawHexColorSet = true
		case d.Kind == TapeOutput && d.Value == "docs/demo.gif":
			sawGifOutput = true
		case d.Kind == TapeKey && d.Repeat == 25:
			sawRepeatKeypress = true
		case d.Kind == TapeType && d.Value == "q":
			nextIsEnter := i+1 < len(tape.Directives) && tape.Directives[i+1].Kind == TapeKey && tape.Directives[i+1].Key == "enter"
			if !nextIsEnter {
				sawTypeWithoutEnter = true
			}
		}
	}
	assert.True(t, sawThemeSet, "expected Set Theme directive")
	assert.True(t, sawHexColorSet, "expected Set MarginFill hex-color directive")
	assert.True(t, sawGifOutput, "expected Output docs/demo.gif directive")
	assert.True(t, sawRepeatKeypress, "expected a repeat-count keypress directive")
	assert.True(t, sawTypeWithoutEnter, "expected a Type with no trailing Enter")
}
