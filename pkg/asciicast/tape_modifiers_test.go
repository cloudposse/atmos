package asciicast

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTapeAtModifiers(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []TapeDirective
	}{
		{
			name:   "Type with per-character speed",
			source: `Type@50ms "x"`,
			want:   []TapeDirective{{Kind: TapeType, Value: "x", Speed: "50ms", Line: 1}},
		},
		{
			name:   "Type without modifier has no speed",
			source: `Type "x"`,
			want:   []TapeDirective{{Kind: TapeType, Value: "x", Line: 1}},
		},
		{
			name:   "keypress with interval and repeat count",
			source: "Enter@200ms 3",
			want:   []TapeDirective{{Kind: TapeKey, Key: "enter", Repeat: 3, Speed: "200ms", Line: 1}},
		},
		{
			name:   "Backspace with interval and repeat count",
			source: "Backspace@100ms 5",
			want:   []TapeDirective{{Kind: TapeKey, Key: "backspace", Repeat: 5, Speed: "100ms", Line: 1}},
		},
		{
			name:   "Ctrl key with interval",
			source: "Ctrl+C@1s",
			want:   []TapeDirective{{Kind: TapeKey, Key: "ctrl+c", Speed: "1s", Line: 1}},
		},
		{
			name:   "Wait timeout",
			source: "Wait@10s /re/",
			want:   []TapeDirective{{Kind: TapeWait, Regex: "re", Timeout: "10s", Line: 1}},
		},
		{
			name:   "Wait+Screen timeout",
			source: "Wait+Screen@10s /re/",
			want:   []TapeDirective{{Kind: TapeWait, Regex: "re", Timeout: "10s", Line: 1}},
		},
		{
			name:   "Wait+Line timeout",
			source: "Wait+Line@5s /re/",
			want:   []TapeDirective{{Kind: TapeWait, Regex: "re", Timeout: "5s", Line: 1}},
		},
		{
			name:   "modifier then following directive on one line",
			source: `Type@10ms "ls" Enter Wait@2s /done$/`,
			want: []TapeDirective{
				{Kind: TapeType, Value: "ls", Speed: "10ms", Line: 1},
				{Kind: TapeKey, Key: "enter", Line: 1},
				{Kind: TapeWait, Regex: "done$", Timeout: "2s", Line: 1},
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

func TestParseTapeAtModifierErrors(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantErr  error
		wantLine int
	}{
		{name: "unparseable duration on Type", source: `Type@fast "x"`, wantErr: ErrTapeParseFailed, wantLine: 1},
		{name: "empty duration", source: `Type@ "x"`, wantErr: ErrTapeParseFailed, wantLine: 1},
		{name: "negative duration", source: "Wait@-1s /x/", wantErr: ErrTapeParseFailed, wantLine: 1},
		{name: "bad duration reports its own line", source: "Sleep 1s\nEnter@bogus", wantErr: ErrTapeParseFailed, wantLine: 2},
		{name: "unknown keyword with modifier", source: "Frobnicate@1s", wantErr: ErrUnsupportedTapeDirective, wantLine: 1},
		{name: "known non-key directive rejects a modifier", source: "Sleep@1s 2s", wantErr: ErrUnsupportedTapeDirective, wantLine: 1},
		{name: "Output rejects a modifier", source: "Output@1s a.gif", wantErr: ErrUnsupportedTapeDirective, wantLine: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTape(tt.source, "test.tape", ".", fakeTapeFileReader{})
			require.ErrorIs(t, err, tt.wantErr)
			var tapeErr *TapeError
			require.ErrorAs(t, err, &tapeErr)
			assert.Equal(t, tt.wantLine, tapeErr.Line)
		})
	}
}

func TestParseTapeExtendedKeys(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"Delete", "delete"},
		{"Insert", "insert"},
		{"Home", "home"},
		{"End", "end"},
		{"Alt+B", "alt+b"},
		{"Alt+b", "alt+b"},
		{"Alt+.", "alt+."},
		{"Alt+1", "alt+1"},
		{"Shift+Tab", "shift+tab"},
		{"Shift+A", "shift+a"},
		{"Shift+z", "shift+z"},
		{"Ctrl+Alt+C", "ctrl+alt+c"},
		{"Ctrl+Alt+d", "ctrl+alt+d"},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			tape, err := ParseTape(tt.source+" 2", "test.tape", ".", fakeTapeFileReader{})
			require.NoError(t, err)
			require.Len(t, tape.Directives, 1)
			assert.Equal(t, TapeKey, tape.Directives[0].Kind)
			assert.Equal(t, tt.want, tape.Directives[0].Key)
			assert.Equal(t, 2, tape.Directives[0].Repeat)
		})
	}
}

func TestParseTapeUnsupportedKeyCombinationsListSupportedForms(t *testing.T) {
	for _, source := range []string{"Shift+Enter", "Shift+1", "Alt+Enter", "Ctrl+Shift+A", "Ctrl+Alt+1", "Meta+A", "Ctrl+Alt+Shift+A", "Alt+"} {
		t.Run(source, func(t *testing.T) {
			_, err := ParseTape(source, "test.tape", ".", fakeTapeFileReader{})
			require.ErrorIs(t, err, ErrUnsupportedTapeDirective)
			assert.Contains(t, err.Error(), "Shift+Tab", "the error must list the supported keypress forms")
			assert.Contains(t, err.Error(), source)
		})
	}
}

// TestParseTapeQuotedStringKeepsBackslashesVerbatim is the end-to-end form of
// the lexer rule: a double-quoted Type argument reaches the directive
// byte-for-byte.
func TestParseTapeQuotedStringKeepsBackslashesVerbatim(t *testing.T) {
	tape, err := ParseTape(`Type "printf 'a\nb'" Enter`, "test.tape", ".", fakeTapeFileReader{})
	require.NoError(t, err)
	require.Len(t, tape.Directives, 2)
	assert.Equal(t, `printf 'a\nb'`, tape.Directives[0].Value)
}
