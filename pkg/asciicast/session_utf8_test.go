package asciicast

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestSessionRecordsUTF8AcrossReadBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"split-runes", "progress ░█ ⣽ 🚀 café\r\n> ", "progress ░█ ⣽ 🚀 café\r\n> "},
		{"truncated-at-eof", "tail\xe2\x96", "tail\ufffd\ufffd"},
		{"invalid-byte", "bad\xffok", "bad\ufffdok"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, iolib.Initialize())
			rec, err := Start(&Options{Path: filepath.Join(t.TempDir(), "unicode.cast")})
			require.NoError(t, err)
			restore := iolib.SetRecorder(rec)
			defer restore()
			state := newSessionState(context.Background(), iotest.OneByteReader(strings.NewReader(tc.input)), &syncWriter{w: io.Discard}, nil)
			defer state.stop()
			require.NoError(t, <-state.done)
			require.NoError(t, rec.Close())
			_, events, err := ReadEvents(rec.Path())
			require.NoError(t, err)
			var got strings.Builder
			for _, event := range events {
				got.WriteString(event.Data)
			}
			require.Equal(t, tc.want, got.String())
			require.Equal(t, tc.input != "", outputMatches(state, tc.input, nil))
		})
	}
}
