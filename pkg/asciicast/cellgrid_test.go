package asciicast

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func TestMeasureContentWidthUsesWidestSegment(t *testing.T) {
	width, height := measureContent("short\r123456789\nab\n")
	if width != 9 || height != 2 {
		t.Fatalf("measure = (%d, %d), want (9, 2)", width, height)
	}
}
