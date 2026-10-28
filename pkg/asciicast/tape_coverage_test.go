package asciicast

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTapeErrorMessageIncludesLocation(t *testing.T) {
	inner := errors.New("boom")
	tests := []struct {
		name string
		err  *TapeError
		want string
	}{
		{name: "with file", err: &TapeError{File: "demo.tape", Line: 7, Err: inner}, want: "demo.tape:7: boom"},
		{name: "inline tape without file", err: &TapeError{Line: 3, Err: inner}, want: "tape line 3: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
			assert.True(t, errors.Is(tt.err, inner), "TapeError must unwrap to its cause")
		})
	}
}

func TestTokenizeTapeQuotedStringsAreVerbatimAndRegexBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    []tapeToken
		wantErr bool
	}{
		{
			// VHS reads a double-quoted string to the closing quote verbatim: \n stays a
			// backslash and an n, so a shell command keeps its own escapes.
			name:   "backslash sequences in a quoted string are preserved byte for byte",
			source: `Type "printf 'a\nb' \\ \t"`,
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: `printf 'a\nb' \\ \t`, Line: 1},
			},
		},
		{
			// A backslash does not escape the closing quote, matching VHS.
			name:   "backslash before the closing quote does not escape it",
			source: `Type "C:\dir\" Enter`,
			want: []tapeToken{
				{Kind: tapeTokenWord, Value: "Type", Line: 1},
				{Kind: tapeTokenString, Value: `C:\dir\`, Line: 1},
				{Kind: tapeTokenWord, Value: "Enter", Line: 1},
			},
		},
		{
			name:    "trailing backslash leaves the quoted string unterminated",
			source:  `Type "abc\`,
			wantErr: true,
		},
		{
			name:    "unterminated backtick string errors",
			source:  "Type `never closed",
			wantErr: true,
		},
		{
			name:    "unterminated single quoted string errors",
			source:  "Type 'never closed",
			wantErr: true,
		},
		{
			name:    "regex may not span a newline",
			source:  "Wait /abc\ndef/",
			wantErr: true,
		},
		{
			name:   "leading slash with no preceding token is a bare word, not a regex",
			source: "/bin/bash",
			want:   []tapeToken{{Kind: tapeTokenWord, Value: "/bin/bash", Line: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tokenizeTape(tt.source, "test.tape")
			if tt.wantErr {
				require.ErrorIs(t, err, ErrTapeParseFailed)
				var tapeErr *TapeError
				require.ErrorAs(t, err, &tapeErr)
				assert.Equal(t, "test.tape", tapeErr.File)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTapeReportsSyntaxErrors(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantLine int
		wantMsg  string
	}{
		{name: "lexer error propagates from ParseTape", source: "Type `never closed", wantLine: 1, wantMsg: "unterminated"},
		{name: "directive keyword cannot be a string token", source: `"Type" hello`, wantLine: 1, wantMsg: "unexpected token"},
		{name: "Set without key", source: "Set", wantLine: 1, wantMsg: "missing Set key"},
		{name: "Set without value", source: "Set Shell", wantLine: 1, wantMsg: "missing Set value"},
		{name: "Env value cannot be a regex literal", source: "Env Wait /x/", wantLine: 1, wantMsg: "unexpected regex literal for Env value"},
		{name: "stray regex literal where a directive is expected", source: "Sleep Wait /x/", wantLine: 1, wantMsg: "unexpected token"},
		{name: "Output without argument", source: "Type hi\nOutput", wantLine: 2, wantMsg: "missing argument"},
		{name: "Sleep without duration", source: "Sleep", wantLine: 1, wantMsg: "missing Sleep duration"},
		{name: "Env without key", source: "Env", wantLine: 1, wantMsg: "missing Env key"},
		{name: "Env without value", source: "Env FOO", wantLine: 1, wantMsg: "missing Env value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTape(tt.source, "test.tape", ".", fakeTapeFileReader{})
			require.ErrorIs(t, err, ErrTapeParseFailed)
			assert.Contains(t, err.Error(), tt.wantMsg)
			var tapeErr *TapeError
			require.ErrorAs(t, err, &tapeErr)
			assert.Equal(t, tt.wantLine, tapeErr.Line)
		})
	}
}

func TestParseTapeRejectsRegexLiteralAsSetValueOnlyForWaitPattern(t *testing.T) {
	tape, err := ParseTape("Set WaitPattern /prompt$/", "test.tape", ".", fakeTapeFileReader{})
	require.NoError(t, err)
	require.Len(t, tape.Directives, 1)
	assert.Equal(t, TapeSet, tape.Directives[0].Kind)
	assert.Equal(t, "WaitPattern", tape.Directives[0].Key)
	assert.Equal(t, "/prompt$/", tape.Directives[0].Value)
}
