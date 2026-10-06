package starlark

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repeatLine(line string, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = line
	}
	return lines
}

func TestCollapseBacktrace(t *testing.T) {
	t.Parallel()
	const head = "Traceback (most recent call last):"
	const tail = "Error: boom"
	const f = "  a.star:2:13: in f"
	const g = "  a.star:5:9: in g"
	for _, tc := range []struct {
		name  string
		lines []string
		want  []string
	}{
		{"no repeats", []string{head, f, g, tail}, []string{head, f, g, tail}},
		{"three identical is unchanged", append(append([]string{head}, repeatLine(f, 3)...), tail), append(append([]string{head}, repeatLine(f, 3)...), tail)},
		{
			"four identical collapses to three plus marker",
			append(append([]string{head}, repeatLine(f, 4)...), tail),
			[]string{head, f, f, f, "  ... (1 more identical frame)", tail},
		},
		{
			"direct recursion",
			append(append([]string{head, g}, repeatLine(f, 10000)...), tail),
			[]string{head, g, f, f, f, "  ... (9,997 more identical frames)", tail},
		},
		{
			"mutual recursion keeps three cycles",
			append(append([]string{head}, repeatLine(f+"\n"+g, 5000)...), tail),
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := strings.Join(tc.lines, "\n")
			got := collapseBacktrace(input)
			if tc.want == nil {
				assert.Equal(t, strings.Join([]string{head, f, g, f, g, f, g, "  ... (9,994 more identical frames)", tail}, "\n"), got)
				return
			}
			assert.Equal(t, strings.Join(tc.want, "\n"), got)
		})
	}
}

func TestCollapseBacktraceLeavesTheErrorMessageIntact(t *testing.T) {
	t.Parallel()
	const head = "Traceback (most recent call last):"
	const f = "  a.star:2:13: in f"
	// A multi-line fail() message with repeated lines follows the frames.
	message := []string{"Error in fail: bad", "x", "x", "x", "x", "x"}
	input := strings.Join(append(append([]string{head}, repeatLine(f, 6)...), message...), "\n")

	got := collapseBacktrace(input)

	want := append([]string{head, f, f, f, "  ... (3 more identical frames)"}, message...)
	assert.Equal(t, strings.Join(want, "\n"), got)
}

func TestCollapseBacktraceKeepsUnchangedTextByteForByte(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"",
		"single line",
		"Traceback (most recent call last):\n  x.star:1:1: in <toplevel>\n  x.star:2:2: in a\n  x.star:3:3: in b\nError: nope\n",
		"  a\n  b\n  a\n  b\n  a\n  b\n",
	} {
		assert.Equal(t, text, collapseBacktrace(text))
	}
}

func TestGroupThousands(t *testing.T) {
	t.Parallel()
	for in, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 9994: "9,994", 1234567: "1,234,567"} {
		assert.Equal(t, want, groupThousands(in), fmt.Sprint(in))
	}
	require.Equal(t, "  ... (2 more identical frames)", collapseMarker(2))
}
