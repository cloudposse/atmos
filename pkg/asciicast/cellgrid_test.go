package asciicast

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestCast writes a minimal asciicast v3 file with the given output events.
func writeTestCast(t *testing.T, width, height int, outputs ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.cast")
	content := fmt.Sprintf(`{"version":3,"term":{"cols":%d,"rows":%d}}`+"\n", width, height)
	for _, out := range outputs {
		encoded, err := json.Marshal([]any{0.1, "o", out})
		if err != nil {
			t.Fatal(err)
		}
		content += string(encoded) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildGridLaysOutPlainLines(t *testing.T) {
	path := writeTestCast(t, 20, 5, "hello\n", "world\n")
	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width() != 20 {
		t.Fatalf("width = %d, want 20", grid.Width())
	}
	if got := gridText(grid); got != "hello\nworld\n" {
		t.Fatalf("text = %q", got)
	}
}

func TestBuildGridGrowsPastRecordedHeight(t *testing.T) {
	path := writeTestCast(t, 10, 2, "one\ntwo\nthree\nfour\n")
	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := gridText(grid); got != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("scrollback lost: %q", got)
	}
}

func TestBuildGridGrowsPastRecordedWidth(t *testing.T) {
	long := "this line is much longer than ten columns"
	path := writeTestCast(t, 10, 2, long+"\n")
	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := gridText(grid); got != long+"\n" {
		t.Fatalf("overflow lost: %q", got)
	}
}

func TestBuildGridCarriageReturnOverwrites(t *testing.T) {
	path := writeTestCast(t, 20, 3, "25%\r50%\r100%\n")
	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := gridText(grid); got != "100%\n" {
		t.Fatalf("carriage return not collapsed: %q", got)
	}
}

func TestSanitizeStreamDropsOSCAndCursorReplies(t *testing.T) {
	in := "\x1b]0;window title\x07before\x1b[24;80Rafter\n"
	got := sanitizeStream(in)
	if got != "beforeafter\n" {
		t.Fatalf("sanitized = %q", got)
	}
}

func TestSanitizeStreamKeepsSGRAndHyperlinks(t *testing.T) {
	in := "\x1b[1;32mgreen\x1b[0m \x1b]8;;https://atmos.tools\x07link\x1b]8;;\x07\n"
	got := sanitizeStream(in)
	if got != in {
		t.Fatalf("styling was stripped: %q", got)
	}
}

func TestSanitizeStreamExpandsTabs(t *testing.T) {
	got := sanitizeStream("ab\tc\n")
	if got != "ab      c\n" {
		t.Fatalf("tab expansion = %q", got)
	}
}

func TestSanitizeStreamHandlesIncompleteEscape(t *testing.T) {
	got := sanitizeStream("before\x1b")
	if got != "before" {
		t.Fatalf("sanitized = %q", got)
	}
}

func TestMeasureContentWidthUsesWidestSegment(t *testing.T) {
	width, height := measureContent("short\r123456789\nab\n")
	if width != 9 || height != 2 {
		t.Fatalf("measure = (%d, %d), want (9, 2)", width, height)
	}
}

func TestMeasureContentEmptyReturnsZero(t *testing.T) {
	width, height := measureContent("")
	if width != 0 || height != 0 {
		t.Fatalf("measure = (%d, %d), want (0, 0)", width, height)
	}
}

func TestBuildGridPropagatesReadEventsError(t *testing.T) {
	_, err := BuildGrid(filepath.Join(t.TempDir(), "does-not-exist.cast"))
	if err == nil {
		t.Fatal("expected error for missing cast file")
	}
}

// writeCastWithStreams writes a cast file whose events carry the given
// (stream, data) pairs verbatim, unlike writeTestCast which always uses the
// "o" stream. This lets tests exercise BuildGrid's per-event stream filter.
func writeCastWithStreams(t *testing.T, width, height int, events ...[2]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "streams.cast")
	content := fmt.Sprintf(`{"version":3,"term":{"cols":%d,"rows":%d}}`+"\n", width, height)
	for _, event := range events {
		encoded, err := json.Marshal([]any{0.1, event[0], event[1]})
		if err != nil {
			t.Fatal(err)
		}
		content += string(encoded) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildGridSkipsUnknownStreams(t *testing.T) {
	// The "i" (input) stream is a known event stream per isKnownStream, but
	// BuildGrid's content accumulation only appends "o" and "e" output; other
	// known streams must be skipped without affecting rendered content.
	path := writeCastWithStreams(
		t, 20, 3,
		[2]string{"i", "typed-but-not-rendered"},
		[2]string{"o", "visible\n"},
	)
	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := gridText(grid); got != "visible\n" {
		t.Fatalf("text = %q, want only the o-stream content", got)
	}
}

func TestBuildGridIncludesErrorStream(t *testing.T) {
	path := writeCastWithStreams(t, 20, 3, [2]string{"e", "stderr line\n"})
	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := gridText(grid); got != "stderr line\n" {
		t.Fatalf("text = %q, want e-stream content included", got)
	}
}

func TestBuildGridDefaultsWidthWhenHeaderHasNone(t *testing.T) {
	// No "term" object and no top-level "width" field: BuildGrid must fall
	// back to DefaultWidth rather than using a zero or negative width.
	path := filepath.Join(t.TempDir(), "no-width.cast")
	content := `{"version":3}` + "\n"
	encoded, err := json.Marshal([]any{0.1, "o", "hi\n"})
	if err != nil {
		t.Fatal(err)
	}
	content += string(encoded) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	grid, err := BuildGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width() != DefaultWidth {
		t.Fatalf("width = %d, want DefaultWidth (%d)", grid.Width(), DefaultWidth)
	}
}

func TestContentGridDefaultsHeightForEmptyContent(t *testing.T) {
	// measureContent("") returns height 0; contentGrid must clamp that to a
	// minimum of 1 so callers always get a valid (non-degenerate) buffer.
	buf := contentGrid("", 10)
	if buf.Height() != 1 {
		t.Fatalf("height = %d, want 1", buf.Height())
	}
	if buf.Width() != 10 {
		t.Fatalf("width = %d, want termWidth (10)", buf.Width())
	}
}

func TestSanitizeStreamRecoversFromInvalidUTF8(t *testing.T) {
	// A lone continuation byte (0x80) is not a valid UTF-8 sequence start.
	// With the vendored ansi.DecodeSequence, invalid bytes are still consumed
	// one at a time via its own fallback (n stays >= 1), so this does not
	// reach the `n <= 0` recovery branch in sanitizeStream (see the note on
	// TestSanitizeStreamRecoversFromTruncatedMultiByteSequence below), but it
	// still verifies malformed input degrades gracefully instead of
	// corrupting or dropping the surrounding valid text.
	in := "before\x80after\n"
	got := sanitizeStream(in)
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("sanitized = %q, want surrounding text preserved", got)
	}
}

func TestSanitizeStreamRecoversFromTruncatedMultiByteSequence(t *testing.T) {
	// "\xe2\x82" is a truncated 3-byte UTF-8 sequence (missing the final
	// continuation byte). Same as above: exercises malformed-input handling,
	// though not the `n <= 0` branch specifically (see cellgrid.go's
	// sanitizeStream for why that branch is currently unreachable).
	in := "start\xe2\x82end\n"
	got := sanitizeStream(in)
	if !strings.Contains(got, "start") || !strings.Contains(got, "end") {
		t.Fatalf("sanitized = %q, want surrounding text preserved", got)
	}
}
