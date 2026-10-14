package query

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponderAnswersQueries(t *testing.T) {
	tests := []struct {
		name  string
		chunk string
		want  string
	}{
		{"background ST", "\x1b]11;?\x1b\\", backgroundReply},
		{"background BEL", "\x1b]11;?\x07", backgroundReply},
		{"foreground ST", "\x1b]10;?\x1b\\", foregroundReply},
		{"foreground BEL", "\x1b]10;?\x07", foregroundReply},
		{"cursor position", "\x1b[6n", cursorPositionReply},
		{"each repeated cursor query", "\x1b[6n\x1b[6n", cursorPositionReply + cursorPositionReply},
		{"no queries", "hello \x1b[31mred\x1b[0m", ""},
		{"set color is not a query", "\x1b]11;rgb:0000/0000/0000\x1b\\", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sink bytes.Buffer
			NewResponder(&sink).Scan([]byte(tt.chunk))
			assert.Equal(t, tt.want, sink.String())
		})
	}
}

func TestResponderAllQueriesInOneChunk(t *testing.T) {
	var sink bytes.Buffer
	NewResponder(&sink).Scan([]byte("\x1b]11;?\x1b\\\x1b[6n\x1b]10;?\x1b\\\x1b[6n"))

	got := sink.String()
	assert.Contains(t, got, backgroundReply)
	assert.Contains(t, got, foregroundReply)
	assert.Equal(t, 2, strings.Count(got, cursorPositionReply))
}

func TestResponderAnswersQueryExactlyOnceAcrossSplits(t *testing.T) {
	const input = "abc\x1b]11;?\x1b\\def\x1b[6nghi"
	for split := 1; split < len(input); split++ {
		var sink bytes.Buffer
		r := NewResponder(&sink)
		r.Scan([]byte(input[:split]))
		r.Scan([]byte(input[split:]))
		assert.Equal(t, backgroundReply+cursorPositionReply, sink.String(), "split at %d", split)
	}
}

func TestResponderNilSafe(t *testing.T) {
	var nilResponder *Responder
	assert.NotPanics(t, func() { nilResponder.Scan([]byte("\x1b[6n")) })
	assert.NotPanics(t, func() { NewResponder(nil).Scan([]byte("\x1b[6n")) })
}

func TestReaderForwardsOutputUnchanged(t *testing.T) {
	input := "plain\r\n\x1b[1mbold\x1b[0m\x1b]11;?\x1b\\more text\x1b[6n\x1b]10;?\x07end"

	for name, src := range map[string]io.Reader{
		"whole":    strings.NewReader(input),
		"one byte": iotest.OneByteReader(strings.NewReader(input)),
		"half":     iotest.HalfReader(strings.NewReader(input)),
	} {
		t.Run(name, func(t *testing.T) {
			var sink, out bytes.Buffer
			_, err := io.Copy(&out, NewReader(src, &sink))
			require.NoError(t, err)

			assert.Equal(t, input, out.String())
			assert.Equal(t, backgroundReply+cursorPositionReply+foregroundReply, sink.String())
		})
	}
}

func TestReaderWithoutQueriesWritesNothing(t *testing.T) {
	var sink, out bytes.Buffer
	_, err := io.Copy(&out, NewReader(iotest.OneByteReader(strings.NewReader("just output\n")), &sink))
	require.NoError(t, err)
	assert.Equal(t, "just output\n", out.String())
	assert.Empty(t, sink.String())
}
